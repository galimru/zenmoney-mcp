package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

type transactionEnv struct {
	resp   models.Response
	maps   LookupMaps
	userID int
	txByID map[string]models.Transaction
}

type importDraft struct {
	ExternalID     string  `json:"external_id,omitempty"`
	Date           string  `json:"date"`
	Amount         float64 `json:"amount"`
	Type           string  `json:"type,omitempty"`
	AccountID      string  `json:"account_id,omitempty"`
	ToAccountID    string  `json:"to_account_id,omitempty"`
	Account        string  `json:"account,omitempty"`
	ToAccount      string  `json:"to_account,omitempty"`
	Payee          string  `json:"payee,omitempty"`
	Comment        string  `json:"comment,omitempty"`
	Category       string  `json:"category,omitempty"`
	Categories     any     `json:"categories,omitempty"`
	Currency       string  `json:"currency,omitempty"`
	AutoCategorize *bool   `json:"auto_categorize,omitempty"`
}

type importPreviewRow struct {
	Index             int                `json:"index"`
	ExternalID        string             `json:"external_id,omitempty"`
	Status            string             `json:"status"`
	Reason            string             `json:"reason,omitempty"`
	NeedsReview       bool               `json:"needs_review,omitempty"`
	ReviewReason      string             `json:"review_reason,omitempty"`
	ResolvedAccount   string             `json:"resolved_account,omitempty"`
	ResolvedToAccount string             `json:"resolved_to_account,omitempty"`
	ResolvedTags      []string           `json:"resolved_categories,omitempty"`
	CategorizedBy     string             `json:"categorized_by,omitempty"`
	Transaction       *transactionResult `json:"transaction,omitempty"`
}

type importPreviewResponse struct {
	ImportPlanID       string             `json:"import_plan_id"`
	ReadyToImport      int                `json:"ready_to_import"`
	ExactDuplicates    int                `json:"exact_duplicates"`
	PossibleDuplicates int                `json:"possible_duplicates"`
	Invalid            int                `json:"invalid"`
	NeedsReview        int                `json:"needs_review"`
	Rows               []importPreviewRow `json:"rows"`
}

type importCommitResponse struct {
	Created            int                 `json:"created"`
	SkippedExact       int                 `json:"skipped_exact_duplicates"`
	SkippedPossible    int                 `json:"skipped_possible_duplicates"`
	SkippedInvalid     int                 `json:"skipped_invalid"`
	SkippedNeedsReview int                 `json:"skipped_needs_review"`
	Failed             int                 `json:"failed"`
	Transactions       []transactionResult `json:"transactions"`
	Errors             []string            `json:"errors,omitempty"`
}

type categorizationPreviewItem struct {
	TransactionID string   `json:"transaction_id"`
	Date          string   `json:"date"`
	Payee         string   `json:"payee,omitempty"`
	Comment       string   `json:"comment,omitempty"`
	CurrentTags   []string `json:"current_categories"`
	SuggestedTags []string `json:"suggested_categories,omitempty"`
	Status        string   `json:"status"`
	Reason        string   `json:"reason,omitempty"`
}

type categorizeResponse struct {
	Applied     int                         `json:"applied"`
	NeedsReview int                         `json:"needs_review"`
	Skipped     int                         `json:"skipped"`
	Items       []categorizationPreviewItem `json:"items"`
}

type plannedImportRow struct {
	Preview importPreviewRow
	Tx      *models.Transaction
}

type PreparedImportPlan struct {
	Rows               []plannedImportRow
	ReadyToImport      int
	ExactDuplicates    int
	PossibleDuplicates int
	Invalid            int
	NeedsReview        int
}

type paginatedTransactions struct {
	Items  []transactionResult `json:"items"`
	Total  int                 `json:"total"`
	Offset int                 `json:"offset"`
	Limit  int                 `json:"limit"`
}

