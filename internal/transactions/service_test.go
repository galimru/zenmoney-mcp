package transactions

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/galimru/zenmoney-mcp/client"
	"github.com/galimru/zenmoney-mcp/internal/config"
	"github.com/galimru/zenmoney-mcp/internal/runtime"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

type mockRuntime struct {
	cfg        *config.Config
	client     client.ZenClient
	resp       models.Response
	serverTime int
}

func (m *mockRuntime) Config() (*config.Config, error) {
	return m.cfg, nil
}

func (m *mockRuntime) Client() (client.ZenClient, error) {
	return m.client, nil
}

func (m *mockRuntime) ScopedSync(context.Context, []models.EntityType) (models.Response, runtime.LookupMaps, error) {
	return m.resp, runtime.BuildLookupMaps(m.resp), nil
}

func (m *mockRuntime) CurrentServerTimestamp() int {
	return m.serverTime
}

func (m *mockRuntime) SaveServerTimestamp(serverTimestamp int) error {
	m.serverTime = serverTimestamp
	return nil
}

type mockClient struct {
	pushFn    func(ctx context.Context, req models.Request) (models.Response, error)
	suggestFn func(ctx context.Context, tx models.Transaction) (models.Transaction, error)
}

func (m *mockClient) FullSync(context.Context) (models.Response, error) {
	return models.Response{}, nil
}
func (m *mockClient) SyncSince(context.Context, time.Time) (models.Response, error) {
	return models.Response{}, nil
}
func (m *mockClient) Sync(context.Context, models.Request) (models.Response, error) {
	return models.Response{}, nil
}
func (m *mockClient) Push(ctx context.Context, req models.Request) (models.Response, error) {
	if m.pushFn != nil {
		return m.pushFn(ctx, req)
	}
	return models.Response{}, nil
}
func (m *mockClient) Suggest(ctx context.Context, tx models.Transaction) (models.Transaction, error) {
	if m.suggestFn != nil {
		return m.suggestFn(ctx, tx)
	}
	return tx, nil
}

func testResponse() models.Response {
	instr1 := int32(1)
	outcomeAcc := "account-1"
	transferOutcomeAcc := "account-1"
	return models.Response{
		ServerTimestamp: 1000,
		User:            []models.User{{ID: 42}},
		Instrument:      []models.Instrument{{ID: 1, Symbol: "$", ShortTitle: "USD", Title: "US Dollar"}},
		Account: []models.Account{
			{ID: "account-1", Title: "Cash", Instrument: &instr1},
			{ID: "account-2", Title: "Card", Instrument: &instr1},
		},
		Tag: []models.Tag{
			{ID: "tag-food", Title: "Food"},
			{ID: "tag-salary", Title: "Salary"},
		},
		Transaction: []models.Transaction{
			{
				ID:                "tx-food",
				User:              42,
				Date:              "2024-01-15",
				Outcome:           500,
				IncomeAccount:     "account-1",
				OutcomeAccount:    &outcomeAcc,
				IncomeInstrument:  1,
				OutcomeInstrument: 1,
				Tag:               []string{"tag-food"},
				Payee:             "Bakery",
			},
			{
				ID:                "tx-uncategorized-expense",
				User:              42,
				Date:              "2024-01-16",
				Outcome:           120,
				IncomeAccount:     "account-1",
				OutcomeAccount:    &outcomeAcc,
				IncomeInstrument:  1,
				OutcomeInstrument: 1,
				Payee:             "Coffee Shop",
			},
			{
				ID:                "tx-uncategorized-transfer",
				User:              42,
				Date:              "2024-01-17",
				Income:            75,
				Outcome:           75,
				IncomeAccount:     "account-2",
				OutcomeAccount:    &transferOutcomeAcc,
				IncomeInstrument:  1,
				OutcomeInstrument: 1,
				Payee:             "Transfer",
			},
		},
	}
}

func TestServiceFind_FiltersTransactions(t *testing.T) {
	rt := &mockRuntime{
		cfg:    &config.Config{TransactionLimit: 100},
		client: &mockClient{},
		resp:   testResponse(),
	}
	svc := NewService(rt)

	out, err := svc.Find(context.Background(), FindInput{
		Account:  "Cash",
		Category: "Food",
		Query:    "bak",
	})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if out.Total != 1 || len(out.Items) != 1 {
		t.Fatalf("Find() = %+v, want exactly one match", out)
	}
}

