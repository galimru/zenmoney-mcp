package transactions

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/galimru/zenmoney-mcp/internal/config"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

func batchRuntime(pushFn func(ctx context.Context, req models.Request) (models.Response, error)) *mockRuntime {
	return &mockRuntime{
		cfg:    &config.Config{TransactionLimit: 100},
		client: &mockClient{pushFn: pushFn},
		resp:   testResponse(),
	}
}

func echoPush(ctx context.Context, req models.Request) (models.Response, error) {
	return models.Response{ServerTimestamp: 2000, Transaction: req.Transaction}, nil
}

func TestServiceEditBatch_UpdatesAllRowsInOnePush(t *testing.T) {
	var pushes int
	var pushed []models.Transaction
	rt := batchRuntime(func(ctx context.Context, req models.Request) (models.Response, error) {
		pushes++
		pushed = append(pushed, req.Transaction...)
		return echoPush(ctx, req)
	})
	svc := NewService(rt)

	out, err := svc.EditBatch(context.Background(), EditBatchInput{Items: []EditInput{
		{TransactionID: "tx-uncategorized-expense", WriteInput: WriteInput{Category: "Food"}},
		{TransactionID: "tx-food", WriteInput: WriteInput{Payee: "New Bakery", PayeeSet: true}},
	}})
	if err != nil {
		t.Fatalf("EditBatch() error = %v", err)
	}
	if !out.Updated {
		t.Fatalf("EditBatch() = %+v, want Updated=true", out)
	}
	if out.Count != 2 || len(out.Items) != 2 {
		t.Fatalf("EditBatch() count = %d, items = %d, want 2/2", out.Count, len(out.Items))
	}
	if pushes != 1 {
		t.Fatalf("push called %d times, want a single batched push", pushes)
	}
	if len(pushed) != 2 {
		t.Fatalf("pushed %d transactions, want 2", len(pushed))
	}
}

func TestServiceEditBatch_LoadsEnvironmentOnce(t *testing.T) {
	rt := batchRuntime(echoPush)
	svc := NewService(rt)

	_, err := svc.EditBatch(context.Background(), EditBatchInput{Items: []EditInput{
		{TransactionID: "tx-food", WriteInput: WriteInput{Payee: "A", PayeeSet: true}},
		{TransactionID: "tx-uncategorized-expense", WriteInput: WriteInput{Payee: "B", PayeeSet: true}},
		{TransactionID: "tx-uncategorized-transfer", WriteInput: WriteInput{Payee: "C", PayeeSet: true}},
	}})
	if err != nil {
		t.Fatalf("EditBatch() error = %v", err)
	}
	if rt.syncCalls != 1 {
		t.Fatalf("ScopedSync called %d times, want exactly 1 for the whole batch", rt.syncCalls)
	}
}

func TestServiceEditBatch_BlocksWholeBatchWhenRowIsUnknown(t *testing.T) {
	var pushes int
	rt := batchRuntime(func(ctx context.Context, req models.Request) (models.Response, error) {
		pushes++
		return echoPush(ctx, req)
	})
	svc := NewService(rt)

	out, err := svc.EditBatch(context.Background(), EditBatchInput{Items: []EditInput{
		{TransactionID: "tx-food", WriteInput: WriteInput{Payee: "New Bakery", PayeeSet: true}},
		{TransactionID: "tx-missing", WriteInput: WriteInput{Payee: "Nope", PayeeSet: true}},
	}})
	if err != nil {
		t.Fatalf("EditBatch() error = %v", err)
	}
	if out.Updated {
		t.Fatalf("EditBatch() = %+v, want Updated=false", out)
	}
	if pushes != 0 {
		t.Fatalf("push called %d times, want 0 when the batch is blocked", pushes)
	}
	if len(out.Rows) != 1 {
		t.Fatalf("EditBatch() rows = %+v, want exactly one problem row", out.Rows)
	}
	if out.Rows[0].Index != 1 || out.Rows[0].Status != "not_found" {
		t.Fatalf("EditBatch() row = %+v, want index 1 with status not_found", out.Rows[0])
	}
}

func TestServiceEditBatch_ReportsDuplicateIDs(t *testing.T) {
	rt := batchRuntime(echoPush)
	svc := NewService(rt)

	out, err := svc.EditBatch(context.Background(), EditBatchInput{Items: []EditInput{
		{TransactionID: "tx-food", WriteInput: WriteInput{Payee: "First", PayeeSet: true}},
		{TransactionID: "tx-food", WriteInput: WriteInput{Payee: "Second", PayeeSet: true}},
	}})
	if err != nil {
		t.Fatalf("EditBatch() error = %v", err)
	}
	if out.Updated {
		t.Fatalf("EditBatch() = %+v, want Updated=false for duplicate IDs", out)
	}
	if len(out.Rows) != 1 || out.Rows[0].Status != "duplicate" || out.Rows[0].Index != 1 {
		t.Fatalf("EditBatch() rows = %+v, want the second row flagged as duplicate", out.Rows)
	}
}

