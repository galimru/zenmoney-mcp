package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

func sampleSyncResponse() models.Response {
	instr1 := int32(1)
	acc1 := "account-1"
	return models.Response{
		ServerTimestamp: 1000,
		User:            []models.User{{ID: 42}},
		Instrument: []models.Instrument{
			{ID: 1, Symbol: "₽"},
		},
		Account: []models.Account{
			{ID: "account-1", Title: "Cash", Instrument: &instr1},
			{ID: "account-2", Title: "Card", Instrument: &instr1},
		},
		Tag: []models.Tag{
			{ID: "tag-food", Title: "Food"},
		},
		Transaction: []models.Transaction{
			{
				ID:                "tx-1",
				User:              42,
				Date:              "2024-01-15",
				Income:            0,
				Outcome:           500,
				IncomeAccount:     "account-1",
				OutcomeAccount:    &acc1,
				IncomeInstrument:  1,
				OutcomeInstrument: 1,
				Tag:               []string{"tag-food"},
				Payee:             "McDonalds",
			},
		},
	}
}

func TestHandleCreateTransaction_Expense(t *testing.T) {
	var pushedTxs []models.Transaction
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return sampleSyncResponse(), nil
		},
		pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			pushedTxs = req.Transaction
			return models.Response{ServerTimestamp: 2000}, nil
		},
	}
	runtime := newTestRuntime(mc)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"transaction_type": "expense",
		"date":             "2024-03-10",
		"account_id":       "account-1",
		"amount":           float64(300),
		"payee":            "Store",
	}

	result, err := handleCreateTransaction(context.Background(), runtime, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}

	if len(pushedTxs) != 1 {
		t.Fatalf("expected 1 pushed transaction, got %d", len(pushedTxs))
	}
	tx := pushedTxs[0]
	if tx.Outcome != 300 {
		t.Errorf("expense Outcome = %v, want 300", tx.Outcome)
	}
	if tx.Income != 0 {
		t.Errorf("expense Income = %v, want 0", tx.Income)
	}
	if tx.Payee != "Store" {
		t.Errorf("Payee = %q, want Store", tx.Payee)
	}
	if tx.User != 42 {
		t.Errorf("User = %d, want 42", tx.User)
	}
}

func TestHandleCreateTransaction_Income(t *testing.T) {
	var pushedTx models.Transaction
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return sampleSyncResponse(), nil
		},
		pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			pushedTx = req.Transaction[0]
			return models.Response{ServerTimestamp: 2000}, nil
		},
	}
	runtime := newTestRuntime(mc)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"transaction_type": "income",
		"date":             "2024-03-10",
		"account_id":       "account-1",
		"amount":           float64(1000),
	}

	result, err := handleCreateTransaction(context.Background(), runtime, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}

	if pushedTx.Income != 1000 {
		t.Errorf("income Income = %v, want 1000", pushedTx.Income)
	}
	if pushedTx.Outcome != 0 {
		t.Errorf("income Outcome = %v, want 0", pushedTx.Outcome)
	}
}

func TestHandleCreateTransaction_Transfer(t *testing.T) {
	var pushedTx models.Transaction
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return sampleSyncResponse(), nil
		},
		pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			pushedTx = req.Transaction[0]
			return models.Response{ServerTimestamp: 2000}, nil
		},
	}
	runtime := newTestRuntime(mc)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"transaction_type": "transfer",
		"date":             "2024-03-10",
		"account_id":       "account-1",
		"amount":           float64(500),
		"to_account_id":    "account-2",
	}

	result, err := handleCreateTransaction(context.Background(), runtime, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}

	if pushedTx.Outcome != 500 {
		t.Errorf("transfer Outcome = %v, want 500", pushedTx.Outcome)
	}
	if pushedTx.Income != 500 {
		t.Errorf("transfer Income = %v, want 500", pushedTx.Income)
	}
	if *pushedTx.OutcomeAccount != "account-1" {
		t.Errorf("OutcomeAccount = %q, want account-1", *pushedTx.OutcomeAccount)
	}
	if pushedTx.IncomeAccount != "account-2" {
		t.Errorf("IncomeAccount = %q, want account-2", pushedTx.IncomeAccount)
	}
}

func TestHandleCreateTransaction_MissingAccountReturnsError(t *testing.T) {
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return sampleSyncResponse(), nil
		},
	}
	runtime := newTestRuntime(mc)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"transaction_type": "expense",
		"date":             "2024-03-10",
		"account_id":       "nonexistent-account",
		"amount":           float64(100),
	}

	result, err := handleCreateTransaction(context.Background(), runtime, req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected tool error for nonexistent account")
	}
}

func TestHandleDeleteTransaction_MarksDeleted(t *testing.T) {
	var pushedTxs []models.Transaction
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return sampleSyncResponse(), nil
		},
		pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			pushedTxs = req.Transaction
			return models.Response{ServerTimestamp: 2000}, nil
		},
	}
	runtime := newTestRuntime(mc)

	result, err := handleDeleteTransaction(context.Background(), runtime, "tx-1")
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}

	if len(pushedTxs) != 1 {
		t.Fatalf("expected 1 pushed deletion, got %d", len(pushedTxs))
	}
	if !pushedTxs[0].Deleted {
		t.Error("deleted transaction should have Deleted=true")
	}
	if pushedTxs[0].ID != "tx-1" {
		t.Errorf("wrong transaction ID: %s", pushedTxs[0].ID)
	}
}

