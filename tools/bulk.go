package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
	"github.com/galimru/zenmoney-mcp/store"
)

// RegisterBulkTools adds prepare_bulk_operations and execute_bulk_operations to the MCP server.
func RegisterBulkTools(s *server.MCPServer, runtime *RuntimeProvider) {
	s.AddTool(
		mcp.NewTool("prepare_bulk_operations",
			mcp.WithDescription(`Validate and preview multiple transaction operations (create, update, delete) without committing them. Returns an enriched preview of all changes and a preparation_id. Pass the preparation_id to execute_bulk_operations to commit. Limit to 20 operations per call; split larger batches.

Each operation in the "operations" array must have an "operation" field: "create", "update", or "delete".

Create fields: transaction_type (required), date (required), account_id (required), amount (required), to_account_id, to_amount, instrument_id, to_instrument_id, tag_ids (JSON array string), payee, comment.
Update fields: id (required), date, amount, to_amount, account_id, to_account_id, tag_ids (JSON array string), payee, comment.
Delete fields: id (required).

Example:
{
  "operations": [
    {"operation":"create","transaction_type":"expense","date":"2024-01-15","account_id":"uuid","amount":500,"payee":"McDonalds","tag_ids":"[\"food-uuid\"]"},
    {"operation":"delete","id":"tx-uuid"}
  ]
}`),
			mcp.WithString("operations",
				mcp.Required(),
				mcp.Description("JSON array of operations. Each must have an \"operation\" field: create, update, or delete."),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handlePrepareBulk(ctx, runtime, req)
		},
	)

	s.AddTool(
		mcp.NewTool("execute_bulk_operations",
			mcp.WithDescription("Execute a previously prepared bulk operation. Commits the validated changes to ZenMoney and returns a summary."),
			mcp.WithString("preparation_id",
				mcp.Required(),
				mcp.Description("The preparation_id returned by prepare_bulk_operations"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			prepID := req.GetString("preparation_id", "")
			return handleExecuteBulk(ctx, runtime, prepID)
		},
	)
}

// bulkTxItem holds a prepared transaction for bulk execution.
type bulkTxItem struct {
	tx      models.Transaction
	isNew   bool // true=create, false=update
	preview transactionResult
}

// bulkPrep holds a prepared bulk operation set.
type bulkPrep struct {
	txItems  []bulkTxItem
	toDelete []deletePair
	created  int
	updated  int
	deleted  int
}

// deletePair holds the ID and preview of a transaction to be deleted.
type deletePair struct {
	id      string
	userID  int
	preview transactionResult
	tx      models.Transaction // original transaction for required fields
}

type prepareResponse struct {
	PreparationID        string              `json:"preparation_id"`
	Created              int                 `json:"created"`
	Updated              int                 `json:"updated"`
	Deleted              int                 `json:"deleted"`
	Transactions         []transactionResult `json:"transactions"`
	DeletedTransactions  []transactionResult `json:"deleted_transactions"`
}

type executeResponse struct {
	Created             int                 `json:"created"`
	Updated             int                 `json:"updated"`
	Deleted             int                 `json:"deleted"`
	Transactions        []transactionResult `json:"transactions"`
	DeletedTransactions []transactionResult `json:"deleted_transactions"`
}

func handlePrepareBulk(ctx context.Context, runtime *RuntimeProvider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	cfg, err := runtime.config()
	if err != nil {
		return runtimeError(err), nil
	}

	// Parse operations JSON array.
	opsRaw := req.GetString("operations", "")
	if opsRaw == "" {
		return mcp.NewToolResultError("operations is required"), nil
	}

	var rawOps []json.RawMessage
	if err := json.Unmarshal([]byte(opsRaw), &rawOps); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid operations JSON: %v", err)), nil
	}

	if len(rawOps) == 0 {
		return mcp.NewToolResultError("operations array is empty"), nil
	}
	if len(rawOps) > cfg.MaxBulkOperations {
		return mcp.NewToolResultError(fmt.Sprintf("too many operations: %d (max %d)", len(rawOps), cfg.MaxBulkOperations)), nil
	}

	resp, maps, err := fetchSyncResponse(ctx, c, runtime.zenStore)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	userID := 0
	if len(resp.User) > 0 {
		userID = resp.User[0].ID
	}

	// Index existing transactions by ID for update/delete lookup.
	txByID := make(map[string]models.Transaction, len(resp.Transaction))
	for _, tx := range resp.Transaction {
		txByID[tx.ID] = tx
	}

	prep := &bulkPrep{}

	for i, rawOp := range rawOps {
		var disc struct {
			Operation string `json:"operation"`
		}
		if err := json.Unmarshal(rawOp, &disc); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("operation[%d]: invalid JSON: %v", i, err)), nil
		}

		switch disc.Operation {
		case "create":
			tx, err := buildTransactionFromRaw(rawOp, maps, userID)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("operation[%d] create: %v", i, err)), nil
			}
			prep.txItems = append(prep.txItems, bulkTxItem{tx: tx, isNew: true, preview: shapeTransaction(tx, maps)})
			prep.created++

		case "update":
			var p struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(rawOp, &p); err != nil || p.ID == "" {
				return mcp.NewToolResultError(fmt.Sprintf("operation[%d] update: id is required", i)), nil
			}
			existing, ok := txByID[p.ID]
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("operation[%d] update: transaction %q not found", i, p.ID)), nil
			}
			updated, err := applyUpdateFromRaw(rawOp, existing, maps)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("operation[%d] update: %v", i, err)), nil
			}
			prep.txItems = append(prep.txItems, bulkTxItem{tx: updated, isNew: false, preview: shapeTransaction(updated, maps)})
			prep.updated++

		case "delete":
			var p struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(rawOp, &p); err != nil || p.ID == "" {
				return mcp.NewToolResultError(fmt.Sprintf("operation[%d] delete: id is required", i)), nil
			}
			existing, ok := txByID[p.ID]
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("operation[%d] delete: transaction %q not found", i, p.ID)), nil
			}
			prep.toDelete = append(prep.toDelete, deletePair{
				id:      p.ID,
				userID:  userID,
				preview: shapeTransaction(existing, maps),
				tx:      existing,
			})
			prep.deleted++

		default:
			return mcp.NewToolResultError(fmt.Sprintf("operation[%d]: unknown operation %q (use create, update, or delete)", i, disc.Operation)), nil
		}
	}

	prepID := uuid.New().String()
	runtime.storePreparation(prepID, &PreparedBulk{
		ToPush:  prep.txItems,
		ToDelete: prep.toDelete,
		Created: prep.created,
		Updated: prep.updated,
		Deleted: prep.deleted,
	})

	txPreviews := make([]transactionResult, len(prep.txItems))
	for i, item := range prep.txItems {
		txPreviews[i] = item.preview
	}

	delPreviews := make([]transactionResult, len(prep.toDelete))
	for i, d := range prep.toDelete {
		delPreviews[i] = d.preview
	}

	out, err := structJSON(prepareResponse{
		PreparationID:       prepID,
		Created:             prep.created,
		Updated:             prep.updated,
		Deleted:             prep.deleted,
		Transactions:        txPreviews,
		DeletedTransactions: delPreviews,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return out, nil
}

func handleExecuteBulk(ctx context.Context, runtime *RuntimeProvider, prepID string) (*mcp.CallToolResult, error) {
	if prepID == "" {
		return mcp.NewToolResultError("preparation_id is required"), nil
	}

	bulk := runtime.takePreparation(prepID)
	if bulk == nil {
		return mcp.NewToolResultError(fmt.Sprintf("preparation %q not found or already executed", prepID)), nil
	}

	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	serverTS := currentServerTimestamp(runtime.zenStore)
	var txResults []transactionResult
	var delResults []transactionResult

	// Push creates and updates.
	if len(bulk.ToPush) > 0 {
		txs := make([]models.Transaction, len(bulk.ToPush))
		for i, item := range bulk.ToPush {
			txs[i] = item.tx
		}
		pushResp, err := c.Push(ctx, pushRequest(serverTS, txs))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("push transactions: %v", err)), nil
		}
		if pushResp.ServerTimestamp > 0 {
			serverTS = pushResp.ServerTimestamp
			_ = runtime.zenStore.Save(&store.SyncState{ServerTimestamp: serverTS, LastSyncAt: time.Now()})
		}
		for _, item := range bulk.ToPush {
			txResults = append(txResults, item.preview)
		}
	}

	// Push deletions.
	if len(bulk.ToDelete) > 0 {
		now := int(time.Now().Unix())
		delTxs := make([]models.Transaction, len(bulk.ToDelete))
		for i, d := range bulk.ToDelete {
			delTxs[i] = models.Transaction{
				ID:                d.tx.ID,
				User:              d.userID,
				Changed:           now,
				Deleted:           true,
				IncomeAccount:     d.tx.IncomeAccount,
				OutcomeAccount:    d.tx.OutcomeAccount,
				IncomeInstrument:  d.tx.IncomeInstrument,
				OutcomeInstrument: d.tx.OutcomeInstrument,
				Date:              d.tx.Date,
			}
			delResults = append(delResults, d.preview)
		}
		pushResp, err := c.Push(ctx, pushRequest(serverTS, delTxs))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("delete transactions: %v", err)), nil
		}
		if pushResp.ServerTimestamp > 0 {
			_ = runtime.zenStore.Save(&store.SyncState{ServerTimestamp: pushResp.ServerTimestamp, LastSyncAt: time.Now()})
		}
	}

	if txResults == nil {
		txResults = []transactionResult{}
	}
	if delResults == nil {
		delResults = []transactionResult{}
	}

	out, err := structJSON(executeResponse{
		Created:             bulk.Created,
		Updated:             bulk.Updated,
		Deleted:             bulk.Deleted,
		Transactions:        txResults,
		DeletedTransactions: delResults,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return out, nil
}

// buildTransactionFromRaw parses a create operation from raw JSON.
func buildTransactionFromRaw(raw json.RawMessage, maps LookupMaps, userID int) (models.Transaction, error) {
	var p struct {
		TransactionType string  `json:"transaction_type"`
		Date            string  `json:"date"`
		AccountID       string  `json:"account_id"`
		Amount          float64 `json:"amount"`
		ToAccountID     string  `json:"to_account_id"`
		ToAmount        float64 `json:"to_amount"`
		InstrumentID    int     `json:"instrument_id"`
		ToInstrumentID  int     `json:"to_instrument_id"`
		TagIDs          string  `json:"tag_ids"`
		Payee           string  `json:"payee"`
		Comment         string  `json:"comment"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return models.Transaction{}, fmt.Errorf("parse create params: %w", err)
	}

	if p.TransactionType == "" {
		return models.Transaction{}, fmt.Errorf("transaction_type is required")
	}
	if p.Date == "" {
		return models.Transaction{}, fmt.Errorf("date is required")
	}
	if p.AccountID == "" {
		return models.Transaction{}, fmt.Errorf("account_id is required")
	}
	if p.Amount <= 0 {
		return models.Transaction{}, fmt.Errorf("amount must be positive")
	}
	if _, ok := maps.Accounts[p.AccountID]; !ok {
		return models.Transaction{}, fmt.Errorf("account %q not found", p.AccountID)
	}

	instrID, _ := maps.AccountInstrument(p.AccountID)
	if p.InstrumentID > 0 {
		instrID = p.InstrumentID
	}
	toInstrID := instrID
	if p.ToAccountID != "" {
		if id, ok := maps.AccountInstrument(p.ToAccountID); ok {
			toInstrID = id
		}
	}
	if p.ToInstrumentID > 0 {
		toInstrID = p.ToInstrumentID
	}

	toAmount := p.ToAmount
	if toAmount <= 0 {
		toAmount = p.Amount
	}

	var tagIDs []string
	if p.TagIDs != "" {
		if err := json.Unmarshal([]byte(p.TagIDs), &tagIDs); err != nil {
			return models.Transaction{}, fmt.Errorf("invalid tag_ids: %w", err)
		}
	}

	now := int(time.Now().Unix())
	tx := models.Transaction{
		ID:      uuid.New().String(),
		User:    userID,
		Date:    p.Date,
		Changed: now,
		Created: now,
		Tag:     tagIDs,
		Payee:   p.Payee,
		Comment: strPtr(p.Comment),
	}

	switch p.TransactionType {
	case "expense":
		tx.Outcome = p.Amount
		tx.IncomeAccount = p.AccountID
		tx.OutcomeAccount = &p.AccountID
		tx.IncomeInstrument = instrID
		tx.OutcomeInstrument = instrID
	case "income":
		tx.Income = p.Amount
		tx.IncomeAccount = p.AccountID
		tx.OutcomeAccount = &p.AccountID
		tx.IncomeInstrument = instrID
		tx.OutcomeInstrument = instrID
	case "transfer":
		if p.ToAccountID == "" {
			return models.Transaction{}, fmt.Errorf("to_account_id is required for transfers")
		}
		if _, ok := maps.Accounts[p.ToAccountID]; !ok {
			return models.Transaction{}, fmt.Errorf("to_account %q not found", p.ToAccountID)
		}
		tx.Outcome = p.Amount
		tx.Income = toAmount
		tx.OutcomeAccount = &p.AccountID
		tx.IncomeAccount = p.ToAccountID
		tx.OutcomeInstrument = instrID
		tx.IncomeInstrument = toInstrID
	default:
		return models.Transaction{}, fmt.Errorf("unknown transaction_type %q", p.TransactionType)
	}

	return tx, nil
}

// applyUpdateFromRaw applies partial update fields from raw JSON onto an existing transaction.
func applyUpdateFromRaw(raw json.RawMessage, existing models.Transaction, maps LookupMaps) (models.Transaction, error) {
	var p struct {
		Date        string  `json:"date"`
		Amount      float64 `json:"amount"`
		ToAmount    float64 `json:"to_amount"`
		AccountID   string  `json:"account_id"`
		ToAccountID string  `json:"to_account_id"`
		TagIDs      string  `json:"tag_ids"`
		Payee       *string `json:"payee"`
		Comment     *string `json:"comment"`
	}
	// Use a map to detect which fields were explicitly provided.
	var raw2 map[string]json.RawMessage
	if err := json.Unmarshal(raw, &raw2); err != nil {
		return models.Transaction{}, fmt.Errorf("parse update params: %w", err)
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return models.Transaction{}, fmt.Errorf("parse update params: %w", err)
	}

	tx := existing
	tx.Changed = int(time.Now().Unix())

	if _, ok := raw2["date"]; ok && p.Date != "" {
		tx.Date = p.Date
	}

	txType := classifyTx(tx)

	if _, ok := raw2["amount"]; ok && p.Amount > 0 {
		switch txType {
		case "expense":
			tx.Outcome = p.Amount
		case "income":
			tx.Income = p.Amount
		case "transfer":
			tx.Outcome = p.Amount
		}
	}

	if _, ok := raw2["to_amount"]; ok && p.ToAmount > 0 && txType == "transfer" {
		tx.Income = p.ToAmount
	}

	if _, ok := raw2["account_id"]; ok && p.AccountID != "" {
		if _, exists := maps.Accounts[p.AccountID]; !exists {
			return models.Transaction{}, fmt.Errorf("account %q not found", p.AccountID)
		}
		switch txType {
		case "expense", "income":
			tx.IncomeAccount = p.AccountID
			tx.OutcomeAccount = &p.AccountID
		case "transfer":
			tx.OutcomeAccount = &p.AccountID
		}
	}

	if _, ok := raw2["to_account_id"]; ok && p.ToAccountID != "" && txType == "transfer" {
		if _, exists := maps.Accounts[p.ToAccountID]; !exists {
			return models.Transaction{}, fmt.Errorf("to_account %q not found", p.ToAccountID)
		}
		tx.IncomeAccount = p.ToAccountID
	}

	if _, ok := raw2["tag_ids"]; ok && p.TagIDs != "" {
		var tagIDs []string
		if err := json.Unmarshal([]byte(p.TagIDs), &tagIDs); err != nil {
			return models.Transaction{}, fmt.Errorf("invalid tag_ids: %w", err)
		}
		tx.Tag = tagIDs
	}

	if _, ok := raw2["payee"]; ok && p.Payee != nil {
		tx.Payee = *p.Payee
	}

	if _, ok := raw2["comment"]; ok && p.Comment != nil {
		tx.Comment = p.Comment
	}

	return tx, nil
}

// PreparedBulk is redefined here to use concrete types (avoiding the interface{} in runtime_provider.go).
// The runtime_provider storePreparation/takePreparation use this type.
type PreparedBulk struct {
	ToPush   []bulkTxItem
	ToDelete []deletePair
	Created  int
	Updated  int
	Deleted  int
}