func TestServiceListUncategorized_ExcludesTransfers(t *testing.T) {
	rt := &mockRuntime{
		cfg:    &config.Config{TransactionLimit: 100},
		client: &mockClient{},
		resp:   testResponse(),
	}
	svc := NewService(rt)

	out, err := svc.ListUncategorized(context.Background(), FindInput{})
	if err != nil {
		t.Fatalf("ListUncategorized() error = %v", err)
	}
	if out.Total != 1 || len(out.Items) != 1 {
		t.Fatalf("ListUncategorized() = %+v, want exactly one uncategorized non-transfer", out)
	}
	if out.Items[0].ID != "tx-uncategorized-expense" {
		t.Fatalf("ListUncategorized() returned %q, want tx-uncategorized-expense", out.Items[0].ID)
	}
}

func TestServiceEdit_UpdatesPayeeAndComment(t *testing.T) {
	rt := &mockRuntime{
		cfg: &config.Config{TransactionLimit: 100},
		client: &mockClient{
			pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
				return models.Response{ServerTimestamp: 2000, Transaction: req.Transaction}, nil
			},
		},
		resp: testResponse(),
	}
	svc := NewService(rt)

	out, err := svc.Edit(context.Background(), EditInput{
		TransactionID: "tx-food",
		WriteInput: WriteInput{
			Payee:      "New Bakery",
			PayeeSet:   true,
			Comment:    "fresh note",
			CommentSet: true,
		},
	})
	if err != nil {
		t.Fatalf("Edit() error = %v", err)
	}
	if out == nil || out.Payee != "New Bakery" || out.Comment != "fresh note" {
		t.Fatalf("Edit() = %+v, want updated payee/comment", out)
	}
}

func TestServiceImportTransactions_ImportsAllRowsInOnePush(t *testing.T) {
	var pushed []models.Transaction
	client := &mockClient{
		pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			pushed = append(pushed, req.Transaction...)
			return models.Response{ServerTimestamp: 2000}, nil
		},
		suggestFn: func(ctx context.Context, tx models.Transaction) (models.Transaction, error) {
			tx.Tag = []string{"tag-food"}
			return tx, nil
		},
	}
	rt := &mockRuntime{
		cfg:        &config.Config{TransactionLimit: 100},
		client:     client,
		resp:       testResponse(),
		serverTime: 1000,
	}
	svc := NewService(rt)

	out, err := svc.ImportTransactions(context.Background(), ImportTransactionsInput{
		AccountID: "account-1",
		Items: []ImportDraft{{
			Date:      "2024-04-02",
			Amount:    15,
			Type:      "expense",
			AccountID: "account-1",
			Payee:     "Coffee Shop",
		}},
	})
	if err != nil {
		t.Fatalf("ImportTransactions() error = %v", err)
	}
	if !out.Imported {
		t.Fatalf("ImportTransactions() = %+v, want Imported=true", out)
	}
	if len(pushed) != 1 {
		t.Fatalf("pushed=%d, want 1", len(pushed))
	}
}

func TestServiceImportTransactions_RequiresAccountID(t *testing.T) {
	rt := &mockRuntime{
		cfg:    &config.Config{TransactionLimit: 100},
		client: &mockClient{},
		resp:   testResponse(),
	}
	svc := NewService(rt)

	_, err := svc.ImportTransactions(context.Background(), ImportTransactionsInput{
		Items: []ImportDraft{{
			Date:   "2024-04-02",
			Amount: 15,
			Type:   "expense",
			Payee:  "Coffee Shop",
		}},
	})
	if err == nil {
		t.Fatal("expected account_id validation error")
	}
}

func TestServiceImportTransactions_ImportsWholeBatchInSinglePush(t *testing.T) {
	var pushed []models.Transaction
	rt := &mockRuntime{
		cfg: &config.Config{TransactionLimit: 100},
		client: &mockClient{
			pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
				pushed = append(pushed, req.Transaction...)
				return models.Response{ServerTimestamp: 2000}, nil
			},
		},
		resp: testResponse(),
	}
	svc := NewService(rt)

	items := []ImportDraft{
		{Date: "2024-04-01", Amount: 10, Type: "expense", Payee: "One"},
		{Date: "2024-04-02", Amount: 20, Type: "expense", Payee: "Two"},
		{Date: "2024-04-03", Amount: 30, Type: "expense", Payee: "Three"},
	}
	out, err := svc.ImportTransactions(context.Background(), ImportTransactionsInput{
		AccountID: "account-1",
		Items:     items,
	})
	if err != nil {
		t.Fatalf("ImportTransactions() error = %v", err)
	}
	if !out.Imported || len(pushed) != 3 {
		t.Fatalf("ImportTransactions() = %+v, pushed=%d, want three imported rows", out, len(pushed))
	}
}