// RegisterTransactionTools adds the public transaction workflow tools to the MCP server.
func RegisterTransactionTools(s *server.MCPServer, runtime *RuntimeProvider) {
	s.AddTool(
		mcp.NewTool("find_transactions",
			mcp.WithDescription("Find transactions by date, account, category, amount, payee, query, or type. Account and category fields accept either a title or an ID."),
			mcp.WithString("date_from", mcp.Description("Start date inclusive (YYYY-MM-DD)")),
			mcp.WithString("date_to", mcp.Description("End date inclusive (YYYY-MM-DD)")),
			mcp.WithString("account", mcp.Description("Account title or ID")),
			mcp.WithString("category", mcp.Description("Category title or ID")),
			mcp.WithString("query", mcp.Description("Case-insensitive search across payee and comment")),
			mcp.WithString("payee", mcp.Description("Case-insensitive payee filter")),
			mcp.WithNumber("min_amount", mcp.Description("Minimum transaction amount")),
			mcp.WithNumber("max_amount", mcp.Description("Maximum transaction amount")),
			mcp.WithBoolean("uncategorized", mcp.Description("If true, return only transactions without categories")),
			mcp.WithString("type", mcp.Description("Transaction type: expense, income, or transfer")),
			mcp.WithString("sort", mcp.Description("Sort direction by date: asc or desc (default: desc)")),
			mcp.WithNumber("limit", mcp.Description("Max results (default: 100, max: 500)")),
			mcp.WithNumber("offset", mcp.Description("Items to skip for pagination (default: 0)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleFindTransactions(ctx, runtime, req)
		},
	)

	s.AddTool(
		mcp.NewTool("add_transaction",
			mcp.WithDescription("Add a transaction from type, date, amount, account_id, and optional category, payee, comment, currency, or to_account_id fields. Category lists can be passed as a JSON array string or a comma-separated string."),
			mcp.WithString("type", mcp.Required(), mcp.Description("Transaction type: expense, income, or transfer")),
			mcp.WithString("date", mcp.Required(), mcp.Description("Transaction date (YYYY-MM-DD)")),
			mcp.WithNumber("amount", mcp.Required(), mcp.Description("Positive transaction amount")),
			mcp.WithString("account_id", mcp.Required(), mcp.Description("Source account ID")),
			mcp.WithString("to_account_id", mcp.Description("Destination account ID for transfers")),
			mcp.WithString("category", mcp.Description("Single category title or ID")),
			mcp.WithString("categories", mcp.Description("Categories as JSON array string or comma-separated string")),
			mcp.WithString("payee", mcp.Description("Payee name")),
			mcp.WithString("comment", mcp.Description("Comment")),
			mcp.WithString("currency", mcp.Description("Optional currency symbol, code, title, or instrument ID")),
			mcp.WithBoolean("auto_categorize", mcp.Description("Apply assisted categorization when category is omitted (default: false)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleAddTransaction(ctx, runtime, req)
		},
	)

	s.AddTool(
		mcp.NewTool("edit_transaction",
			mcp.WithDescription("Edit an existing transaction by ID. Optional fields update only the provided parts of the transaction, and clear_category, clear_payee, and clear_comment remove those fields."),
			mcp.WithString("transaction_id", mcp.Required(), mcp.Description("Transaction ID to update")),
			mcp.WithString("type", mcp.Description("New transaction type: expense, income, or transfer")),
			mcp.WithString("date", mcp.Description("New date (YYYY-MM-DD)")),
			mcp.WithNumber("amount", mcp.Description("New amount")),
			mcp.WithString("account_id", mcp.Description("New primary account ID")),
			mcp.WithString("to_account_id", mcp.Description("New destination account ID for transfers")),
			mcp.WithString("category", mcp.Description("Single category title or ID")),
			mcp.WithString("categories", mcp.Description("Categories as JSON array string or comma-separated string")),
			mcp.WithString("payee", mcp.Description("New payee")),
			mcp.WithString("comment", mcp.Description("New comment")),
			mcp.WithString("currency", mcp.Description("Optional currency symbol, code, title, or instrument ID")),
			mcp.WithBoolean("auto_categorize", mcp.Description("Apply assisted categorization when categories are omitted")),
			mcp.WithBoolean("clear_category", mcp.Description("If true, remove all categories")),
			mcp.WithBoolean("clear_payee", mcp.Description("If true, clear payee")),
			mcp.WithBoolean("clear_comment", mcp.Description("If true, clear comment")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleEditTransaction(ctx, runtime, req)
		},
	)

	s.AddTool(
		mcp.NewTool("remove_transaction",
			mcp.WithDescription("Delete a transaction by ID."),
			mcp.WithString("transaction_id", mcp.Required(), mcp.Description("Transaction ID to delete")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleDeleteTransaction(ctx, runtime, req.GetString("transaction_id", ""))
		},
	)

	s.AddTool(
		mcp.NewTool("categorize_transactions",
			mcp.WithDescription("Preview or apply categories for existing transactions selected by IDs or filters. If category is omitted, the server uses assisted categorization and marks ambiguous results for review."),
			mcp.WithString("transaction_ids", mcp.Description("Transaction IDs as JSON array string or comma-separated string")),
			mcp.WithString("account_id", mcp.Description("Filter by account ID")),
			mcp.WithString("category", mcp.Description("Apply this category title or ID to all matched transactions")),
			mcp.WithString("date_from", mcp.Description("Start date inclusive (YYYY-MM-DD)")),
			mcp.WithString("date_to", mcp.Description("End date inclusive (YYYY-MM-DD)")),
			mcp.WithBoolean("uncategorized", mcp.Description("If true, limit to uncategorized transactions")),
			mcp.WithString("type", mcp.Description("Transaction type: expense, income, or transfer")),
			mcp.WithString("query", mcp.Description("Case-insensitive search across payee and comment")),
			mcp.WithBoolean("auto_apply", mcp.Description("If true, apply clear suggestions immediately")),
			mcp.WithBoolean("dry_run", mcp.Description("If true, preview without writing (default: true)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleCategorizeTransactions(ctx, runtime, req)
		},
	)

	s.AddTool(
		mcp.NewTool("preview_transaction_import",
			mcp.WithDescription("Preview an import from canonical transaction rows. The items field must be a JSON array of rows such as date, amount, type, payee, comment, category, and account_id, and the preview reports duplicates, invalid rows, and review items before any write happens."),
			mcp.WithString("items", mcp.Required(), mcp.Description("JSON array of canonical transaction drafts")),
			mcp.WithString("account_id", mcp.Description("Default account ID for rows that omit account_id")),
			mcp.WithString("default_type", mcp.Description("Default transaction type for rows that omit type")),
			mcp.WithBoolean("auto_categorize", mcp.Description("Apply assisted categorization for rows without categories (default: true)")),
			mcp.WithString("duplicate_policy", mcp.Description("Duplicate handling policy: skip_possible_duplicates or allow_possible_duplicates (default: skip_possible_duplicates)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handlePreviewTransactionImport(ctx, runtime, req)
		},
	)

	s.AddTool(
		mcp.NewTool("commit_transaction_import",
			mcp.WithDescription("Commit a previously previewed import plan sequentially and return the created, skipped, and failed counts."),
			mcp.WithString("import_plan_id", mcp.Required(), mcp.Description("The import_plan_id returned by preview_transaction_import")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleCommitTransactionImport(ctx, runtime, req.GetString("import_plan_id", ""))
		},
	)
}

func loadTransactionEnv(ctx context.Context, runtime *RuntimeProvider, scope []models.EntityType) (*transactionEnv, error) {
	resp, maps, err := runtime.scopedSync(ctx, scope)
	if err != nil {
		return nil, err
	}

	env := &transactionEnv{
		resp:   resp,
		maps:   maps,
		txByID: make(map[string]models.Transaction, len(resp.Transaction)),
	}
	for _, tx := range resp.Transaction {
		env.txByID[tx.ID] = tx
	}
	if len(resp.User) > 0 {
		env.userID = resp.User[0].ID
	}
	return env, nil
}

func handleFindTransactions(ctx context.Context, runtime *RuntimeProvider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cfg, err := runtime.config()
	if err != nil {
		return runtimeError(err), nil
	}

	env, err := loadTransactionEnv(ctx, runtime, scopeTransactionsRead)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	accountID := ""
	if ref := req.GetString("account", ""); ref != "" {
		accountID, err = resolveAccountRef(ref, env)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}

	tagID := ""
	if ref := req.GetString("category", ""); ref != "" {
		tagID, err = resolveCategoryRef(ref, env)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}

	dateFrom := req.GetString("date_from", "")
	dateTo := req.GetString("date_to", "")
	query := strings.ToLower(req.GetString("query", ""))
	payee := strings.ToLower(req.GetString("payee", ""))
	txType := req.GetString("type", "")
	sortDir := req.GetString("sort", "desc")
	minAmount := req.GetFloat("min_amount", 0)
	maxAmount := req.GetFloat("max_amount", 0)
	uncategorized := req.GetBool("uncategorized", false)
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

	var filtered []models.Transaction
	for _, tx := range env.resp.Transaction {
		if tx.Deleted {
			continue
		}
		if dateFrom != "" && tx.Date < dateFrom {
			continue
		}
		if dateTo != "" && tx.Date > dateTo {
			continue
		}
		if accountID != "" && !transactionTouchesAccount(tx, accountID) {
			continue
		}
		if tagID != "" && !containsStringFold(tx.Tag, tagID) {
			continue
		}
		if query != "" {
			comment := ""
			if tx.Comment != nil {
				comment = *tx.Comment
			}
			if !strings.Contains(strings.ToLower(tx.Payee), query) && !strings.Contains(strings.ToLower(comment), query) {
				continue
			}
		}
		if payee != "" && !strings.Contains(strings.ToLower(tx.Payee), payee) {
			continue
		}
		if uncategorized && len(tx.Tag) > 0 {
			continue
		}
		if txType != "" && classifyTx(tx) != txType {
			continue
		}
		amount := majorAmount(tx)
		if minAmount > 0 && amount < minAmount {
			continue
		}
		if maxAmount > 0 && amount > maxAmount {
			continue
		}
		filtered = append(filtered, tx)
	}

	sort.Slice(filtered, func(i, j int) bool {
		if sortDir == "asc" {
			return filtered[i].Date < filtered[j].Date
		}
		return filtered[i].Date > filtered[j].Date
	})

	total := len(filtered)
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
		items[i] = shapeTransaction(tx, env.maps)
	}

	return structJSON(paginatedTransactions{
		Items:  items,
		Total:  total,
		Offset: offset,
		Limit:  limit,
	})
}

func handleAddTransaction(ctx context.Context, runtime *RuntimeProvider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	env, err := loadTransactionEnv(ctx, runtime, scopeTransactionsWrite)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	tx, _, err := buildTransactionFromRequest(ctx, req, env, c, nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	pushResp, err := c.Push(ctx, pushRequest(runtime.currentServerTimestamp(), []models.Transaction{tx}))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("create transaction: %v", err)), nil
	}
	if pushResp.ServerTimestamp > 0 {
		runtime.saveServerTimestamp(pushResp.ServerTimestamp)
	}

	return structJSON(shapeTransaction(tx, env.maps))
}

func handleEditTransaction(ctx context.Context, runtime *RuntimeProvider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	env, err := loadTransactionEnv(ctx, runtime, scopeTransactionsWrite)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	txID := req.GetString("transaction_id", "")
	existing, ok := env.txByID[txID]
	if !ok {
		return mcp.NewToolResultText(fmt.Sprintf("No transaction found with ID %q", txID)), nil
	}

	updated, err := applyRequestToTransaction(ctx, req, existing, env, c)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	pushResp, err := c.Push(ctx, pushRequest(runtime.currentServerTimestamp(), []models.Transaction{updated}))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("update transaction: %v", err)), nil
	}
	if pushResp.ServerTimestamp > 0 {
		runtime.saveServerTimestamp(pushResp.ServerTimestamp)
	}

	return structJSON(shapeTransaction(updated, env.maps))
}

func handleCategorizeTransactions(ctx context.Context, runtime *RuntimeProvider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	env, err := loadTransactionEnv(ctx, runtime, scopeTransactionsWrite)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	selected, err := selectTransactionsForCategorization(req, env)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	explicitCategory := req.GetString("category", "")
	autoApply := req.GetBool("auto_apply", false)
	dryRun := req.GetBool("dry_run", true)

	var explicitTagIDs []string
	if explicitCategory != "" {
		explicitTagIDs, _, err = resolveRequestedTags(explicitCategory, "", env)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
	}

	now := int(time.Now().Unix())
	var toPush []models.Transaction
	items := make([]categorizationPreviewItem, 0, len(selected))
	applied := 0
	needsReview := 0
	skipped := 0

	for _, tx := range selected {
		item := categorizationPreviewItem{
			TransactionID: tx.ID,
			Date:          tx.Date,
			Payee:         tx.Payee,
			CurrentTags:   env.maps.TagNames(tx.Tag),
		}
		if tx.Comment != nil {
			item.Comment = *tx.Comment
		}

		var nextTags []string
		source := ""
		reviewReason := ""

		if len(explicitTagIDs) > 0 {
			nextTags = explicitTagIDs
			source = "explicit"
		} else {
			nextTags, source, reviewReason, err = resolveCategoriesForTransaction(ctx, c, env, tx, true)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
		}

		item.SuggestedTags = env.maps.TagNames(nextTags)
		switch {
		case reviewReason != "":
			item.Status = "needs_review"
			item.Reason = reviewReason
			needsReview++
		case len(nextTags) == 0:
			item.Status = "skipped"
			item.Reason = "no clear category"
			skipped++
		case equalStringSlices(tx.Tag, nextTags):
			item.Status = "skipped"
			item.Reason = "already categorized"
			skipped++
		case dryRun || !autoApply:
			item.Status = "ready"
			item.Reason = source
			applied++
		default:
			updated := tx
			updated.Tag = nextTags
			updated.Changed = now
			toPush = append(toPush, updated)
			item.Status = "applied"
			item.Reason = source
			applied++
		}

		items = append(items, item)
	}

	if len(toPush) > 0 {
		pushResp, err := c.Push(ctx, pushRequest(runtime.currentServerTimestamp(), toPush))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("categorize transactions: %v", err)), nil
		}
		if pushResp.ServerTimestamp > 0 {
			runtime.saveServerTimestamp(pushResp.ServerTimestamp)
		}
	}

	return structJSON(categorizeResponse{
		Applied:     applied,
		NeedsReview: needsReview,
		Skipped:     skipped,
		Items:       items,
	})
}

func handlePreviewTransactionImport(ctx context.Context, runtime *RuntimeProvider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cfg, err := runtime.config()
	if err != nil {
		return runtimeError(err), nil
	}
	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	env, err := loadTransactionEnv(ctx, runtime, scopeTransactionsWrite)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	rawItems := req.GetString("items", "")
	if rawItems == "" {
		return mcp.NewToolResultError("items is required"), nil
	}

	var drafts []importDraft
	if err := json.Unmarshal([]byte(rawItems), &drafts); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid items JSON: %v", err)), nil
	}
	if len(drafts) == 0 {
		return mcp.NewToolResultError("items array is empty"), nil
	}
	if len(drafts) > cfg.MaxBulkOperations {
		return mcp.NewToolResultError(fmt.Sprintf("too many rows: %d (max %d)", len(drafts), cfg.MaxBulkOperations)), nil
	}

	defaultAccount := req.GetString("account_id", "")
	defaultType := req.GetString("default_type", "")
	autoCategorize := true
	if _, ok := requestArgs(req)["auto_categorize"]; ok {
		autoCategorize = req.GetBool("auto_categorize", true)
	}
	duplicatePolicy := req.GetString("duplicate_policy", "skip_possible_duplicates")

	plan := &PreparedImportPlan{}
	planID := uuid.New().String()

	for i, draft := range drafts {
		if draft.Account == "" {
			draft.Account = defaultAccount
		}
		if draft.Type == "" {
			draft.Type = defaultType
		}
		if draft.AutoCategorize == nil {
			draft.AutoCategorize = &autoCategorize
		}

		row := importPreviewRow{
			Index:      i,
			ExternalID: draft.ExternalID,
		}

		tx, resolved, buildErr := buildTransactionFromDraft(ctx, c, env, draft)
		if buildErr != nil {
			row.Status = "invalid"
			row.Reason = buildErr.Error()
			plan.Invalid++
			plan.Rows = append(plan.Rows, plannedImportRow{Preview: row})
			continue
		}
		row.ResolvedAccount = resolved.account
		row.ResolvedToAccount = resolved.toAccount
		row.ResolvedTags = resolved.tags
		row.CategorizedBy = resolved.categorizedBy
		if resolved.reviewReason != "" {
			row.NeedsReview = true
			row.ReviewReason = resolved.reviewReason
			plan.NeedsReview++
		}

		switch status, reason := classifyImportDuplicate(*tx, env, duplicatePolicy); status {
		case "exact_duplicate":
			row.Status = status
			row.Reason = reason
			plan.ExactDuplicates++
		case "possible_duplicate":
			row.Status = status
			row.Reason = reason
			plan.PossibleDuplicates++
		default:
			row.Status = "new"
			preview := shapeTransaction(*tx, env.maps)
			row.Transaction = &preview
			if row.NeedsReview {
				row.Reason = "categorization review required"
			} else {
				plan.ReadyToImport++
				plan.Rows = append(plan.Rows, plannedImportRow{Preview: row, Tx: tx})
				continue
			}
		}

		plan.Rows = append(plan.Rows, plannedImportRow{Preview: row})
	}

	runtime.storeImportPlan(planID, plan)

	rows := make([]importPreviewRow, len(plan.Rows))
	for i, row := range plan.Rows {
		rows[i] = row.Preview
	}

	return structJSON(importPreviewResponse{
		ImportPlanID:       planID,
		ReadyToImport:      plan.ReadyToImport,
		ExactDuplicates:    plan.ExactDuplicates,
		PossibleDuplicates: plan.PossibleDuplicates,
		Invalid:            plan.Invalid,
		NeedsReview:        plan.NeedsReview,
		Rows:               rows,
	})
}

func handleCommitTransactionImport(ctx context.Context, runtime *RuntimeProvider, planID string) (*mcp.CallToolResult, error) {
	if planID == "" {
		return mcp.NewToolResultError("import_plan_id is required"), nil
	}

	plan := runtime.takeImportPlan(planID)
	if plan == nil {
		return mcp.NewToolResultError(fmt.Sprintf("import plan %q not found or already committed", planID)), nil
	}

	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	env, err := loadTransactionEnv(ctx, runtime, scopeTransactionsWrite)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	resp := importCommitResponse{
		SkippedExact:       plan.ExactDuplicates,
		SkippedPossible:    plan.PossibleDuplicates,
		SkippedInvalid:     plan.Invalid,
		SkippedNeedsReview: plan.NeedsReview,
		Transactions:       []transactionResult{},
	}

	serverTS := runtime.currentServerTimestamp()
	for _, row := range plan.Rows {
		if row.Tx == nil {
			continue
		}

		pushResp, err := c.Push(ctx, pushRequest(serverTS, []models.Transaction{*row.Tx}))
		if err != nil {
			resp.Failed++
			resp.Errors = append(resp.Errors, fmt.Sprintf("row %d: %v", row.Preview.Index, err))
			continue
		}
		if pushResp.ServerTimestamp > 0 {
			serverTS = pushResp.ServerTimestamp
			runtime.saveServerTimestamp(serverTS)
		}

		resp.Created++
		resp.Transactions = append(resp.Transactions, shapeTransaction(*row.Tx, env.maps))
	}

	return structJSON(resp)
}

func handleDeleteTransaction(ctx context.Context, runtime *RuntimeProvider, txID string) (*mcp.CallToolResult, error) {
	if txID == "" {
		return mcp.NewToolResultError("id is required"), nil
	}

	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	resp, maps, err := runtime.scopedSync(ctx, scopeTransactionsWrite)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	userID := 0
	if len(resp.User) > 0 {
		userID = resp.User[0].ID
	}

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
		ID:                txID,
		User:              userID,
		Changed:           int(time.Now().Unix()),
		Deleted:           true,
		IncomeAccount:     existing.IncomeAccount,
		OutcomeAccount:    existing.OutcomeAccount,
		IncomeInstrument:  existing.IncomeInstrument,
		OutcomeInstrument: existing.OutcomeInstrument,
		Date:              existing.Date,
	}

	pushResp, err := c.Push(ctx, pushRequest(runtime.currentServerTimestamp(), []models.Transaction{deletionTx}))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("delete transaction: %v", err)), nil
	}

	if pushResp.ServerTimestamp > 0 {
		runtime.saveServerTimestamp(pushResp.ServerTimestamp)
	}

	type deleteResult struct {
		Message     string            `json:"message"`
		Transaction transactionResult `json:"transaction"`
	}
	return structJSON(deleteResult{
		Message:     fmt.Sprintf("Transaction %s deleted", txID),
		Transaction: shapeTransaction(*existing, maps),
	})
}

type transactionResolution struct {
	account       string
	toAccount     string
	tags          []string
	categorizedBy string
	reviewReason  string
}

func buildTransactionFromRequest(ctx context.Context, req mcp.CallToolRequest, env *transactionEnv, c clientLike, existing *models.Transaction) (models.Transaction, transactionResolution, error) {
	args := requestArgs(req)
	txType := req.GetString("type", "")
	if txType == "" && existing == nil {
		return models.Transaction{}, transactionResolution{}, fmt.Errorf("type is required")
	}

	dateStr := req.GetString("date", "")
	if dateStr == "" && existing == nil {
		return models.Transaction{}, transactionResolution{}, fmt.Errorf("date is required (YYYY-MM-DD)")
	}

	amount := req.GetFloat("amount", 0)
	if amount <= 0 && existing == nil {
		return models.Transaction{}, transactionResolution{}, fmt.Errorf("amount must be a positive number")
	}

	if _, ok := args["account"]; ok {
		return models.Transaction{}, transactionResolution{}, fmt.Errorf("account titles are not accepted for write operations; use account_id")
	}
	if _, ok := args["to_account"]; ok {
		return models.Transaction{}, transactionResolution{}, fmt.Errorf("account titles are not accepted for write operations; use to_account_id")
	}

	accountRef := req.GetString("account_id", "")
	if accountRef == "" && existing == nil {
		return models.Transaction{}, transactionResolution{}, fmt.Errorf("account_id is required")
	}
	toAccountRef := req.GetString("to_account_id", "")
	currencyRef := req.GetString("currency", "")

	autoCategorize := false
	if _, ok := args["auto_categorize"]; ok {
		autoCategorize = req.GetBool("auto_categorize", false)
	}

	requestedTagIDs, requestedExplicit, err := resolveRequestedTags(req.GetString("category", ""), req.GetString("categories", ""), env)
	if err != nil {
		return models.Transaction{}, transactionResolution{}, err
	}

	var tx models.Transaction
	if existing != nil {
		tx = *existing
	} else {
		now := int(time.Now().Unix())
		tx = models.Transaction{
			ID:      uuid.New().String(),
			User:    env.userID,
			Changed: now,
			Created: now,
		}
	}

	if dateStr != "" {
		if _, err := time.Parse("2006-01-02", dateStr); err != nil {
			return models.Transaction{}, transactionResolution{}, fmt.Errorf("invalid date %q: use YYYY-MM-DD", dateStr)
		}
		tx.Date = dateStr
	}

	if txType == "" {
		txType = classifyTx(tx)
	}

	accountID := tx.IncomeAccount
	if accountRef != "" {
		accountID, err = resolveWritableAccountID(accountRef, env)
		if err != nil {
			return models.Transaction{}, transactionResolution{}, err
		}
	}
	toAccountID := tx.IncomeAccount
	if classifyTx(tx) == "transfer" && txType == "transfer" {
		toAccountID = tx.IncomeAccount
	}
	if txType == "transfer" {
		if toAccountRef != "" {
			toAccountID, err = resolveWritableAccountID(toAccountRef, env)
			if err != nil {
				return models.Transaction{}, transactionResolution{}, err
			}
		} else if existing == nil {
			return models.Transaction{}, transactionResolution{}, fmt.Errorf("to_account_id is required for transfers")
		}
	}

	outcomeAccount := accountID
	if txType == "transfer" {
		outcomeAccount = accountID
	}

	accountInstrument, _ := env.maps.AccountInstrument(accountID)
	instrumentID := accountInstrument
	if currencyRef != "" {
		instrumentID, err = resolveInstrumentRef(currencyRef, env)
		if err != nil {
			return models.Transaction{}, transactionResolution{}, err
		}
	}

	toInstrumentID := instrumentID
	if txType == "transfer" {
		if id, ok := env.maps.AccountInstrument(toAccountID); ok {
			toInstrumentID = id
		}
	}

	if _, ok := args["amount"]; ok || existing == nil {
		switch txType {
		case "expense":
			tx.Income = 0
			tx.Outcome = amount
		case "income":
			tx.Income = amount
			tx.Outcome = 0
		case "transfer":
			tx.Outcome = amount
			tx.Income = amount
		default:
			return models.Transaction{}, transactionResolution{}, fmt.Errorf("unknown type %q: use expense, income, or transfer", txType)
		}
	}

	switch txType {
	case "expense", "income":
		tx.IncomeAccount = accountID
		tx.OutcomeAccount = &outcomeAccount
		tx.IncomeInstrument = instrumentID
		tx.OutcomeInstrument = instrumentID
	case "transfer":
		tx.OutcomeAccount = &outcomeAccount
		tx.IncomeAccount = toAccountID
		tx.OutcomeInstrument = instrumentID
		tx.IncomeInstrument = toInstrumentID
	}

	if _, ok := args["payee"]; ok {
		tx.Payee = req.GetString("payee", "")
	}
	if _, ok := args["comment"]; ok {
		tx.Comment = strPtr(req.GetString("comment", ""))
	}
	if req.GetBool("clear_payee", false) {
		tx.Payee = ""
	}
	if req.GetBool("clear_comment", false) {
		tx.Comment = nil
	}

	resolution := transactionResolution{
		account:   env.maps.AccountName(accountID),
		toAccount: env.maps.AccountName(toAccountID),
	}

	switch {
	case req.GetBool("clear_category", false):
		tx.Tag = nil
	case requestedExplicit:
		tx.Tag = requestedTagIDs
		resolution.tags = env.maps.TagNames(tx.Tag)
		resolution.categorizedBy = "explicit"
	case autoCategorize:
		tags, source, reviewReason, err := resolveCategoriesForTransaction(ctx, c, env, tx, true)
		if err != nil {
			return models.Transaction{}, transactionResolution{}, err
		}
		if reviewReason != "" {
			resolution.reviewReason = reviewReason
		} else if len(tags) > 0 {
			tx.Tag = tags
			resolution.categorizedBy = source
			resolution.tags = env.maps.TagNames(tags)
		}
	default:
		resolution.tags = env.maps.TagNames(tx.Tag)
	}

	if resolution.tags == nil {
		resolution.tags = env.maps.TagNames(tx.Tag)
	}
	tx.Changed = int(time.Now().Unix())
	return tx, resolution, nil
}

type clientLike interface {
	Suggest(context.Context, models.Transaction) (models.Transaction, error)
}

func applyRequestToTransaction(ctx context.Context, req mcp.CallToolRequest, existing models.Transaction, env *transactionEnv, c clientLike) (models.Transaction, error) {
	args := requestArgs(req)
	tx := existing
	if _, ok := args["account"]; ok {
		return models.Transaction{}, fmt.Errorf("account titles are not accepted for write operations; use account_id")
	}
	if _, ok := args["to_account"]; ok {
		return models.Transaction{}, fmt.Errorf("account titles are not accepted for write operations; use to_account_id")
	}
	accountID := tx.IncomeAccount
	if classifyTx(tx) == "transfer" && tx.OutcomeAccount != nil {
		accountID = *tx.OutcomeAccount
	}

	if ref, ok := args["account_id"]; ok && ref != nil {
		resolved, err := resolveWritableAccountID(req.GetString("account_id", ""), env)
		if err != nil {
			return models.Transaction{}, err
		}
		accountID = resolved
	}

	txType := classifyTx(tx)
	if ref, ok := args["type"]; ok && ref != nil {
		txType = req.GetString("type", "")
	}

	if date := req.GetString("date", ""); date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return models.Transaction{}, fmt.Errorf("invalid date %q: use YYYY-MM-DD", date)
		}
		tx.Date = date
	}

	if req.GetBool("clear_payee", false) {
		tx.Payee = ""
	} else if _, ok := args["payee"]; ok {
		tx.Payee = req.GetString("payee", "")
	}
	if req.GetBool("clear_comment", false) {
		tx.Comment = nil
	} else if _, ok := args["comment"]; ok {
		tx.Comment = strPtr(req.GetString("comment", ""))
	}

	amount := majorAmount(tx)
	if _, ok := args["amount"]; ok {
		amount = req.GetFloat("amount", 0)
		if amount <= 0 {
			return models.Transaction{}, fmt.Errorf("amount must be a positive number")
		}
	}

	toAccountID := tx.IncomeAccount
	if txType == "transfer" {
		if _, ok := args["to_account_id"]; ok {
			resolved, err := resolveWritableAccountID(req.GetString("to_account_id", ""), env)
			if err != nil {
				return models.Transaction{}, err
			}
			toAccountID = resolved
		}
		if toAccountID == "" {
			return models.Transaction{}, fmt.Errorf("to_account_id is required for transfers")
		}
	}

	instrumentID, _ := env.maps.AccountInstrument(accountID)
	if ref, ok := args["currency"]; ok && ref != nil {
		resolved, err := resolveInstrumentRef(req.GetString("currency", ""), env)
		if err != nil {
			return models.Transaction{}, err
		}
		instrumentID = resolved
	}

	switch txType {
	case "expense":
		tx.Income = 0
		tx.Outcome = amount
		tx.IncomeAccount = accountID
		tx.OutcomeAccount = &accountID
		tx.IncomeInstrument = instrumentID
		tx.OutcomeInstrument = instrumentID
	case "income":
		tx.Income = amount
		tx.Outcome = 0
		tx.IncomeAccount = accountID
		tx.OutcomeAccount = &accountID
		tx.IncomeInstrument = instrumentID
		tx.OutcomeInstrument = instrumentID
	case "transfer":
		toInstrumentID, _ := env.maps.AccountInstrument(toAccountID)
		tx.Outcome = amount
		tx.Income = amount
		tx.OutcomeAccount = &accountID
		tx.IncomeAccount = toAccountID
		tx.OutcomeInstrument = instrumentID
		tx.IncomeInstrument = toInstrumentID
	default:
		return models.Transaction{}, fmt.Errorf("unknown type %q: use expense, income, or transfer", txType)
	}

	requestedTags, explicit, err := resolveRequestedTags(req.GetString("category", ""), req.GetString("categories", ""), env)
	if err != nil {
		return models.Transaction{}, err
	}
	if req.GetBool("clear_category", false) {
		tx.Tag = nil
	} else if explicit {
		tx.Tag = requestedTags
	} else if req.GetBool("auto_categorize", false) {
		tags, _, reviewReason, err := resolveCategoriesForTransaction(ctx, c, env, tx, true)
		if err != nil {
			return models.Transaction{}, err
		}
		if reviewReason == "" && len(tags) > 0 {
			tx.Tag = tags
		}
	}

	tx.Changed = int(time.Now().Unix())
	return tx, nil
}

