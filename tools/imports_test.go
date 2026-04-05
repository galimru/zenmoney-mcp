package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/galimru/zenmoney-mcp/internal/transactions"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

func TestHandleImportTransactions_Success(t *testing.T) {
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
	p := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"account_id": "account-1",
		"rows": []map[string]any{
			{
				"date":   "2024-04-02",
				"amount": 120.0,
				"type":   "expense",
				"payee":  "Coffee Shop",
			},
		},
	})

	result, err := handleImportTransactions(context.Background(), p, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}

	var out transactions.ImportTransactionsResponse
	if err := json.Unmarshal([]byte(resultText(t, result)), &out); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if !out.Imported {
		t.Fatalf("expected Imported=true, got message: %s", out.Message)
	}
	if len(pushed) != 1 {
		t.Fatalf("pushed = %#v, want one transaction", pushed)
	}
}

func TestHandleImportTransactions_PossibleDuplicatesPass(t *testing.T) {
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
	p := newTestRuntime(mc)

	// Same payee and amount as existing tx-existing-food but different date — possible duplicate, should still import
	req := mcpReqWithArgs(map[string]any{
		"account_id": "account-1",
		"rows": []map[string]any{
			{
				"date":   "2024-01-16",
				"amount": 500.0,
				"type":   "expense",
				"payee":  "McDonalds",
			},
		},
	})

	result, err := handleImportTransactions(context.Background(), p, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}

	var out transactions.ImportTransactionsResponse
	if err := json.Unmarshal([]byte(resultText(t, result)), &out); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if !out.Imported {
		t.Fatalf("expected possible duplicate to pass, got: %s", out.Message)
	}
	if len(pushed) != 1 {
		t.Fatalf("pushed = %#v, want one transaction", pushed)
	}
}

func TestHandleImportTransactions_BlockedOnInvalidRows(t *testing.T) {
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
	p := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"account_id": "account-1",
		"rows": []map[string]any{
			// exact duplicate of tx-existing-food in workflowSyncResponse
			{
				"date":   "2024-01-15",
				"amount": 500.0,
				"type":   "expense",
				"payee":  "McDonalds",
			},
			// invalid date
			{
				"date":   "2024-02-30",
				"amount": 120.0,
				"type":   "expense",
			},
		},
	})

	result, err := handleImportTransactions(context.Background(), p, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}

	var out transactions.ImportTransactionsResponse
	if err := json.Unmarshal([]byte(resultText(t, result)), &out); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if out.Imported {
		t.Fatalf("expected import to be blocked, got: %s", out.Message)
	}
	if !contains(out.Message, "Row 0") || !contains(out.Message, "Row 1") {
		t.Fatalf("expected both rows mentioned in message, got: %s", out.Message)
	}
	if len(out.Rows) != 2 {
		t.Fatalf("expected 2 invalid rows, got %d: %+v", len(out.Rows), out.Rows)
	}
	if len(pushed) != 0 {
		t.Fatalf("pushed = %#v, want no imports", pushed)
	}
}