func TestServiceImportTransactions_BlocksDuplicateRows(t *testing.T) {
	var pushed []models.Transaction
	rt := &mockRuntime{
		cfg: &config.Config{TransactionLimit: 100},
		client: &mockClient{
			pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
				pushed = append(pushed, req.Transaction...)
				return models.Response{ServerTimestamp: 2000}, nil
			},
		},
		resp:       testResponse(),
		serverTime: 1000,
	}
	svc := NewService(rt)

	out, err := svc.ImportTransactions(context.Background(), ImportTransactionsInput{
		AccountID: "account-1",
		Items: []ImportDraft{{
			Date:   "2024-01-15",
			Amount: 500,
			Type:   "expense",
			Payee:  "Bakery",
		}},
	})
	if err != nil {
		t.Fatalf("ImportTransactions() error = %v", err)
	}
	if out.Imported {
		t.Fatalf("ImportTransactions() = %+v, want blocked import", out)
	}
	if out.Message == "" || !strings.Contains(out.Message, "Row 0") {
		t.Fatalf("ImportTransactions() = %+v, want message mentioning Row 0", out)
	}
	if len(out.Rows) != 1 || out.Rows[0].Index != 0 || out.Rows[0].Status != "duplicate" {
		t.Fatalf("ImportTransactions() rows = %+v, want one duplicate at index 0", out.Rows)
	}
	if len(pushed) != 0 {
		t.Fatalf("pushed=%d, want 0", len(pushed))
	}
}

func TestServiceImportTransactions_BlocksDuplicateRowsWithinBatch(t *testing.T) {
	var pushed []models.Transaction
	rt := &mockRuntime{
		cfg: &config.Config{TransactionLimit: 100},
		client: &mockClient{
			pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
				pushed = append(pushed, req.Transaction...)
				return models.Response{ServerTimestamp: 2000}, nil
			},
		},
		resp: testResponse(),
	}
	svc := NewService(rt)

	out, err := svc.ImportTransactions(context.Background(), ImportTransactionsInput{
		AccountID: "account-1",
		Items: []ImportDraft{
			{Date: "2024-04-02", Amount: 15, Type: "expense", Payee: "Coffee Shop"},
			{Date: "2024-04-02", Amount: 15, Type: "expense", Payee: "Coffee Shop"},
		},
	})
	if err != nil {
		t.Fatalf("ImportTransactions() error = %v", err)
	}
	if out.Imported || !strings.Contains(out.Message, "Row 1") {
		t.Fatalf("ImportTransactions() = %+v, want message mentioning in-batch duplicate at Row 1", out)
	}
	if len(out.Rows) != 1 || out.Rows[0].Index != 1 || out.Rows[0].Status != "duplicate" {
		t.Fatalf("ImportTransactions() rows = %+v, want one duplicate at index 1", out.Rows)
	}
	if len(pushed) != 0 {
		t.Fatalf("pushed=%d, want 0", len(pushed))
	}
}

func TestServiceImportTransactions_ReturnsFailureRowsWhenPushFails(t *testing.T) {
	rt := &mockRuntime{
		cfg: &config.Config{TransactionLimit: 100},
		client: &mockClient{
			pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
				return models.Response{}, context.DeadlineExceeded
			},
		},
		resp: testResponse(),
	}
	svc := NewService(rt)

	out, err := svc.ImportTransactions(context.Background(), ImportTransactionsInput{
		AccountID: "account-1",
		Items: []ImportDraft{
			{Date: "2024-04-02", Amount: 15, Type: "expense", Payee: "Coffee Shop"},
			{Date: "2024-04-03", Amount: 25, Type: "expense", Payee: "Bakery 2"},
		},
	})
	if err != nil {
		t.Fatalf("ImportTransactions() error = %v", err)
	}
	if out.Imported || !strings.Contains(out.Message, "Import failed") {
		t.Fatalf("ImportTransactions() = %+v, want failure message", out)
	}
	if len(out.Rows) != 2 || out.Rows[0].Status != "failed" {
		t.Fatalf("ImportTransactions() rows = %+v, want two failed rows", out.Rows)
	}
}
