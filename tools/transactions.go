package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
	"github.com/galimru/zenmoney-mcp/store"
)

// RegisterTransactionTools adds list_transactions, create_transaction, update_transaction,
// and delete_transaction to the MCP server.
func RegisterTransactionTools(s *server.MCPServer, runtime *RuntimeProvider) {
	s.AddTool(
		mcp.NewTool("list_transactions",
			mcp.WithDescription("List transactions with optional filters. Returns {items, total, offset, limit}."),
			mcp.WithString("date_from", mcp.Description("Start date inclusive (YYYY-MM-DD)")),
			mcp.WithString("date_to", mcp.Description("End date inclusive (YYYY-MM-DD)")),
			mcp.WithString("account_id", mcp.Description("Filter by account ID")),
			mcp.WithString("tag_id", mcp.Description("Filter by tag ID")),
			mcp.WithString("payee", mcp.Description("Filter by payee (case-insensitive substring)")),
			mcp.WithString("merchant_id", mcp.Description("Filter by merchant ID")),
			mcp.WithNumber("min_amount", mcp.Description("Minimum amount (income or outcome)")),
			mcp.WithNumber("max_amount", mcp.Description("Maximum amount (income or outcome)")),
			mcp.WithBoolean("uncategorized", mcp.Description("If true, return only transactions with no tags")),
			mcp.WithString("transaction_type", mcp.Description("Filter by type: expense, income, or transfer")),
			mcp.WithString("sort", mcp.Description("Sort direction by date: asc or desc (default: desc)")),
			mcp.WithNumber("limit", mcp.Description("Max results (default: 100, max: 500)")),
			mcp.WithNumber("offset", mcp.Description("Items to skip for pagination (default: 0)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleListTransactions(ctx, runtime, req)
		},
	)

	s.AddTool(
		mcp.NewTool("create_transaction",
			mcp.WithDescription("Create a new financial transaction. For transfers, provide to_account_id. Currency instruments are auto-resolved from the account unless overridden."),
			mcp.WithString("transaction_type",
				mcp.Required(),
				mcp.Description("Transaction type: expense, income, or transfer"),
			),
			mcp.WithString("date",
				mcp.Required(),
				mcp.Description("Transaction date (YYYY-MM-DD)"),
			),
			mcp.WithString("account_id",
				mcp.Required(),
				mcp.Description("Account ID: source for expense/transfer, destination for income"),
			),
			mcp.WithNumber("amount",
				mcp.Required(),
				mcp.Description("Transaction amount (positive number)"),
			),
			mcp.WithString("to_account_id",
				mcp.Description("Destination account ID (required for transfers)"),
			),
			mcp.WithNumber("to_amount",
				mcp.Description("Amount in destination account currency (transfers with conversion; defaults to amount)"),
			),
			mcp.WithNumber("instrument_id",
				mcp.Description("Currency instrument ID (overrides account currency)"),
			),
			mcp.WithNumber("to_instrument_id",
				mcp.Description("Destination currency instrument ID (overrides destination account currency)"),
			),
			mcp.WithString("tag_ids",
				mcp.Description("JSON array of tag IDs for categorization, e.g. [\"uuid1\",\"uuid2\"]"),
			),
			mcp.WithString("payee", mcp.Description("Payee name")),
			mcp.WithString("comment", mcp.Description("Transaction comment")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleCreateTransaction(ctx, runtime, req)
		},
	)

	s.AddTool(
		mcp.NewTool("update_transaction",
			mcp.WithDescription("Update an existing transaction by ID. Only provided fields are changed. Use empty string for payee or comment to clear them."),
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("Transaction UUID to update"),
			),
			mcp.WithString("date", mcp.Description("New date (YYYY-MM-DD)")),
			mcp.WithNumber("amount", mcp.Description("New amount (applied to the correct side based on transaction type)")),
			mcp.WithNumber("to_amount", mcp.Description("New destination amount (for transfers with currency conversion)")),
			mcp.WithString("account_id", mcp.Description("New primary account ID")),
			mcp.WithString("to_account_id", mcp.Description("New destination account ID (for transfers)")),
			mcp.WithString("tag_ids",
				mcp.Description("JSON array of tag IDs, e.g. [\"uuid1\",\"uuid2\"]. Replaces existing tags."),
			),
			mcp.WithString("payee", mcp.Description("New payee. Empty string clears it.")),
			mcp.WithString("comment", mcp.Description("New comment. Empty string clears it.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleUpdateTransaction(ctx, runtime, req)
		},
	)

	s.AddTool(
		mcp.NewTool("delete_transaction",
			mcp.WithDescription("Delete a transaction by ID. Returns details of the deleted transaction for confirmation."),
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("Transaction UUID to delete"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id := req.GetString("id", "")
			return handleDeleteTransaction(ctx, runtime, id)
		},
	)
}

type paginatedTransactions struct {
	Items  []transactionResult `json:"items"`
	Total  int                 `json:"total"`
	Offset int                 `json:"offset"`
	Limit  int                 `json:"limit"`
}

func handleListTransactions(ctx context.Context, runtime *RuntimeProvider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	cfg, err := runtime.config()
	if err != nil {
		return runtimeError(err), nil
	}

	resp, maps, err := fetchSyncResponse(ctx, c, runtime.zenStore)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	// Parse filter parameters.
	dateFrom := req.GetString("date_from", "")
	dateTo := req.GetString("date_to", "")
	accountID := req.GetString("account_id", "")
	tagID := req.GetString("tag_id", "")
	payeeFilter := req.GetString("payee", "")
	merchantID := req.GetString("merchant_id", "")
	minAmount := req.GetFloat("min_amount", 0)
	maxAmount := req.GetFloat("max_amount", 0)
	uncategorized := req.GetBool("uncategorized", false)
	txType := req.GetString("transaction_type", "")
	sortDir := req.GetString("sort", "desc")
	limit := int(req.GetFloat("limit", float64(cfg.TransactionLimit)))
	offset := int(req.GetFloat("offset", 0))

	if limit <= 0 {
		limit = cfg.TransactionLimit
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}

	// Filter transactions.
	var filtered []models.Transaction
	for _, tx := range resp.Transaction {
		if tx.Deleted {
			continue
		}
		if dateFrom != "" && tx.Date < dateFrom {
			continue
		}
		if dateTo != "" && tx.Date > dateTo {
			continue
		}
		if accountID != "" {
			outcomeAcc := tx.IncomeAccount
			if tx.OutcomeAccount != nil {
				outcomeAcc = *tx.OutcomeAccount
			}
			if tx.IncomeAccount != accountID && outcomeAcc != accountID {
				continue
			}
		}
		if tagID != "" && !containsStringFold(tx.Tag, tagID) {
			continue
		}
		if payeeFilter != "" && !strings.Contains(strings.ToLower(tx.Payee), strings.ToLower(payeeFilter)) {
			continue
		}
		if merchantID != "" && (tx.Merchant == nil || *tx.Merchant != merchantID) {
			continue
		}
		if minAmount > 0 {
			maxSide := tx.Income
			if tx.Outcome > maxSide {
				maxSide = tx.Outcome
			}
			if maxSide < minAmount {
				continue
			}
		}
		if maxAmount > 0 {
			maxSide := tx.Income
			if tx.Outcome > maxSide {
				maxSide = tx.Outcome
			}
			if maxSide > maxAmount {
				continue
			}
		}
		if uncategorized && len(tx.Tag) > 0 {
			continue
		}
		if txType != "" && classifyTx(tx) != txType {
			continue
		}
		filtered = append(filtered, tx)
	}

	// Sort by date.
	sort.Slice(filtered, func(i, j int) bool {
		if sortDir == "asc" {
			return filtered[i].Date < filtered[j].Date
		}
		return filtered[i].Date > filtered[j].Date
	})

	total := len(filtered)

	// Paginate.
	if offset >= len(filtered) {
		filtered = nil
	} else {
		filtered = filtered[offset:]
		if len(filtered) > limit {
			filtered = filtered[:limit]
		}
	}

	items := make([]transactionResult, len(filtered))
	for i, tx := range filtered {
		items[i] = shapeTransaction(tx, maps)
	}

	out, err := structJSON(paginatedTransactions{
		Items:  items,
		Total:  total,
		Offset: offset,
		Limit:  limit,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return out, nil
}

func handleCreateTransaction(ctx context.Context, runtime *RuntimeProvider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	resp, maps, err := fetchSyncResponse(ctx, c, runtime.zenStore)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	userID := 0
	if len(resp.User) > 0 {
		userID = resp.User[0].ID
	}

	txType := req.GetString("transaction_type", "")
	if txType == "" {
		return mcp.NewToolResultError("transaction_type is required (expense, income, or transfer)"), nil
	}

	dateStr := req.GetString("date", "")
	if dateStr == "" {
		return mcp.NewToolResultError("date is required (YYYY-MM-DD)"), nil
	}
	if _, err := time.Parse("2006-01-02", dateStr); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid date %q: use YYYY-MM-DD", dateStr)), nil
	}

	accountID := req.GetString("account_id", "")
	if accountID == "" {
		return mcp.NewToolResultError("account_id is required"), nil
	}
	if _, ok := maps.Accounts[accountID]; !ok {
		return mcp.NewToolResultError(fmt.Sprintf("account %q not found", accountID)), nil
	}

	amount := req.GetFloat("amount", 0)
	if amount <= 0 {
		return mcp.NewToolResultError("amount must be a positive number"), nil
	}

	toAccountID := req.GetString("to_account_id", "")
	toAmount := req.GetFloat("to_amount", amount)
	if toAmount <= 0 {
		toAmount = amount
	}

	// Resolve instruments.
	instrID, ok := maps.AccountInstrument(accountID)
	if !ok {
		instrID = 0
	}
	if v := req.GetFloat("instrument_id", 0); v > 0 {
		instrID = int(v)
	}

	toInstrID := instrID
	if toAccountID != "" {
		if id, ok := maps.AccountInstrument(toAccountID); ok {
			toInstrID = id
		}
	}
	if v := req.GetFloat("to_instrument_id", 0); v > 0 {
		toInstrID = int(v)
	}

	// Parse tag_ids (JSON array string).
	var tagIDs []string
	if raw := req.GetString("tag_ids", ""); raw != "" {
		if err := json.Unmarshal([]byte(raw), &tagIDs); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid tag_ids: %v", err)), nil
		}
	}

	payee := req.GetString("payee", "")
	comment := strPtr(req.GetString("comment", ""))

	now := int(time.Now().Unix())
	tx := models.Transaction{
		ID:      uuid.New().String(),
		User:    userID,
		Date:    dateStr,
		Changed: now,
		Created: now,
		Tag:     tagIDs,
		Payee:   payee,
		Comment: comment,
	}

	switch txType {
	case "expense":
		tx.Income = 0
		tx.Outcome = amount
		tx.IncomeAccount = accountID
		tx.OutcomeAccount = &accountID
		tx.IncomeInstrument = instrID
		tx.OutcomeInstrument = instrID
	case "income":
		tx.Income = amount
		tx.Outcome = 0
		tx.IncomeAccount = accountID
		tx.OutcomeAccount = &accountID
		tx.IncomeInstrument = instrID
		tx.OutcomeInstrument = instrID
	case "transfer":
		if toAccountID == "" {
			return mcp.NewToolResultError("to_account_id is required for transfers"), nil
		}
		if _, ok := maps.Accounts[toAccountID]; !ok {
			return mcp.NewToolResultError(fmt.Sprintf("to_account %q not found", toAccountID)), nil
		}
		tx.Outcome = amount
		tx.Income = toAmount
		tx.OutcomeAccount = &accountID
		tx.IncomeAccount = toAccountID
		tx.OutcomeInstrument = instrID
		tx.IncomeInstrument = toInstrID
	default:
		return mcp.NewToolResultError(fmt.Sprintf("unknown transaction_type %q: use expense, income, or transfer", txType)), nil
	}

	pushResp, err := c.Push(ctx, pushRequest(currentServerTimestamp(runtime.zenStore), []models.Transaction{tx}))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("create transaction: %v", err)), nil
	}

	if pushResp.ServerTimestamp > 0 {
		_ = runtime.zenStore.Save(&store.SyncState{ServerTimestamp: pushResp.ServerTimestamp, LastSyncAt: time.Now()})
	}

	// Rebuild maps from push response to get updated instrument info, then shape result.
	if len(pushResp.Instrument) > 0 || len(pushResp.Account) > 0 {
		pushMaps := BuildLookupMaps(pushResp)
		// Merge: keep original maps for existing data, add any new entries.
		for k, v := range pushMaps.Accounts {
			maps.Accounts[k] = v
		}
		for k, v := range pushMaps.Instruments {
			maps.Instruments[k] = v
		}
		for k, v := range pushMaps.Tags {
			maps.Tags[k] = v
		}
		for k, v := range pushMaps.AccountInstruments {
			maps.AccountInstruments[k] = v
		}
	}

	out, err := structJSON(shapeTransaction(tx, maps))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return out, nil
}

func handleUpdateTransaction(ctx context.Context, runtime *RuntimeProvider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	txID := req.GetString("id", "")
	if txID == "" {
		return mcp.NewToolResultError("id is required"), nil
	}

	resp, maps, err := fetchSyncResponse(ctx, c, runtime.zenStore)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	// Find existing transaction.
	var existing *models.Transaction
	for i := range resp.Transaction {
		if resp.Transaction[i].ID == txID {
			existing = &resp.Transaction[i]
			break
		}
	}
	if existing == nil {
		return mcp.NewToolResultText(fmt.Sprintf("No transaction found with ID %q", txID)), nil
	}

	tx := *existing
	tx.Changed = int(time.Now().Unix())

	if date := req.GetString("date", ""); date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid date %q: use YYYY-MM-DD", date)), nil
		}
		tx.Date = date
	}

	txType := classifyTx(tx)

	if amount := req.GetFloat("amount", -1); amount >= 0 {
		switch txType {
		case "expense":
			tx.Outcome = amount
		case "income":
			tx.Income = amount
		case "transfer":
			tx.Outcome = amount
		}
	}

	if toAmount := req.GetFloat("to_amount", -1); toAmount >= 0 && txType == "transfer" {
		tx.Income = toAmount
	}

	if accountID := req.GetString("account_id", ""); accountID != "" {
		if _, ok := maps.Accounts[accountID]; !ok {
			return mcp.NewToolResultError(fmt.Sprintf("account %q not found", accountID)), nil
		}
		switch txType {
		case "expense", "income":
			tx.IncomeAccount = accountID
			tx.OutcomeAccount = &accountID
		case "transfer":
			tx.OutcomeAccount = &accountID
		}
	}

	if toAccountID := req.GetString("to_account_id", ""); toAccountID != "" && txType == "transfer" {
		if _, ok := maps.Accounts[toAccountID]; !ok {
			return mcp.NewToolResultError(fmt.Sprintf("to_account %q not found", toAccountID)), nil
		}
		tx.IncomeAccount = toAccountID
	}

	if raw := req.GetString("tag_ids", ""); raw != "" {
		var tagIDs []string
		if err := json.Unmarshal([]byte(raw), &tagIDs); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid tag_ids: %v", err)), nil
		}
		tx.Tag = tagIDs
	}

	// Check if payee/comment were explicitly provided (even as empty string to clear).
	if args, ok := req.Params.Arguments.(map[string]any); ok {
		if _, hasPay := args["payee"]; hasPay {
			tx.Payee = req.GetString("payee", "")
		}
		if _, hasCom := args["comment"]; hasCom {
			c2 := req.GetString("comment", "")
			tx.Comment = &c2
		}
	}

	pushResp, err := c.Push(ctx, pushRequest(currentServerTimestamp(runtime.zenStore), []models.Transaction{tx}))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("update transaction: %v", err)), nil
	}

	if pushResp.ServerTimestamp > 0 {
		_ = runtime.zenStore.Save(&store.SyncState{ServerTimestamp: pushResp.ServerTimestamp, LastSyncAt: time.Now()})
	}

	out, err := structJSON(shapeTransaction(tx, maps))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return out, nil
}

