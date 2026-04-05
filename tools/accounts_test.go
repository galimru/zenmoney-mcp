package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

func TestHandleListAccounts_FilterActiveOnly(t *testing.T) {
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			instr := int32(1)
			return models.Response{
				ServerTimestamp: 1000,
				Instrument: []models.Instrument{{ID: 1, Symbol: "₽"}},
				Account: []models.Account{
					{ID: "a1", Title: "Active", Instrument: &instr, Archive: false},
					{ID: "a2", Title: "Archived", Instrument: &instr, Archive: true},
				},
			}, nil
		},
	}

	runtime := newTestRuntime(mc)

	// Without filter: both accounts returned.
	result, err := handleListAccounts(context.Background(), runtime, false)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}

	// With filter: only active account.
	result, err = handleListAccounts(context.Background(), runtime, true)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error with active_only: %v / %v", err, result)
	}
	// The result JSON should only contain "Active", not "Archived".
	text := resultText(t, result)
	if contains(text, "Archived") {
		t.Error("archived account should not appear when active_only=true")
	}
	if !contains(text, "Active") {
		t.Error("active account should appear when active_only=true")
	}
}

func TestHandleFindAccount_CaseInsensitive(t *testing.T) {
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return models.Response{
				ServerTimestamp: 1000,
				Account: []models.Account{
					{ID: "a1", Title: "My Cash"},
				},
			}, nil
		},
	}
	runtime := newTestRuntime(mc)

	result, err := handleFindAccount(context.Background(), runtime, "my cash")
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	text := resultText(t, result)
	if !contains(text, "My Cash") {
		t.Error("expected to find account by case-insensitive title")
	}
}

func TestHandleFindAccount_NotFound(t *testing.T) {
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return models.Response{ServerTimestamp: 1000}, nil
		},
	}
	runtime := newTestRuntime(mc)

	result, err := handleFindAccount(context.Background(), runtime, "nonexistent")
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	text := resultText(t, result)
	if !contains(text, "not found") && !contains(text, "No account") {
		t.Errorf("expected not-found message, got: %s", text)
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
