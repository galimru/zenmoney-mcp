package tools

import (
	"context"
	"fmt"
	"strings"

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

func toAccountResult(acc models.Account, maps LookupMaps) accountResult {
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

// RegisterAccountTools adds list_accounts and find_account to the MCP server.
func RegisterAccountTools(s *server.MCPServer, runtime *RuntimeProvider) {
	s.AddTool(
		mcp.NewTool("list_accounts",
			mcp.WithDescription("Fetch and list current financial accounts from ZenMoney. Set active_only=true to exclude archived accounts."),
			mcp.WithBoolean("active_only",
				mcp.Description("If true, exclude archived accounts (default: false)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			activeOnly := req.GetBool("active_only", false)
			return handleListAccounts(ctx, runtime, activeOnly)
		},
	)

	s.AddTool(
		mcp.NewTool("find_account",
			mcp.WithDescription("Fetch current accounts from ZenMoney and return the first title match (case-insensitive)."),
			mcp.WithString("title",
				mcp.Required(),
				mcp.Description("Account title to search for"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			title := req.GetString("title", "")
			return handleFindAccount(ctx, runtime, title)
		},
	)
}

func handleListAccounts(ctx context.Context, runtime *RuntimeProvider, activeOnly bool) (*mcp.CallToolResult, error) {
	resp, maps, err := runtime.scopedSync(ctx, scopeAccounts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	results := make([]accountResult, 0, len(resp.Account))
	for _, acc := range resp.Account {
		if activeOnly && acc.Archive {
			continue
		}
		results = append(results, toAccountResult(acc, maps))
	}

	out, err := structJSON(results)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return out, nil
}

func handleFindAccount(ctx context.Context, runtime *RuntimeProvider, title string) (*mcp.CallToolResult, error) {
	resp, maps, err := runtime.scopedSync(ctx, scopeAccounts)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	for _, acc := range resp.Account {
		if strings.EqualFold(acc.Title, title) {
			out, err := structJSON(toAccountResult(acc, maps))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return out, nil
		}
	}
	return mcp.NewToolResultText(fmt.Sprintf("No account found with title %q", title)), nil
}