func buildTransactionFromDraft(ctx context.Context, c clientLike, env *transactionEnv, draft importDraft) (*models.Transaction, transactionResolution, error) {
	req := mcp.CallToolRequest{}
	args := map[string]any{
		"type":       draft.Type,
		"date":       draft.Date,
		"amount":     draft.Amount,
		"account_id": draft.AccountID,
	}
	req.Params.Arguments = args
	if draft.Account != "" {
		args["account"] = draft.Account
	}
	if draft.ToAccountID != "" {
		args["to_account_id"] = draft.ToAccountID
	}
	if draft.ToAccount != "" {
		args["to_account"] = draft.ToAccount
	}
	if draft.Category != "" {
		args["category"] = draft.Category
	}
	if draft.Payee != "" {
		args["payee"] = draft.Payee
	}
	if draft.Comment != "" {
		args["comment"] = draft.Comment
	}
	if draft.Currency != "" {
		args["currency"] = draft.Currency
	}
	if draft.Categories != nil {
		switch v := draft.Categories.(type) {
		case string:
			args["categories"] = v
		default:
			raw, _ := json.Marshal(v)
			args["categories"] = string(raw)
		}
	}
	if draft.AutoCategorize != nil {
		args["auto_categorize"] = *draft.AutoCategorize
	}

	tx, resolution, err := buildTransactionFromRequest(ctx, req, env, c, nil)
	if err != nil {
		return nil, transactionResolution{}, err
	}
	return &tx, resolution, nil
}