func handleDeleteTransaction(ctx context.Context, runtime *RuntimeProvider, txID string) (*mcp.CallToolResult, error) {
	if txID == "" {
		return mcp.NewToolResultError("id is required"), nil
	}

	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	resp, maps, err := fetchSyncResponse(ctx, c, runtime.zenStore)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	userID := 0
	if len(resp.User) > 0 {
		userID = resp.User[0].ID
	}

	// Capture full transaction details before deletion.
	var existing *models.Transaction
	for i := range resp.Transaction {
		if resp.Transaction[i].ID == txID {
			existing = &resp.Transaction[i]
			break
		}
	}
	if existing == nil {
		return mcp.NewToolResultText(fmt.Sprintf("No transaction found with ID %q", txID)), nil
	}

	deletionTx := models.Transaction{
		ID:      txID,
		User:    userID,
		Changed: int(time.Now().Unix()),
		Deleted: true,
		// Required fields to satisfy the API.
		IncomeAccount:     existing.IncomeAccount,
		OutcomeAccount:    existing.OutcomeAccount,
		IncomeInstrument:  existing.IncomeInstrument,
		OutcomeInstrument: existing.OutcomeInstrument,
		Date:              existing.Date,
	}

	pushResp, err := c.Push(ctx, pushRequest(currentServerTimestamp(runtime.zenStore), []models.Transaction{deletionTx}))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("delete transaction: %v", err)), nil
	}

	if pushResp.ServerTimestamp > 0 {
		_ = runtime.zenStore.Save(&store.SyncState{ServerTimestamp: pushResp.ServerTimestamp, LastSyncAt: time.Now()})
	}

	type deleteResult struct {
		Message     string            `json:"message"`
		Transaction transactionResult `json:"transaction"`
	}
	out, err := structJSON(deleteResult{
		Message:     fmt.Sprintf("Transaction %s deleted", txID),
		Transaction: shapeTransaction(*existing, maps),
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return out, nil
}
