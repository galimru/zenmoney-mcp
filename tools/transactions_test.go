package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

func workflowSyncResponse() models.Response {
	instr1 := int32(1)
	instr2 := int32(2)
	outcomeAcc := "account-1"
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
	runtime := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"type":       "expense",
		"date":       "2024-04-01",
		"amount":     float64(250),
		"account_id": "account-1",
		"category":   "Food",
		"payee":      "Bakery",
	})

	result, err := handleAddTransaction(context.Background(), runtime, req)
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
	runtime := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"transaction_id": "tx-existing-food",
		"type":           "income",
		"account_id":     "account-2",
		"amount":         float64(900),
		"clear_comment":  true,
	})

	result, err := handleEditTransaction(context.Background(), runtime, req)
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

func TestHandleCategorizeTransactions_AutoApplySuggest(t *testing.T) {
	var pushed []models.Transaction
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return workflowSyncResponse(), nil
		},
		suggestFn: func(ctx context.Context, tx models.Transaction) (models.Transaction, error) {
			tx.Tag = []string{"tag-food"}
			return tx, nil
		},
		pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			pushed = req.Transaction
			return models.Response{ServerTimestamp: 2000}, nil
		},
	}
	runtime := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"transaction_ids": "[\"tx-uncategorized\"]",
		"auto_apply":      true,
		"dry_run":         false,
	})

	result, err := handleCategorizeTransactions(context.Background(), runtime, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	if len(pushed) != 1 || len(pushed[0].Tag) != 1 || pushed[0].Tag[0] != "tag-food" {
		t.Fatalf("pushed tags = %#v, want tag-food", pushed)
	}

	var out categorizeResponse
	if err := json.Unmarshal([]byte(resultText(t, result)), &out); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if out.Applied != 1 || out.NeedsReview != 0 {
		t.Fatalf("response counts = %+v, want applied=1 needs_review=0", out)
	}
}

func TestHandlePreviewTransactionImport_ClassifiesDuplicateAndStoresPlan(t *testing.T) {
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return workflowSyncResponse(), nil
		},
		suggestFn: func(ctx context.Context, tx models.Transaction) (models.Transaction, error) {
			tx.Tag = []string{"tag-food"}
			return tx, nil
		},
	}
	runtime := newTestRuntime(mc)

	items := []map[string]any{
		{
			"date":       "2024-01-15",
			"amount":     500.0,
			"type":       "expense",
			"account_id": "account-1",
			"payee":      "McDonalds",
		},
		{
			"date":       "2024-04-02",
			"amount":     120.0,
			"type":       "expense",
			"account_id": "account-1",
			"payee":      "Coffee Shop",
		},
	}
	raw, _ := json.Marshal(items)

	req := mcpReqWithArgs(map[string]any{
		"items": string(raw),
	})

	result, err := handlePreviewTransactionImport(context.Background(), runtime, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}

	var out importPreviewResponse
	if err := json.Unmarshal([]byte(resultText(t, result)), &out); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if out.ExactDuplicates != 1 || out.ReadyToImport != 1 {
		t.Fatalf("preview counts = %+v, want exact_duplicates=1 ready_to_import=1", out)
	}
	if runtime.takeImportPlan(out.ImportPlanID) == nil {
		t.Fatal("expected import plan to be stored")
	}
}

func TestHandleAddTransaction_RejectsAccountTitlesForWrites(t *testing.T) {
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return workflowSyncResponse(), nil
		},
	}
	runtime := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"type":    "expense",
		"date":    "2024-04-01",
		"amount":  float64(250),
		"account": "Cash",
	})

	result, err := handleAddTransaction(context.Background(), runtime, req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error for account title in write operation")
	}
	if !contains(resultText(t, result), "account_id") {
		t.Fatalf("expected account_id guidance, got: %s", resultText(t, result))
	}
}

func TestHandleCommitTransactionImport_CommitsOnlyReadyRows(t *testing.T) {
	var pushed []models.Transaction
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return workflowSyncResponse(), nil
		},
		pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			pushed = append(pushed, req.Transaction...)
			return models.Response{ServerTimestamp: 2000}, nil
		},
	}
	runtime := newTestRuntime(mc)

	runtime.storeImportPlan("plan-1", &PreparedImportPlan{
		Rows: []plannedImportRow{
			{
				Preview: importPreviewRow{Index: 0, Status: "new"},
				Tx: &models.Transaction{
					ID:                "new-tx",
					User:              42,
					Date:              "2024-04-10",
					Outcome:           100,
					IncomeAccount:     "account-1",
					OutcomeAccount:    strPtr("account-1"),
					IncomeInstrument:  1,
					OutcomeInstrument: 1,
				},
			},
			{
				Preview: importPreviewRow{Index: 1, Status: "exact_duplicate"},
			},
		},
		ReadyToImport:   1,
		ExactDuplicates: 1,
	})

	result, err := handleCommitTransactionImport(context.Background(), runtime, "plan-1")
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	if len(pushed) != 1 || pushed[0].ID != "new-tx" {
		t.Fatalf("pushed = %#v, want one new transaction", pushed)
	}

	var out importCommitResponse
	if err := json.Unmarshal([]byte(resultText(t, result)), &out); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if out.Created != 1 || out.SkippedExact != 1 {
		t.Fatalf("commit response = %+v, want created=1 skipped_exact=1", out)
	}
}