func resolveAccountRef(ref string, env *transactionEnv) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("account is required")
	}
	if _, ok := env.maps.Accounts[ref]; ok {
		return ref, nil
	}
	for id, title := range env.maps.Accounts {
		if strings.EqualFold(title, ref) {
			return id, nil
		}
	}
	return "", fmt.Errorf("account %q not found", ref)
}

func resolveWritableAccountID(ref string, env *transactionEnv) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("account_id is required")
	}
	if _, ok := env.maps.Accounts[ref]; ok {
		return ref, nil
	}
	return "", fmt.Errorf("account %q not found by ID; use find_accounts to resolve the account ID first", ref)
}

func resolveCategoryRef(ref string, env *transactionEnv) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("category is required")
	}
	if _, ok := env.maps.Tags[ref]; ok {
		return ref, nil
	}
	for id, title := range env.maps.Tags {
		if strings.EqualFold(title, ref) {
			return id, nil
		}
	}
	return "", fmt.Errorf("category %q not found", ref)
}

func resolveInstrumentRef(ref string, env *transactionEnv) (int, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return 0, fmt.Errorf("currency is required")
	}
	if id, err := strconv.Atoi(ref); err == nil {
		for _, instr := range env.resp.Instrument {
			if instr.ID == id {
				return id, nil
			}
		}
	}
	for _, instr := range env.resp.Instrument {
		if strings.EqualFold(instr.Symbol, ref) || strings.EqualFold(instr.ShortTitle, ref) || strings.EqualFold(instr.Title, ref) {
			return instr.ID, nil
		}
	}
	return 0, fmt.Errorf("currency %q not found", ref)
}

