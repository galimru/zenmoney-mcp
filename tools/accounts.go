package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/galimru/zenmoney-mcp/internal/runtime"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

type accountResult struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Type      string   `json:"type"`
	Balance   *float64 `json:"balance"`
	Currency  string   `json:"currency"`
	Archive   bool     `json:"archive"`
	InBalance bool     `json:"in_balance"`
}

func toAccountResult(acc models.Account, maps runtime.LookupMaps) accountResult {
	currency := ""
	if acc.Instrument != nil {
		currency = maps.InstrumentSymbol(int(*acc.Instrument))
	}
	return accountResult{
		ID:        acc.ID,
		Title:     acc.Title,
		Type:      acc.Type,
		Balance:   acc.Balance,
		Currency:  currency,
		Archive:   acc.Archive,
		InBalance: acc.InBalance,
	}
}

// RegisterAccountTools adds list_accounts and find_accounts to the MCP server.
func RegisterAccountTools(s *server.MCPServer, p *runtime.Provider) {
	s.AddTool(
		mcp.NewTool("list_accounts",
			mcp.WithDescription("Fetch and list current financial accounts from ZenMoney. Archived accounts are hidden by default."),
			mcp.WithBoolean("show_archived",
				mcp.Description("If true, include archived accounts (default: false)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			showArchived := req.GetBool("show_archived", false)
			return handleListAccounts(ctx, p, showArchived)
		},
	)

	s.AddTool(
		mcp.NewTool("find_accounts",
			mcp.WithDescription("Find accounts by title. Exact title matches are returned first, followed by substring matches."),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("Case-insensitive title query"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Maximum number of matches to return (default: 20, max: 100)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query := req.GetString("query", "")
			limit := int(req.GetFloat("limit", 20))
			return handleFindAccounts(ctx, p, query, limit)
		},
	)
}

func handleListAccounts(ctx context.Context, p *runtime.Provider, showArchived bool) (*mcp.CallToolResult, error) {
	resp, maps, err := p.ScopedSync(ctx, runtime.ScopeAccounts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	results := make([]accountResult, 0, len(resp.Account))
	for _, acc := range resp.Account {
		if !showArchived && acc.Archive {
			continue
		}
		results = append(results, toAccountResult(acc, maps))
	}

	return structJSON(results)
}

func handleFindAccounts(ctx context.Context, p *runtime.Provider, query string, limit int) (*mcp.CallToolResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	resp, maps, err := p.ScopedSync(ctx, runtime.ScopeAccounts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	queryLower := strings.ToLower(query)
	exact := make([]accountResult, 0, limit)
	partial := make([]accountResult, 0, limit)
	for _, acc := range resp.Account {
		titleLower := strings.ToLower(acc.Title)
		result := toAccountResult(acc, maps)
		switch {
		case strings.EqualFold(acc.Title, query):
			exact = append(exact, result)
		case strings.Contains(titleLower, queryLower):
			partial = append(partial, result)
		}
	}

	results := append(exact, partial...)
	if len(results) > limit {
		results = results[:limit]
	}
	return structJSON(results)
}
