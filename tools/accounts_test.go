package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

func TestHandleListAccounts_HidesArchivedByDefault(t *testing.T) {
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			instr := int32(1)
			return models.Response{
				ServerTimestamp: 1000,
				Instrument:      []models.Instrument{{ID: 1, Symbol: "₽"}},
				Account: []models.Account{
					{ID: "a1", Title: "Active", Instrument: &instr, Archive: false},
					{ID: "a2", Title: "Archived", Instrument: &instr, Archive: true},
				},
			}, nil
		},
		syncFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			instr := int32(1)
			return models.Response{
				ServerTimestamp: req.ServerTimestamp + 1,
				Instrument:      []models.Instrument{{ID: 1, Symbol: "₽"}},
				Account: []models.Account{
					{ID: "a1", Title: "Active", Instrument: &instr, Archive: false},
					{ID: "a2", Title: "Archived", Instrument: &instr, Archive: true},
				},
			}, nil
		},
	}

	runtime := newTestRuntime(mc)

	// Default: archived accounts are hidden.
	result, err := handleListAccounts(context.Background(), runtime, false)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	text := resultText(t, result)
	if contains(text, "Archived") {
		t.Error("archived account should not appear by default")
	}
	if !contains(text, "Active") {
		t.Error("active account should appear by default")
	}

	// When requested: archived accounts are included.
	result, err = handleListAccounts(context.Background(), runtime, true)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error with show_archived: %v / %v", err, result)
	}
	text = resultText(t, result)
	if !contains(text, "Archived") {
		t.Error("archived account should appear when show_archived=true")
	}
}

func TestHandleFindAccounts_CaseInsensitive(t *testing.T) {
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

	result, err := handleFindAccounts(context.Background(), runtime, "my cash", 20)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	text := resultText(t, result)
	if !contains(text, "My Cash") {
		t.Error("expected to find account by case-insensitive title")
	}
}

func TestHandleFindAccounts_NotFound(t *testing.T) {
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return models.Response{ServerTimestamp: 1000}, nil
		},
	}
	runtime := newTestRuntime(mc)

	result, err := handleFindAccounts(context.Background(), runtime, "nonexistent", 20)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	text := resultText(t, result)
	if text != "[]" && !contains(text, "[]") {
		t.Errorf("expected empty result array, got: %s", text)
	}
}

func TestHandleFindAccounts_ExactMatchesBeforePartialMatches(t *testing.T) {
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			return models.Response{
				ServerTimestamp: 1000,
				Account: []models.Account{
					{ID: "a1", Title: "Cash"},
					{ID: "a2", Title: "Main Cash"},
					{ID: "a3", Title: "Cash Reserve"},
				},
			}, nil
		},
	}
	runtime := newTestRuntime(mc)

	result, err := handleFindAccounts(context.Background(), runtime, "cash", 20)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %v", err, result)
	}
	text := resultText(t, result)
	firstCash := strings.Index(text, "\"title\": \"Cash\"")
	mainCash := strings.Index(text, "\"title\": \"Main Cash\"")
	if firstCash == -1 || mainCash == -1 {
		t.Fatalf("expected both Cash and Main Cash in result, got: %s", text)
	}
	if firstCash > mainCash {
		t.Fatalf("expected exact match to appear before partial match, got: %s", text)
	}
}

func TestHandleListAccounts_UsesForceFetchAfterStateExists(t *testing.T) {
	t.Setenv("ZENMONEY_TOKEN", "token-a")

	var syncCalls int
	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			instr := int32(1)
			return models.Response{
				ServerTimestamp: 1000,
				Instrument:      []models.Instrument{{ID: 1, Symbol: "₽"}},
				Account:         []models.Account{{ID: "a1", Title: "Active", Instrument: &instr}},
			}, nil
		},
		syncFn: func(ctx context.Context, req models.Request) (models.Response, error) {
			syncCalls++
			if req.ServerTimestamp != 1000 {
				t.Fatalf("ServerTimestamp = %d, want 1000", req.ServerTimestamp)
			}
			if len(req.ForceFetch) != 2 || req.ForceFetch[0] != models.EntityTypeAccount || req.ForceFetch[1] != models.EntityTypeInstrument {
				t.Fatalf("ForceFetch = %v, want [account instrument]", req.ForceFetch)
			}
			instr := int32(1)
			return models.Response{
				ServerTimestamp: 1001,
				Instrument:      []models.Instrument{{ID: 1, Symbol: "₽"}},
				Account:         []models.Account{{ID: "a1", Title: "Active", Instrument: &instr}},
			}, nil
		},
	}

	runtime := newTestRuntime(mc)

	first, err := handleListAccounts(context.Background(), runtime, false)
	if err != nil || first.IsError {
		t.Fatalf("unexpected error on first call: %v / %v", err, first)
	}

	second, err := handleListAccounts(context.Background(), runtime, false)
	if err != nil || second.IsError {
		t.Fatalf("unexpected error on second call: %v / %v", err, second)
	}
	if syncCalls != 1 {
		t.Fatalf("syncFn calls = %d, want 1", syncCalls)
	}
	if !contains(resultText(t, second), "Active") {
		t.Fatal("expected account to still be returned on second call")
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