func resolveRequestedTags(single string, many string, env *transactionEnv) ([]string, bool, error) {
	var refs []string
	if strings.TrimSpace(single) != "" {
		refs = append(refs, single)
	}
	if strings.TrimSpace(many) != "" {
		list, err := parseFlexibleStringList(many)
		if err != nil {
			return nil, false, fmt.Errorf("invalid categories: %v", err)
		}
		refs = append(refs, list...)
	}
	if len(refs) == 0 {
		return nil, false, nil
	}

	tagIDs := make([]string, 0, len(refs))
	for _, ref := range refs {
		tagID, err := resolveCategoryRef(ref, env)
		if err != nil {
			return nil, false, err
		}
		tagIDs = append(tagIDs, tagID)
	}
	sort.Strings(tagIDs)
	tagIDs = uniqueStrings(tagIDs)
	return tagIDs, true, nil
}

func resolveCategoriesForTransaction(ctx context.Context, c clientLike, env *transactionEnv, tx models.Transaction, assisted bool) ([]string, string, string, error) {
	if !assisted {
		return nil, "", "", nil
	}

	historyMatches := make(map[string][]string)
	key := historicalCategorizationKey(tx)
	for _, candidate := range env.resp.Transaction {
		if candidate.Deleted || len(candidate.Tag) == 0 || classifyTx(candidate) != classifyTx(tx) {
			continue
		}
		if historicalCategorizationKey(candidate) != key {
			continue
		}
		sortedTags := append([]string(nil), candidate.Tag...)
		sort.Strings(sortedTags)
		historyMatches[strings.Join(sortedTags, ",")] = sortedTags
	}

	switch len(historyMatches) {
	case 1:
		for _, tags := range historyMatches {
			if len(tags) == 1 {
				return tags, "history", "", nil
			}
			return nil, "", "multiple categories found in historical match", nil
		}
	case 0:
	default:
		return nil, "", "conflicting historical categories", nil
	}

	suggested, err := c.Suggest(ctx, models.Transaction{
		Payee:   tx.Payee,
		Comment: tx.Comment,
	})
	if err != nil {
		return nil, "", fmt.Sprintf("suggest failed: %v", err), nil
	}
	if len(suggested.Tag) == 1 {
		if err := validateTagIDs(suggested.Tag, env.maps); err != nil {
			return nil, "", "", err
		}
		return suggested.Tag, "zenmoney_suggest", "", nil
	}
	if len(suggested.Tag) > 1 {
		return nil, "", "multiple suggested categories", nil
	}
	return nil, "", "", nil
}

