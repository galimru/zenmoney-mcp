package tools

import (
	"context"

	"github.com/galimru/zenmoney-mcp/internal/categories"
	"github.com/galimru/zenmoney-mcp/internal/runtime"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterCategoryTools adds find_categories and add_category to the MCP server.
func RegisterCategoryTools(s *server.MCPServer, p *runtime.Provider) {
	s.AddTool(
		mcp.NewTool("find_categories",
			mcp.WithDescription("Find categories by title. If query is omitted, return categories up to the limit."),
			mcp.WithString("query",
				mcp.Description("Optional case-insensitive substring to search for in category titles"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Maximum number of matches to return (default: 20, max: 100)"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleFindCategories(ctx, p, req)
		},
	)

	s.AddTool(
		mcp.NewTool("add_category",
			mcp.WithDescription("Create a transaction category if needed. If a category with the same title already exists, return the existing category."),
			mcp.WithString("title",
				mcp.Required(),
				mcp.Description("Category title"),
			),
			mcp.WithString("parent_category",
				mcp.Description("Parent category title or ID (for nested categories, max one level deep)"),
			),
			mcp.WithString("icon", mcp.Description("Icon ID")),
			mcp.WithNumber("color", mcp.Description("Icon color as ARGB integer")),
			mcp.WithBoolean("show_income", mcp.Description("Show in income categories (default: false)")),
			mcp.WithBoolean("show_outcome", mcp.Description("Show in expense categories (default: true)")),
			mcp.WithBoolean("budget_income", mcp.Description("Include in income budget (default: false)")),
			mcp.WithBoolean("budget_outcome", mcp.Description("Include in expense budget (default: true)")),
			mcp.WithBoolean("required", mcp.Description("Mark expenses as required/mandatory")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleAddCategory(ctx, p, req)
		},
	)
}

func handleFindCategories(ctx context.Context, p *runtime.Provider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	svc := categories.NewService(p)
	results, err := svc.Find(ctx, categories.FindInput{
		Query: req.GetString("query", ""),
		Limit: int(req.GetFloat("limit", 20)),
	})
	if err != nil {
		return runtimeError(err), nil
	}
	return structJSON(results)
}

func handleAddCategory(ctx context.Context, p *runtime.Provider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var colorPtr *int64
	if color := req.GetFloat("color", 0); color != 0 {
		c64 := int64(color)
		colorPtr = &c64
	}
	requiredVal := req.GetBool("required", false)

	svc := categories.NewService(p)
	result, err := svc.Add(ctx, categories.AddInput{
		Title:          req.GetString("title", ""),
		ParentCategory: req.GetString("parent_category", ""),
		Icon:           req.GetString("icon", ""),
		Color:          colorPtr,
		ShowIncome:     req.GetBool("show_income", false),
		ShowOutcome:    req.GetBool("show_outcome", true),
		BudgetIncome:   req.GetBool("budget_income", false),
		BudgetOutcome:  req.GetBool("budget_outcome", true),
		Required:       &requiredVal,
	})
	if err != nil {
		return runtimeError(err), nil
	}
	return structJSON(result)
}
