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
	"github.com/galimru/zenmoney-mcp/store"
)

type tagResult struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Parent *string `json:"parent,omitempty"` // resolved parent tag title
}

func toTagResult(tag models.Tag, maps LookupMaps) tagResult {
	var parent *string
	if tag.Parent != nil && *tag.Parent != "" {
		name := maps.Tags[*tag.Parent]
		if name == "" {
			name = *tag.Parent
		}
		parent = &name
	}
	return tagResult{
		ID:     tag.ID,
		Title:  tag.Title,
		Parent: parent,
	}
}

// RegisterTagTools adds list_tags, find_tag, and create_tag to the MCP server.
func RegisterTagTools(s *server.MCPServer, runtime *RuntimeProvider) {
	s.AddTool(
		mcp.NewTool("list_tags",
			mcp.WithDescription("List all transaction category tags."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleListTags(ctx, runtime)
		},
	)

	s.AddTool(
		mcp.NewTool("find_tag",
			mcp.WithDescription("Find a category tag by title (case-insensitive). Returns the first match."),
			mcp.WithString("title",
				mcp.Required(),
				mcp.Description("Tag title to search for"),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			title := req.GetString("title", "")
			return handleFindTag(ctx, runtime, title)
		},
	)

	s.AddTool(
		mcp.NewTool("create_tag",
			mcp.WithDescription("Create a new category tag (also used for expense/income categories). Idempotent: returns the existing tag if one with the same title already exists (case-insensitive)."),
			mcp.WithString("title",
				mcp.Required(),
				mcp.Description("Tag title"),
			),
			mcp.WithString("parent_tag_id",
				mcp.Description("Parent tag ID (for nested categories, max one level deep)"),
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
			return handleCreateTag(ctx, runtime, req)
		},
	)
}

func handleListTags(ctx context.Context, runtime *RuntimeProvider) (*mcp.CallToolResult, error) {
	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	resp, maps, err := fetchSyncResponse(ctx, c, runtime.zenStore)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	results := make([]tagResult, len(resp.Tag))
	for i, tag := range resp.Tag {
		results[i] = toTagResult(tag, maps)
	}

	out, err := structJSON(results)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return out, nil
}

func handleFindTag(ctx context.Context, runtime *RuntimeProvider, title string) (*mcp.CallToolResult, error) {
	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	resp, maps, err := fetchSyncResponse(ctx, c, runtime.zenStore)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	for _, tag := range resp.Tag {
		if strings.EqualFold(tag.Title, title) {
			out, err := structJSON(toTagResult(tag, maps))
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return out, nil
		}
	}
	return mcp.NewToolResultText(fmt.Sprintf("No tag found with title %q", title)), nil
}

func handleCreateTag(ctx context.Context, runtime *RuntimeProvider, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title := strings.TrimSpace(req.GetString("title", ""))
	if title == "" {
		return mcp.NewToolResultError("title is required and must not be empty"), nil
	}

	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	resp, maps, err := fetchSyncResponse(ctx, c, runtime.zenStore)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	// Idempotent: return existing tag if title matches.
	for _, tag := range resp.Tag {
		if strings.EqualFold(tag.Title, title) {
			out, err := structJSON(toTagResult(tag, maps))
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

	// Validate parent tag if provided.
	parentID := req.GetString("parent_tag_id", "")
	if parentID != "" {
		if _, ok := maps.Tags[parentID]; !ok {
			return mcp.NewToolResultError(fmt.Sprintf("parent tag %q not found", parentID)), nil
		}
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

	pushResp, err := c.Push(ctx, pushTagRequest(currentServerTimestamp(runtime.zenStore), []models.Tag{newTag}))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("create tag: %v", err)), nil
	}

	if pushResp.ServerTimestamp > 0 {
		_ = runtime.zenStore.Save(&store.SyncState{ServerTimestamp: pushResp.ServerTimestamp, LastSyncAt: time.Now()})
	}

	// Add new tag to maps for parent resolution in response.
	maps.Tags[newTag.ID] = newTag.Title

	out, err := structJSON(toTagResult(newTag, maps))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return out, nil
}