func selectTransactionsForCategorization(req mcp.CallToolRequest, env *transactionEnv) ([]models.Transaction, error) {
	if rawIDs := req.GetString("transaction_ids", ""); rawIDs != "" {
		ids, err := parseFlexibleStringList(rawIDs)
		if err != nil {
			return nil, fmt.Errorf("invalid transaction_ids: %v", err)
		}
		selected := make([]models.Transaction, 0, len(ids))
		for _, id := range ids {
			tx, ok := env.txByID[id]
			if !ok {
				return nil, fmt.Errorf("transaction %q not found", id)
			}
			if tx.Deleted {
				continue
			}
			selected = append(selected, tx)
		}
		return selected, nil
	}

	accountID := ""
	var err error
	if ref := req.GetString("account_id", ""); ref != "" {
		accountID, err = resolveWritableAccountID(ref, env)
		if err != nil {
			return nil, err
		}
	}
	dateFrom := req.GetString("date_from", "")
	dateTo := req.GetString("date_to", "")
	query := strings.ToLower(req.GetString("query", ""))
	uncategorized := req.GetBool("uncategorized", false)
	txType := req.GetString("type", "")

	var selected []models.Transaction
	for _, tx := range env.resp.Transaction {
		if tx.Deleted {
			continue
		}
		if accountID != "" && !transactionTouchesAccount(tx, accountID) {
			continue
		}
		if dateFrom != "" && tx.Date < dateFrom {
			continue
		}
		if dateTo != "" && tx.Date > dateTo {
			continue
		}
		if uncategorized && len(tx.Tag) > 0 {
			continue
		}
		if txType != "" && classifyTx(tx) != txType {
			continue
		}
		if query != "" {
			comment := ""
			if tx.Comment != nil {
				comment = *tx.Comment
			}
			if !strings.Contains(strings.ToLower(tx.Payee), query) && !strings.Contains(strings.ToLower(comment), query) {
				continue
			}
		}
		selected = append(selected, tx)
	}
	return selected, nil
}

