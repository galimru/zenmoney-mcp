package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/galimru/zenmoney-mcp/internal/runtime"
	"github.com/galimru/zenmoney-mcp/internal/transactions"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterImportTools adds the public transaction import workflow tool to the MCP server.
func RegisterImportTools(s *server.MCPServer, p *runtime.Provider) {
	s.AddTool(
		mcp.NewTool("import_transactions",
			mcp.WithDescription("Import a batch of structured transaction rows into one account. Provide account_id and rows. Each row must include date (YYYY-MM-DD), amount (positive number), and type (expense, income, or transfer). Optional fields are payee, comment, category, external_id, and to_account_id for transfers. The tool validates the whole batch before importing. If any row has an issue, the response lists row indexes and reasons so you can fix the rows and retry the batch."),
			mcp.WithString("account_id", mcp.Required(), mcp.Description("Account ID for all imported transactions. Use find_accounts first if needed.")),
			mcp.WithArray("rows",
				mcp.Description("Structured transaction rows. Each row must include date (YYYY-MM-DD), amount (positive number), and type (expense, income, or transfer). Optional fields: payee, comment, category, external_id, and to_account_id for transfers."),
				mcp.Items(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"external_id":   map[string]any{"type": "string", "description": "Optional caller-provided row identifier echoed back in issue responses."},
						"date":          map[string]any{"type": "string", "description": "Transaction date in YYYY-MM-DD format."},
						"amount":        map[string]any{"type": "number", "description": "Positive transaction amount."},
						"type":          map[string]any{"type": "string", "enum": []string{"expense", "income", "transfer"}},
						"payee":         map[string]any{"type": "string"},
						"comment":       map[string]any{"type": "string"},
						"category":      map[string]any{"type": "string", "description": "Category title or ID."},
						"to_account_id": map[string]any{"type": "string", "description": "Required for transfer rows."},
					},
					"required":             []string{"date", "amount", "type"},
					"additionalProperties": false,
				}),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleImportTransactions(ctx, p, req)
		},
	)
}

func handleImportTransactions(ctx context.Context, p *runtime.Provider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()

	in := transactions.ImportTransactionsInput{
		AccountID: req.GetString("account_id", ""),
	}

	rows, err := decodeImportRows(args["rows"])
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	in.Items = rows

	out, err := transactions.NewService(p).ImportTransactions(ctx, in)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return structJSON(out)
}

func decodeImportRows(raw any) ([]transactions.ImportDraft, error) {
	if raw == nil {
		return nil, fmt.Errorf("rows is required")
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid rows: %v", err)
	}

	var rows []transactions.ImportDraft
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("invalid rows: %v", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("rows is required")
	}
	return rows, nil
}
