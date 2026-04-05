package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

type categoryResult struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Parent *string `json:"parent,omitempty"`
}

func toCategoryResult(tag models.Tag, maps LookupMaps) categoryResult {
	var parent *string
	if tag.Parent != nil && *tag.Parent != "" {
		name := maps.Tags[*tag.Parent]
		if name == "" {
			name = *tag.Parent
		}
		parent = &name
	}
	return categoryResult{
		ID:     tag.ID,
		Title:  tag.Title,
		Parent: parent,
	}
}

// RegisterCategoryTools adds find_categories and add_category to the MCP server.
func RegisterCategoryTools(s *server.MCPServer, runtime *RuntimeProvider) {
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
			query := req.GetString("query", "")
			limit := int(req.GetFloat("limit", 20))
			return handleFindCategories(ctx, runtime, query, limit)
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
			return handleAddCategory(ctx, runtime, req)
		},
	)
}

func handleFindCategories(ctx context.Context, runtime *RuntimeProvider, query string, limit int) (*mcp.CallToolResult, error) {
	query = strings.TrimSpace(query)
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	resp, maps, err := runtime.scopedSync(ctx, scopeTags)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	query = strings.ToLower(query)
	results := make([]categoryResult, 0, limit)
	for _, tag := range resp.Tag {
		if query != "" && !strings.Contains(strings.ToLower(tag.Title), query) {
			continue
		}
		results = append(results, toCategoryResult(tag, maps))
		if len(results) >= limit {
			break
		}
	}

	out, err := structJSON(results)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return out, nil
}

func handleAddCategory(ctx context.Context, runtime *RuntimeProvider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title := strings.TrimSpace(req.GetString("title", ""))
	if title == "" {
		return mcp.NewToolResultError("title is required and must not be empty"), nil
	}

	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	resp, maps, err := runtime.scopedSync(ctx, scopeTagsWithUser)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	for _, tag := range resp.Tag {
		if strings.EqualFold(tag.Title, title) {
			out, err := structJSON(toCategoryResult(tag, maps))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return out, nil
		}
	}

	userID := 0
	if len(resp.User) > 0 {
		userID = resp.User[0].ID
	}

	parentID := req.GetString("parent_category", "")
	if parentID != "" {
		resolvedParentID, err := resolveCategoryRef(parentID, &transactionEnv{maps: maps})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		parentID = resolvedParentID
	}

	var parentPtr *string
	if parentID != "" {
		parentPtr = &parentID
	}

	var iconPtr *string
	if icon := req.GetString("icon", ""); icon != "" {
		iconPtr = &icon
	}

	var colorPtr *int64
	if color := req.GetFloat("color", 0); color != 0 {
		c64 := int64(color)
		colorPtr = &c64
	}

	requiredVal := req.GetBool("required", false)
	requiredPtr := &requiredVal

	newTag := models.Tag{
		ID:            uuid.New().String(),
		User:          userID,
		Changed:       int(time.Now().Unix()),
		Title:         title,
		Icon:          iconPtr,
		Color:         colorPtr,
		Parent:        parentPtr,
		ShowIncome:    req.GetBool("show_income", false),
		ShowOutcome:   req.GetBool("show_outcome", true),
		BudgetIncome:  req.GetBool("budget_income", false),
		BudgetOutcome: req.GetBool("budget_outcome", true),
		Required:      requiredPtr,
	}

	pushResp, err := c.Push(ctx, pushTagRequest(runtime.currentServerTimestamp(), []models.Tag{newTag}))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("create category: %v", err)), nil
	}

	if pushResp.ServerTimestamp > 0 {
		runtime.saveServerTimestamp(pushResp.ServerTimestamp)
	}

	maps.Tags[newTag.ID] = newTag.Title

	out, err := structJSON(toCategoryResult(newTag, maps))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return out, nil
}
