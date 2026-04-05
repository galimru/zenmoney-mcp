package tools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

func tagSyncResponse() models.Response {
	return models.Response{
		ServerTimestamp: 1000,
		User:            []models.User{{ID: 10}},
		Tag: []models.Tag{
			{ID: "tag-1", Title: "Food", User: 10},
			{ID: "tag-2", Title: "Transport", User: 10},
		},
	}
}

func TestHandleCreateTag_IdempotentOnExistingTitle(t *testing.T) {
	pushCalled := false
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return tagSyncResponse(), nil
		},
		pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			pushCalled = true
			return models.Response{ServerTimestamp: 2000}, nil
		},
	}
	runtime := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"title": "food", // same as existing "Food", different case
	})

	result, err := handleCreateTag(context.Background(), runtime, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	if pushCalled {
		t.Error("push should not be called when tag already exists")
	}
	text := resultText(t, result)
	if !contains(text, "Food") {
		t.Errorf("expected existing tag title in response, got: %s", text)
	}
}

func TestHandleCreateTag_CreatesNew(t *testing.T) {
	var pushedTags []models.Tag
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return tagSyncResponse(), nil
		},
		pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			pushedTags = req.Tag
			return models.Response{ServerTimestamp: 2000}, nil
		},
	}
	runtime := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"title":        "Entertainment",
		"show_outcome": true,
	})

	result, err := handleCreateTag(context.Background(), runtime, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	if len(pushedTags) != 1 {
		t.Fatalf("expected 1 pushed tag, got %d", len(pushedTags))
	}
	if pushedTags[0].Title != "Entertainment" {
		t.Errorf("pushed tag title = %q, want Entertainment", pushedTags[0].Title)
	}
	if pushedTags[0].User != 10 {
		t.Errorf("pushed tag user = %d, want 10", pushedTags[0].User)
	}
}

func TestHandleCreateTag_InvalidParentReturnsError(t *testing.T) {
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return tagSyncResponse(), nil
		},
	}
	runtime := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"title":         "Sub",
		"parent_tag_id": "nonexistent-tag",
	})

	result, err := handleCreateTag(context.Background(), runtime, req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected tool error for nonexistent parent tag")
	}
}

// mcpReqWithArgs builds a minimal CallToolRequest with the given arguments.
func mcpReqWithArgs(args map[string]any) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	return req
}
