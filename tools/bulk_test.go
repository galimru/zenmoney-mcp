package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

func bulkSyncResponse() models.Response {
	instr1 := int32(1)
	acc1 := "account-1"
	return models.Response{
		ServerTimestamp: 1000,
		User:            []models.User{{ID: 42}},
		Instrument:      []models.Instrument{{ID: 1, Symbol: "₽"}},
		Account: []models.Account{
			{ID: "account-1", Title: "Cash", Instrument: &instr1},
		},
		Transaction: []models.Transaction{
			{
				ID:                "tx-existing",
				User:              42,
				Date:              "2024-01-01",
				Outcome:           100,
				IncomeAccount:     "account-1",
				OutcomeAccount:    &acc1,
				IncomeInstrument:  1,
				OutcomeInstrument: 1,
			},
		},
	}
}

func TestPrepareBulk_TooManyOperationsReturnsError(t *testing.T) {
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return bulkSyncResponse(), nil
		},
	}
	runtime := newTestRuntime(mc)

	// Build 25 operations exceeding the default limit of 20.
	ops := make([]map[string]any, 25)
	for i := range ops {
		ops[i] = map[string]any{"operation": "delete", "id": "tx-existing"}
	}
	opsJSON, _ := json.Marshal(ops)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"operations": string(opsJSON),
	}

	result, err := handlePrepareBulk(context.Background(), runtime, req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected tool error for too many operations")
	}
}

func TestPrepareBulk_PrepareAndExecuteRoundTrip(t *testing.T) {
	var pushedTxs []models.Transaction
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return bulkSyncResponse(), nil
		},
		pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			pushedTxs = req.Transaction
			return models.Response{ServerTimestamp: 2000}, nil
		},
	}
	runtime := newTestRuntime(mc)

	// Prepare a single create operation.
	ops := []map[string]any{
		{
			"operation":        "create",
			"transaction_type": "expense",
			"date":             "2024-03-15",
			"account_id":       "account-1",
			"amount":           float64(250),
			"payee":            "Supermarket",
		},
	}
	opsJSON, _ := json.Marshal(ops)

	prepReq := mcp.CallToolRequest{}
	prepReq.Params.Arguments = map[string]any{"operations": string(opsJSON)}

	prepResult, err := handlePrepareBulk(context.Background(), runtime, prepReq)
	if err != nil || prepResult.IsError {
		t.Fatalf("prepare failed: %v / %v", err, prepResult)
	}

	text := resultText(t, prepResult)
	var prepResp prepareResponse
	if err := json.Unmarshal([]byte(text), &prepResp); err != nil {
		t.Fatalf("parse prepare response: %v", err)
	}
	if prepResp.PreparationID == "" {
		t.Fatal("expected non-empty preparation_id")
	}
	if prepResp.Created != 1 {
		t.Errorf("expected created=1, got %d", prepResp.Created)
	}

	// Execute the preparation.
	execResult, err := handleExecuteBulk(context.Background(), runtime, prepResp.PreparationID)
	if err != nil || execResult.IsError {
		t.Fatalf("execute failed: %v / %v", err, execResult)
	}

	if len(pushedTxs) != 1 {
		t.Fatalf("expected 1 pushed transaction, got %d", len(pushedTxs))
	}
	if pushedTxs[0].Outcome != 250 {
		t.Errorf("pushed Outcome = %v, want 250", pushedTxs[0].Outcome)
	}
}

func TestExecuteBulk_UnknownPreparationIDReturnsError(t *testing.T) {
	runtime := newTestRuntime(&mockZenClient{})

	result, err := handleExecuteBulk(context.Background(), runtime, "nonexistent-id")
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected tool error for unknown preparation_id")
	}
}

func TestPrepareBulk_ExecuteConsumesPreparedSet(t *testing.T) {
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return bulkSyncResponse(), nil
		},
		pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			return models.Response{ServerTimestamp: 2000}, nil
		},
	}
	runtime := newTestRuntime(mc)

	ops := []map[string]any{
		{
			"operation":        "create",
			"transaction_type": "expense",
			"date":             "2024-03-15",
			"account_id":       "account-1",
			"amount":           float64(100),
		},
	}
	opsJSON, _ := json.Marshal(ops)

	prepReq := mcp.CallToolRequest{}
	prepReq.Params.Arguments = map[string]any{"operations": string(opsJSON)}

	prepResult, _ := handlePrepareBulk(context.Background(), runtime, prepReq)
	text := resultText(t, prepResult)
	var prepResp prepareResponse
	json.Unmarshal([]byte(text), &prepResp)

	// Execute once — should succeed.
	result1, _ := handleExecuteBulk(context.Background(), runtime, prepResp.PreparationID)
	if result1.IsError {
		t.Fatal("first execute should succeed")
	}

	// Execute again — should fail (preparation consumed).
	result2, _ := handleExecuteBulk(context.Background(), runtime, prepResp.PreparationID)
	if !result2.IsError {
		t.Error("second execute should fail with not-found error")
	}
}

func TestPrepareBulk_UpdateRecomputesTransferInstruments(t *testing.T) {
	instr1 := int32(1)
	instr2 := int32(2)
	outcomeAcc := "account-1"
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
					{ID: "account-2", Title: "USD", Instrument: &instr2},
				},
				Transaction: []models.Transaction{
					{
						ID:                "tx-existing",
						User:              42,
						Date:              "2024-01-01",
						Outcome:           100,
						Income:            100,
						IncomeAccount:     "account-2",
						OutcomeAccount:    &outcomeAcc,
						IncomeInstrument:  1,
						OutcomeInstrument: 1,
					},
				},
			}, nil
		},
	}
	runtime := newTestRuntime(mc)

	ops := []map[string]any{
		{
			"operation":     "update",
			"id":            "tx-existing",
			"to_account_id": "account-2",
		},
	}
	opsJSON, _ := json.Marshal(ops)
	req := mcpReqWithArgs(map[string]any{"operations": string(opsJSON)})

	result, err := handlePrepareBulk(context.Background(), runtime, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}

	text := resultText(t, result)
	var prepResp prepareResponse
	if err := json.Unmarshal([]byte(text), &prepResp); err != nil {
		t.Fatalf("parse prepare response: %v", err)
	}
	bulk := runtime.takePreparation(prepResp.PreparationID)
	if bulk == nil || len(bulk.ToPush) != 1 {
		t.Fatal("expected prepared bulk item")
	}
	tx := bulk.ToPush[0].tx
	if tx.IncomeAccount != "account-2" || tx.IncomeInstrument != 2 {
		t.Fatalf("income side = %q/%d, want account-2/2", tx.IncomeAccount, tx.IncomeInstrument)
	}
}
