package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

// RegisterSearchTools adds suggest_category and get_instrument to the MCP server.
func RegisterSearchTools(s *server.MCPServer, runtime *RuntimeProvider) {
	s.AddTool(
		mcp.NewTool("suggest_category",
			mcp.WithDescription("Suggest a category tag for a transaction based on payee and/or comment, resolving returned tag IDs against current ZenMoney categories. ZenMoney does not provide confidence scores."),
			mcp.WithString("payee", mcp.Description("Payee name")),
			mcp.WithString("comment", mcp.Description("Transaction comment")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			payee := req.GetString("payee", "")
			comment := req.GetString("comment", "")
			return handleSuggestCategory(ctx, runtime, payee, comment)
		},
	)

	s.AddTool(
		mcp.NewTool("get_instrument",
			mcp.WithDescription("Fetch current currency instruments from ZenMoney and return the one matching the numeric ID."),
			mcp.WithNumber("id",
				mcp.Required(),
				mcp.Description("Numeric instrument ID"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id := int(req.GetFloat("id", 0))
			return handleGetInstrument(ctx, runtime, id)
		},
	)
}

type suggestResult struct {
	Payee string   `json:"payee,omitempty"`
	Tags  []string `json:"tags"`
}

func handleSuggestCategory(ctx context.Context, runtime *RuntimeProvider, payee, comment string) (*mcp.CallToolResult, error) {
	if payee == "" && comment == "" {
		return mcp.NewToolResultError("at least one of payee or comment must be provided"), nil
	}

	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	// Fetch lookup maps so we can resolve tag IDs to names.
	_, maps, err := runtime.scopedSync(ctx, scopeTags)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	commentPtr := strPtr(comment)
	tx := models.Transaction{
		Payee:   payee,
		Comment: commentPtr,
	}

	suggested, err := c.Suggest(ctx, tx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("suggest failed: %v", err)), nil
	}

	tagNames := maps.TagNames(suggested.Tag)
	if tagNames == nil {
		tagNames = []string{}
	}

	result := suggestResult{
		Payee: suggested.Payee,
		Tags:  tagNames,
	}

	out, err := structJSON(result)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return out, nil
}

func handleGetInstrument(ctx context.Context, runtime *RuntimeProvider, id int) (*mcp.CallToolResult, error) {
	if id <= 0 {
		return mcp.NewToolResultError("id must be a positive integer"), nil
	}

	resp, _, err := runtime.scopedSync(ctx, scopeInstruments)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	for _, instr := range resp.Instrument {
		if instr.ID == id {
			out, err := structJSON(instrumentResult{
				ID:         instr.ID,
				Title:      instr.Title,
				ShortTitle: instr.ShortTitle,
				Symbol:     instr.Symbol,
				Rate:       instr.Rate,
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return out, nil
		}
	}
	return mcp.NewToolResultText(fmt.Sprintf("No instrument found with ID %d", id)), nil
}
