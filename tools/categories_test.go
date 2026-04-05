package tools

import (
	"context"
	"testing"

	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

func categorySyncResponse() models.Response {
	return models.Response{
		ServerTimestamp: 1000,
		User:            []models.User{{ID: 10}},
		Tag: []models.Tag{
			{ID: "tag-1", Title: "Food", User: 10},
			{ID: "tag-2", Title: "Transport", User: 10},
		},
	}
}

func TestHandleAddCategory_IdempotentOnExistingTitle(t *testing.T) {
	pushCalled := false
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return categorySyncResponse(), nil
		},
		pushFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			pushCalled = true
			return models.Response{ServerTimestamp: 2000}, nil
		},
	}
	runtime := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"title": "food",
	})

	result, err := handleAddCategory(context.Background(), runtime, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	if pushCalled {
		t.Error("push should not be called when category already exists")
	}
	text := resultText(t, result)
	if !contains(text, "Food") {
		t.Errorf("expected existing category title in response, got: %s", text)
	}
}

func TestHandleAddCategory_CreatesNew(t *testing.T) {
	var pushedTags []models.Tag
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return categorySyncResponse(), nil
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

	result, err := handleAddCategory(context.Background(), runtime, req)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	if len(pushedTags) != 1 {
		t.Fatalf("expected 1 pushed category, got %d", len(pushedTags))
	}
	if pushedTags[0].Title != "Entertainment" {
		t.Errorf("pushed category title = %q, want Entertainment", pushedTags[0].Title)
	}
	if pushedTags[0].User != 10 {
		t.Errorf("pushed category user = %d, want 10", pushedTags[0].User)
	}
}

func TestHandleAddCategory_InvalidParentReturnsError(t *testing.T) {
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return categorySyncResponse(), nil
		},
	}
	runtime := newTestRuntime(mc)

	req := mcpReqWithArgs(map[string]any{
		"title":           "Sub",
		"parent_category": "nonexistent-tag",
	})

	result, err := handleAddCategory(context.Background(), runtime, req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected tool error for nonexistent parent category")
	}
}

func TestHandleFindCategories_ReturnsSubstringMatches(t *testing.T) {
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return categorySyncResponse(), nil
		},
	}
	runtime := newTestRuntime(mc)

	result, err := handleFindCategories(context.Background(), runtime, "foo", 20)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	text := resultText(t, result)
	if !contains(text, "Food") {
		t.Fatalf("expected Food in result, got: %s", text)
	}
	if contains(text, "Transport") {
		t.Fatalf("did not expect Transport in result, got: %s", text)
	}
}

func TestHandleFindCategories_WithoutQueryReturnsList(t *testing.T) {
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return categorySyncResponse(), nil
		},
	}
	runtime := newTestRuntime(mc)

	result, err := handleFindCategories(context.Background(), runtime, "", 20)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	text := resultText(t, result)
	if !contains(text, "Food") || !contains(text, "Transport") {
		t.Fatalf("expected both categories in result, got: %s", text)
	}
}