func TestServiceEditBatch_SplitsLargeBatchIntoChunks(t *testing.T) {
	var sizes []int
	rt := batchRuntime(func(ctx context.Context, req models.Request) (models.Response, error) {
		sizes = append(sizes, len(req.Transaction))
		return echoPush(ctx, req)
	})
	svc := NewService(rt)

	resp := testResponse()
	outcomeAcc := "account-1"
	var items []EditInput
	for i := range 5 {
		id := fmt.Sprintf("tx-bulk-%d", i)
		resp.Transaction = append(resp.Transaction, models.Transaction{
			ID:                id,
			User:              42,
			Date:              "2024-02-01",
			Outcome:           10,
			IncomeAccount:     "account-1",
			OutcomeAccount:    &outcomeAcc,
			IncomeInstrument:  1,
			OutcomeInstrument: 1,
		})
		items = append(items, EditInput{TransactionID: id, WriteInput: WriteInput{Category: "Food"}})
	}
	rt.resp = resp

	out, err := svc.EditBatch(context.Background(), EditBatchInput{Items: items, ChunkSize: 2})
	if err != nil {
		t.Fatalf("EditBatch() error = %v", err)
	}
	if !out.Updated || out.Count != 5 {
		t.Fatalf("EditBatch() = %+v, want 5 updated rows", out)
	}
	if out.Chunks != 3 {
		t.Fatalf("EditBatch() chunks = %d, want 3", out.Chunks)
	}
	if len(sizes) != 3 || sizes[0] != 2 || sizes[1] != 2 || sizes[2] != 1 {
		t.Fatalf("push sizes = %v, want [2 2 1]", sizes)
	}
}

func TestServiceEditBatch_StopsAndReportsWhenAChunkFails(t *testing.T) {
	var pushes int
	rt := batchRuntime(func(ctx context.Context, req models.Request) (models.Response, error) {
		pushes++
		if pushes == 2 {
			return models.Response{}, fmt.Errorf("network is down")
		}
		return echoPush(ctx, req)
	})
	svc := NewService(rt)

	out, err := svc.EditBatch(context.Background(), EditBatchInput{
		ChunkSize: 1,
		Items: []EditInput{
			{TransactionID: "tx-food", WriteInput: WriteInput{Payee: "A", PayeeSet: true}},
			{TransactionID: "tx-uncategorized-expense", WriteInput: WriteInput{Payee: "B", PayeeSet: true}},
			{TransactionID: "tx-uncategorized-transfer", WriteInput: WriteInput{Payee: "C", PayeeSet: true}},
		},
	})
	if err != nil {
		t.Fatalf("EditBatch() error = %v", err)
	}
	if out.Updated {
		t.Fatalf("EditBatch() = %+v, want Updated=false after a failed chunk", out)
	}
	if pushes != 2 {
		t.Fatalf("push called %d times, want 2 — the run must stop at the first failing chunk", pushes)
	}
	if out.Count != 1 {
		t.Fatalf("EditBatch() count = %d, want 1 already-applied row", out.Count)
	}
	if !strings.Contains(out.Message, "network is down") {
		t.Fatalf("EditBatch() message = %q, want the transport error surfaced", out.Message)
	}
	if len(out.Rows) != 2 {
		t.Fatalf("EditBatch() rows = %+v, want the two unapplied rows reported", out.Rows)
	}
}

func TestServiceEditBatch_RejectsEmptyBatch(t *testing.T) {
	svc := NewService(batchRuntime(echoPush))

	if _, err := svc.EditBatch(context.Background(), EditBatchInput{}); err == nil {
		t.Fatal("EditBatch() error = nil, want an error for an empty batch")
	}
}

func TestServiceEditBatch_ReturnItemsFalseOmitsEcho(t *testing.T) {
	rt := batchRuntime(echoPush)
	svc := NewService(rt)
	no := false

	out, err := svc.EditBatch(context.Background(), EditBatchInput{
		ReturnItems: &no,
		Items: []EditInput{
			{TransactionID: "tx-uncategorized-expense", WriteInput: WriteInput{Category: "Food"}},
			{TransactionID: "tx-food", WriteInput: WriteInput{Payee: "New Bakery", PayeeSet: true}},
		},
	})
	if err != nil {
		t.Fatalf("EditBatch() error = %v", err)
	}
	if out.Count != 2 {
		t.Fatalf("EditBatch() count = %d, want 2", out.Count)
	}
	if len(out.Items) != 0 || out.ItemsOmitted != 2 {
		t.Fatalf("EditBatch() items = %d, omitted = %d, want 0/2", len(out.Items), out.ItemsOmitted)
	}
}

func TestEchoItems_DefaultsBySize(t *testing.T) {
	small := make([]TransactionResult, editBatchItemsEchoLimit)
	if items, omitted := echoItems(small, nil); len(items) != len(small) || omitted != 0 {
		t.Fatalf("echoItems(small) = %d/%d, want %d/0", len(items), omitted, len(small))
	}

	large := make([]TransactionResult, editBatchItemsEchoLimit+1)
	if items, omitted := echoItems(large, nil); items != nil || omitted != len(large) {
		t.Fatalf("echoItems(large) = %d/%d, want 0/%d", len(items), omitted, len(large))
	}

	yes := true
	if items, omitted := echoItems(large, &yes); len(items) != len(large) || omitted != 0 {
		t.Fatalf("echoItems(large, return_items=true) = %d/%d, want %d/0", len(items), omitted, len(large))
	}
}
