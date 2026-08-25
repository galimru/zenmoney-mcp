package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

func TestHandleEditTransactions_CategorizesManyRowsInOnePush(t *testing.T) {
	var pushes int
	var pushed []models.Transaction
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return workflowSyncResponse(), nil
		},
		pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			pushes++
			pushed = append(pushed, req.Transaction...)
			return models.Response{ServerTimestamp: 2000}, nil
		},
	}
	p := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"items": []any{
			map[string]any{"transaction_id": "tx-uncategorized", "category": "Food"},
			map[string]any{"transaction_id": "tx-existing-food", "payee": "New Bakery"},
		},
	})

	result, err := handleEditTransactions(context.Background(), p, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	if pushes != 1 {
		t.Fatalf("push called %d times, want a single batched push", pushes)
	}
	if len(pushed) != 2 {
		t.Fatalf("pushed %d transactions, want 2", len(pushed))
	}
	for _, tx := range pushed {
		if tx.ID == "tx-uncategorized" && (len(tx.Tag) != 1 || tx.Tag[0] != "tag-food") {
			t.Fatalf("tx-uncategorized tags = %v, want [tag-food]", tx.Tag)
		}
		if tx.ID == "tx-existing-food" && tx.Payee != "New Bakery" {
			t.Fatalf("tx-existing-food payee = %q, want New Bakery", tx.Payee)
		}
	}
}

func TestHandleEditTransactions_ReportsBadRowsWithoutWriting(t *testing.T) {
	var pushes int
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return workflowSyncResponse(), nil
		},
		pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			pushes++
			return models.Response{ServerTimestamp: 2000}, nil
		},
	}
	p := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"items": []any{
			map[string]any{"transaction_id": "tx-uncategorized", "category": "Food"},
			map[string]any{"transaction_id": "tx-does-not-exist", "category": "Food"},
		},
	})

	result, err := handleEditTransactions(context.Background(), p, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	if pushes != 0 {
		t.Fatalf("push called %d times, want 0 when a row is unusable", pushes)
	}
	if !strings.Contains(resultText(t, result), "not_found") {
		t.Fatalf("result = %s, want the unknown row reported as not_found", resultText(t, result))
	}
}

func TestHandleEditTransactions_RequiresItems(t *testing.T) {
	p := newTestRuntime(&mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return workflowSyncResponse(), nil
		},
	})

	result, err := handleEditTransactions(context.Background(), p, mcpReqWithArgs(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected an error result when items is missing")
	}
}