func classifyImportDuplicate(tx models.Transaction, env *transactionEnv, policy string) (string, string) {
	exactKey := duplicateKey(tx)
	txDate, _ := time.Parse("2006-01-02", tx.Date)
	for _, existing := range env.resp.Transaction {
		if existing.Deleted || classifyTx(existing) != classifyTx(tx) {
			continue
		}
		if duplicateKey(existing) == exactKey {
			return "exact_duplicate", fmt.Sprintf("matches existing transaction %s", existing.ID)
		}

		existingDate, err := time.Parse("2006-01-02", existing.Date)
		if err != nil {
			continue
		}
		if absDuration(txDate.Sub(existingDate)) > 24*time.Hour {
			continue
		}
		if majorAmount(existing) != majorAmount(tx) {
			continue
		}
		if similarityKey(existing) == similarityKey(tx) && policy != "allow_possible_duplicates" {
			return "possible_duplicate", fmt.Sprintf("similar to existing transaction %s", existing.ID)
		}
	}
	return "new", ""
}

func transactionTouchesAccount(tx models.Transaction, accountID string) bool {
	if tx.IncomeAccount == accountID {
		return true
	}
	return tx.OutcomeAccount != nil && *tx.OutcomeAccount == accountID
}

func majorAmount(tx models.Transaction) float64 {
	if tx.Outcome > tx.Income {
		return tx.Outcome
	}
	return tx.Income
}