func TestHandleListTransactions_TypeFilter(t *testing.T) {
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return sampleSyncResponse(), nil
		},
	}
	runtime := newTestRuntime(mc)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"transaction_type": "expense",
	}

	result, err := handleListTransactions(context.Background(), runtime, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}

	text := resultText(t, result)
	var page paginatedTransactions
	if err := json.Unmarshal([]byte(text), &page); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if page.Total != 1 {
		t.Errorf("expected 1 expense, got %d", page.Total)
	}
	if page.Items[0].Type != "expense" {
		t.Errorf("expected expense, got %s", page.Items[0].Type)
	}
}

func TestHandleListTransactions_QueryMatchesCommentOrPayee(t *testing.T) {
	instr1 := int32(1)
	acc1 := "account-1"
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return models.Response{
				ServerTimestamp: 1000,
				Instrument: []models.Instrument{
					{ID: 1, Symbol: "₽"},
				},
				Account: []models.Account{
					{ID: "account-1", Title: "Cash", Instrument: &instr1},
				},
				Transaction: []models.Transaction{
					{
						ID:                "tx-payee",
						Date:              "2024-01-15",
						Outcome:           100,
						IncomeAccount:     "account-1",
						OutcomeAccount:    &acc1,
						IncomeInstrument:  1,
						OutcomeInstrument: 1,
						Payee:             "Mdulo 14",
					},
					{
						ID:                "tx-comment",
						Date:              "2024-01-16",
						Outcome:           200,
						IncomeAccount:     "account-1",
						OutcomeAccount:    &acc1,
						IncomeInstrument:  1,
						OutcomeInstrument: 1,
						Comment:           strPtr("To Khrystina S"),
					},
				},
			}, nil
		},
	}
	runtime := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"query": "khrystina",
	})

	result, err := handleListTransactions(context.Background(), runtime, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}

	text := resultText(t, result)
	var page paginatedTransactions
	if err := json.Unmarshal([]byte(text), &page); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("expected 1 match, got %d", page.Total)
	}
	if page.Items[0].ID != "tx-comment" {
		t.Fatalf("matched ID = %q, want tx-comment", page.Items[0].ID)
	}
}

func TestHandleListTransactions_QueryAndPayeeAreCombined(t *testing.T) {
	instr1 := int32(1)
	acc1 := "account-1"
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return models.Response{
				ServerTimestamp: 1000,
				Instrument: []models.Instrument{
					{ID: 1, Symbol: "₽"},
				},
				Account: []models.Account{
					{ID: "account-1", Title: "Cash", Instrument: &instr1},
				},
				Transaction: []models.Transaction{
					{
						ID:                "tx-1",
						Date:              "2024-01-15",
						Outcome:           100,
						IncomeAccount:     "account-1",
						OutcomeAccount:    &acc1,
						IncomeInstrument:  1,
						OutcomeInstrument: 1,
						Payee:             "Alice",
						Comment:           strPtr("Birthday gift"),
					},
					{
						ID:                "tx-2",
						Date:              "2024-01-16",
						Outcome:           100,
						IncomeAccount:     "account-1",
						OutcomeAccount:    &acc1,
						IncomeInstrument:  1,
						OutcomeInstrument: 1,
						Payee:             "Bob",
						Comment:           strPtr("Birthday gift"),
					},
				},
			}, nil
		},
	}
	runtime := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"query": "gift",
		"payee": "alice",
	})

	result, err := handleListTransactions(context.Background(), runtime, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}

	text := resultText(t, result)
	var page paginatedTransactions
	if err := json.Unmarshal([]byte(text), &page); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("expected 1 combined-filter match, got %d", page.Total)
	}
	if page.Items[0].ID != "tx-1" {
		t.Fatalf("matched ID = %q, want tx-1", page.Items[0].ID)
	}
}

func TestHandleUpdateTransaction_UpdatesInstrumentsWhenAccountChanges(t *testing.T) {
	instr1 := int32(1)
	instr2 := int32(2)
	acc1 := "account-1"
	var pushedTx models.Transaction

	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return models.Response{
				ServerTimestamp: 1000,
				User:            []models.User{{ID: 42}},
				Instrument: []models.Instrument{
					{ID: 1, Symbol: "₽"},
					{ID: 2, Symbol: "$"},
				},
				Account: []models.Account{
					{ID: "account-1", Title: "Cash", Instrument: &instr1},
					{ID: "account-2", Title: "Card", Instrument: &instr2},
				},
				Transaction: []models.Transaction{
					{
						ID:                "tx-1",
						User:              42,
						Date:              "2024-01-15",
						IncomeAccount:     "account-1",
						OutcomeAccount:    &acc1,
						IncomeInstrument:  1,
						OutcomeInstrument: 1,
						Outcome:           500,
					},
				},
			}, nil
		},
		pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			pushedTx = req.Transaction[0]
			return models.Response{ServerTimestamp: 2000}, nil
		},
	}
	runtime := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"id":         "tx-1",
		"account_id": "account-2",
	})

	result, err := handleUpdateTransaction(context.Background(), runtime, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	if pushedTx.IncomeAccount != "account-2" {
		t.Fatalf("IncomeAccount = %q, want account-2", pushedTx.IncomeAccount)
	}
	if pushedTx.OutcomeAccount == nil || *pushedTx.OutcomeAccount != "account-2" {
		t.Fatalf("OutcomeAccount = %v, want account-2", pushedTx.OutcomeAccount)
	}
	if pushedTx.IncomeInstrument != 2 || pushedTx.OutcomeInstrument != 2 {
		t.Fatalf("instruments = %d/%d, want 2/2", pushedTx.IncomeInstrument, pushedTx.OutcomeInstrument)
	}
}
