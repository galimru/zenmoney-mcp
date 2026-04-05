package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/galimru/zenmoney-mcp/internal/transactions"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

func workflowSyncResponse() models.Response {
	instr1 := int32(1)
	instr2 := int32(2)
	outcomeAcc := "account-1"
	transferOutcomeAcc := "account-1"
	return models.Response{
		ServerTimestamp: 1000,
		User:            []models.User{{ID: 42}},
		Instrument: []models.Instrument{
			{ID: 1, Symbol: "₽", ShortTitle: "RUB", Title: "Russian Ruble"},
			{ID: 2, Symbol: "$", ShortTitle: "USD", Title: "US Dollar"},
		},
		Account: []models.Account{
			{ID: "account-1", Title: "Cash", Instrument: &instr1},
			{ID: "account-2", Title: "Card", Instrument: &instr2},
		},
		Tag: []models.Tag{
			{ID: "tag-food", Title: "Food"},
			{ID: "tag-salary", Title: "Salary"},
		},
		Transaction: []models.Transaction{
			{
				ID:                "tx-existing-food",
				User:              42,
				Date:              "2024-01-15",
				Outcome:           500,
				IncomeAccount:     "account-1",
				OutcomeAccount:    &outcomeAcc,
				IncomeInstrument:  1,
				OutcomeInstrument: 1,
				Tag:               []string{"tag-food"},
				Payee:             "McDonalds",
			},
			{
				ID:                "tx-uncategorized",
				User:              42,
				Date:              "2024-01-20",
				Outcome:           300,
				IncomeAccount:     "account-1",
				OutcomeAccount:    &outcomeAcc,
				IncomeInstrument:  1,
				OutcomeInstrument: 1,
				Payee:             "Coffee Shop",
			},
			{
				ID:                "tx-uncategorized-transfer",
				User:              42,
				Date:              "2024-01-21",
				Income:            50,
				Outcome:           50,
				IncomeAccount:     "account-2",
				OutcomeAccount:    &transferOutcomeAcc,
				IncomeInstrument:  2,
				OutcomeInstrument: 1,
				Payee:             "Transfer",
			},
		},
	}
}

func TestHandleAddTransaction_ResolvesAccountAndCategoryByTitle(t *testing.T) {
	var pushed models.Transaction
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return workflowSyncResponse(), nil
		},
		pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			pushed = req.Transaction[0]
			return models.Response{ServerTimestamp: 2000}, nil
		},
	}
	p := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"type":       "expense",
		"date":       "2024-04-01",
		"amount":     float64(250),
		"account_id": "account-1",
		"category":   "Food",
		"payee":      "Bakery",
	})

	result, err := handleAddTransaction(context.Background(), p, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	if pushed.IncomeAccount != "account-1" {
		t.Fatalf("IncomeAccount = %q, want account-1", pushed.IncomeAccount)
	}
	if len(pushed.Tag) != 1 || pushed.Tag[0] != "tag-food" {
		t.Fatalf("Tag = %v, want [tag-food]", pushed.Tag)
	}
}

func TestHandleEditTransaction_ClearsCommentAndChangesType(t *testing.T) {
	var pushed models.Transaction
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			resp := workflowSyncResponse()
			comment := "old note"
			resp.Transaction[0].Comment = &comment
			return resp, nil
		},
		pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			pushed = req.Transaction[0]
			return models.Response{ServerTimestamp: 2000}, nil
		},
	}
	p := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"transaction_id": "tx-existing-food",
		"type":           "income",
		"account_id":     "account-2",
		"amount":         float64(900),
		"clear_comment":  true,
	})

	result, err := handleEditTransaction(context.Background(), p, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	if pushed.Income != 900 || pushed.Outcome != 0 {
		t.Fatalf("updated amounts = income:%v outcome:%v, want income 900 outcome 0", pushed.Income, pushed.Outcome)
	}
	if pushed.IncomeAccount != "account-2" {
		t.Fatalf("IncomeAccount = %q, want account-2", pushed.IncomeAccount)
	}
	if pushed.Comment != nil {
		t.Fatal("expected comment to be cleared")
	}
}

func TestHandleEditTransaction_UpdatesPayeeAndComment(t *testing.T) {
	var pushed models.Transaction
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			resp := workflowSyncResponse()
			comment := "old note"
			resp.Transaction[0].Comment = &comment
			return resp, nil
		},
		pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			pushed = req.Transaction[0]
			return models.Response{ServerTimestamp: 2000}, nil
		},
	}
	p := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"transaction_id": "tx-existing-food",
		"payee":          "New Bakery",
		"comment":        "fresh note",
	})

	result, err := handleEditTransaction(context.Background(), p, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	if pushed.Payee != "New Bakery" {
		t.Fatalf("Payee = %q, want New Bakery", pushed.Payee)
	}
	if pushed.Comment == nil || *pushed.Comment != "fresh note" {
		t.Fatalf("Comment = %#v, want fresh note", pushed.Comment)
	}
}

func TestHandleSuggestTransactionCategories_ReturnsSuggestions(t *testing.T) {
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return workflowSyncResponse(), nil
		},
		suggestFn: func(ctx context.Context, tx models.Transaction) (models.Transaction, error) {
			tx.Tag = []string{"tag-food"}
			return tx, nil
		},
	}
	p := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"transaction_ids": "[\"tx-uncategorized\"]",
	})

	result, err := handleSuggestTransactionCategories(context.Background(), p, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}

	var out transactions.SuggestCategoriesResponse
	if err := json.Unmarshal([]byte(resultText(t, result)), &out); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if out.Ready != 1 || out.NeedsReview != 0 || out.Skipped != 0 {
		t.Fatalf("response counts = %+v, want ready=1 needs_review=0 skipped=0", out)
	}
	if len(out.Items) != 1 {
		t.Fatalf("items = %+v, want one item", out.Items)
	}
	if out.Items[0].Transaction.ID != "tx-uncategorized" {
		t.Fatalf("transaction id = %q, want tx-uncategorized", out.Items[0].Transaction.ID)
	}
	if len(out.Items[0].SuggestedCategories) != 1 || out.Items[0].SuggestedCategories[0] != "Food" {
		t.Fatalf("suggested categories = %#v, want [Food]", out.Items[0].SuggestedCategories)
	}
	if out.Items[0].Status != "ready" {
		t.Fatalf("status = %q, want ready", out.Items[0].Status)
	}
}

func TestHandleListUncategorizedTransactions_ExcludesTransfers(t *testing.T) {
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return workflowSyncResponse(), nil
		},
	}
	p := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{})

	result, err := handleListUncategorizedTransactions(context.Background(), p, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}

	var out transactions.PaginatedTransactions
	if err := json.Unmarshal([]byte(resultText(t, result)), &out); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if out.Total != 1 || len(out.Items) != 1 {
		t.Fatalf("response = %+v, want one uncategorized non-transfer", out)
	}
	if out.Items[0].ID != "tx-uncategorized" {
		t.Fatalf("returned %q, want tx-uncategorized", out.Items[0].ID)
	}
}
