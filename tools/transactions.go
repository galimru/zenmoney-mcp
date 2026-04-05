package tools

import (
	"context"
	"fmt"

	"github.com/galimru/zenmoney-mcp/internal/runtime"
	"github.com/galimru/zenmoney-mcp/internal/transactions"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterTransactionTools adds the public transaction workflow tools to the MCP server.
func RegisterTransactionTools(s *server.MCPServer, p *runtime.Provider) {
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
			mcp.WithString("type", mcp.Description("Transaction type: expense, income, or transfer")),
			mcp.WithString("sort", mcp.Description("Sort direction by date: asc or desc (default: desc)")),
			mcp.WithNumber("limit", mcp.Description("Max results (default: 100, max: 500)")),
			mcp.WithNumber("offset", mcp.Description("Items to skip for pagination (default: 0)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleFindTransactions(ctx, p, req)
		},
	)

	s.AddTool(
		mcp.NewTool("list_uncategorized_transactions",
			mcp.WithDescription("List transactions without categories."),
			mcp.WithString("sort", mcp.Description("Sort direction by date: asc or desc (default: desc)")),
			mcp.WithNumber("limit", mcp.Description("Max results (default: 100, max: 500)")),
			mcp.WithNumber("offset", mcp.Description("Items to skip for pagination (default: 0)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleListUncategorizedTransactions(ctx, p, req)
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
			mcp.WithString("category", mcp.Description("Category title or ID")),
			mcp.WithString("categories", mcp.Description("Categories as JSON array string or comma-separated string")),
			mcp.WithString("payee", mcp.Description("Payee name")),
			mcp.WithString("comment", mcp.Description("Comment")),
			mcp.WithString("currency", mcp.Description("Optional currency symbol, code, title, or instrument ID")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleAddTransaction(ctx, p, req)
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
			mcp.WithString("category", mcp.Description("Category title or ID")),
			mcp.WithString("categories", mcp.Description("Categories as JSON array string or comma-separated string")),
			mcp.WithString("payee", mcp.Description("New payee")),
			mcp.WithString("comment", mcp.Description("New comment")),
			mcp.WithString("currency", mcp.Description("Optional currency symbol, code, title, or instrument ID")),
			mcp.WithBoolean("clear_category", mcp.Description("If true, remove all categories")),
			mcp.WithBoolean("clear_payee", mcp.Description("If true, clear payee")),
			mcp.WithBoolean("clear_comment", mcp.Description("If true, clear comment")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleEditTransaction(ctx, p, req)
		},
	)

	s.AddTool(
		mcp.NewTool("remove_transaction",
			mcp.WithDescription("Delete a transaction by ID."),
			mcp.WithString("transaction_id", mcp.Required(), mcp.Description("Transaction ID to delete")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleDeleteTransaction(ctx, p, req.GetString("transaction_id", ""))
		},
	)

	s.AddTool(
		mcp.NewTool("suggest_transaction_categories",
			mcp.WithDescription("Suggest categories for the given transaction IDs."),
			mcp.WithString("transaction_ids", mcp.Required(), mcp.Description("Transaction IDs as JSON array string or comma-separated string")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleSuggestTransactionCategories(ctx, p, req)
		},
	)

}

func handleFindTransactions(ctx context.Context, p *runtime.Provider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	out, err := transactions.NewService(p).Find(ctx, transactions.FindInput{
		DateFrom:  req.GetString("date_from", ""),
		DateTo:    req.GetString("date_to", ""),
		Account:   req.GetString("account", ""),
		Category:  req.GetString("category", ""),
		Query:     req.GetString("query", ""),
		Payee:     req.GetString("payee", ""),
		MinAmount: req.GetFloat("min_amount", 0),
		MaxAmount: req.GetFloat("max_amount", 0),
		Type:      transactions.TxType(req.GetString("type", "")),
		Sort:      req.GetString("sort", "desc"),
		Limit:     int(req.GetFloat("limit", 0)),
		Offset:    int(req.GetFloat("offset", 0)),
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return structJSON(out)
}

func handleListUncategorizedTransactions(ctx context.Context, p *runtime.Provider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	out, err := transactions.NewService(p).ListUncategorized(ctx, transactions.FindInput{
		Sort:   req.GetString("sort", "desc"),
		Limit:  int(req.GetFloat("limit", 0)),
		Offset: int(req.GetFloat("offset", 0)),
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return structJSON(out)
}

func handleAddTransaction(ctx context.Context, p *runtime.Provider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	out, err := transactions.NewService(p).Add(ctx, writeInputFromRequest(req))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return structJSON(out)
}

func handleEditTransaction(ctx context.Context, p *runtime.Provider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	out, err := transactions.NewService(p).Edit(ctx, transactions.EditInput{
		TransactionID: req.GetString("transaction_id", ""),
		WriteInput:    writeInputFromRequest(req),
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if out == nil {
		return mcp.NewToolResultText(fmt.Sprintf("No transaction found with ID %q", req.GetString("transaction_id", ""))), nil
	}
	return structJSON(out)
}

func handleSuggestTransactionCategories(ctx context.Context, p *runtime.Provider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var ids []string
	if rawIDs := req.GetString("transaction_ids", ""); rawIDs != "" {
		parsed, err := transactions.ParseFlexibleStringList(rawIDs)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid transaction_ids: %v", err)), nil
		}
		ids = parsed
	}
	if len(ids) == 0 {
		return mcp.NewToolResultError("transaction_ids is required"), nil
	}

	out, err := transactions.NewService(p).SuggestCategories(ctx, transactions.SuggestCategoriesInput{
		TransactionIDs: ids,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return structJSON(out)
}

func handleDeleteTransaction(ctx context.Context, p *runtime.Provider, txID string) (*mcp.CallToolResult, error) {
	out, err := transactions.NewService(p).Delete(ctx, txID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if out == nil {
		return mcp.NewToolResultText(fmt.Sprintf("No transaction found with ID %q", txID)), nil
	}
	return structJSON(out)
}

func writeInputFromRequest(req mcp.CallToolRequest) transactions.WriteInput {
	args := req.GetArguments()
	_, payeeSet := args["payee"]
	_, commentSet := args["comment"]

	return transactions.WriteInput{
		Type:          transactions.TxType(req.GetString("type", "")),
		Date:          req.GetString("date", ""),
		Amount:        req.GetFloat("amount", 0),
		AccountID:     req.GetString("account_id", ""),
		ToAccountID:   req.GetString("to_account_id", ""),
		Category:      req.GetString("category", ""),
		Categories:    req.GetString("categories", ""),
		Payee:         req.GetString("payee", ""),
		PayeeSet:      payeeSet,
		Comment:       req.GetString("comment", ""),
		CommentSet:    commentSet,
		Currency:      req.GetString("currency", ""),
		ClearCategory: req.GetBool("clear_category", false),
		ClearPayee:    req.GetBool("clear_payee", false),
		ClearComment:  req.GetBool("clear_comment", false),
	}
}