func historicalCategorizationKey(tx models.Transaction) string {
	comment := ""
	if tx.Comment != nil {
		comment = *tx.Comment
	}
	return strings.Join([]string{
		classifyTx(tx),
		normalizeText(tx.Payee),
		normalizeText(comment),
	}, "|")
}

func duplicateKey(tx models.Transaction) string {
	comment := ""
	if tx.Comment != nil {
		comment = *tx.Comment
	}
	accountID := tx.IncomeAccount
	if tx.OutcomeAccount != nil {
		accountID = *tx.OutcomeAccount
	}
	return strings.Join([]string{
		accountID,
		tx.Date,
		classifyTx(tx),
		fmt.Sprintf("%.2f", majorAmount(tx)),
		normalizeText(tx.Payee),
		normalizeText(comment),
	}, "|")
}

func similarityKey(tx models.Transaction) string {
	comment := ""
	if tx.Comment != nil {
		comment = *tx.Comment
	}
	return strings.Join([]string{
		classifyTx(tx),
		normalizeText(tx.Payee),
		normalizeText(comment),
	}, "|")
}

func normalizeText(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	replacer := strings.NewReplacer(",", " ", ".", " ", ";", " ", ":", " ", "-", " ", "_", " ", "\t", " ", "\n", " ")
	s = replacer.Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

func parseFlexibleStringList(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "[") {
		var out []string
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return nil, err
		}
		return compactStrings(out), nil
	}
	return compactStrings(strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n'
	})), nil
}

func requestArgs(req mcp.CallToolRequest) map[string]any {
	if args, ok := req.Params.Arguments.(map[string]any); ok {
		return args
	}
	return map[string]any{}
}

func compactStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func uniqueStrings(items []string) []string {
	if len(items) == 0 {
		return items
	}
	out := items[:0]
	seen := map[string]struct{}{}
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
