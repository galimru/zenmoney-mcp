package tools

import (
	"context"
	"testing"
	"time"

	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
	"github.com/galimru/zenmoney-mcp/store"
)

func TestHandleSync_UsesFullSyncWhenNoState(t *testing.T) {
	fullSyncCalled := false
	syncSinceCalled := false

	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			fullSyncCalled = true
			return models.Response{ServerTimestamp: 5000, Account: []models.Account{{ID: "a1", Title: "Cash"}}}, nil
		},
		syncSinceFn: func(ctx context.Context, since time.Time) (models.Response, error) {
			syncSinceCalled = true
			return models.Response{ServerTimestamp: 6000}, nil
		},
	}

	runtime := newTestRuntime(mc)
	result, err := handleSync(context.Background(), runtime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result")
	}
	if !fullSyncCalled {
		t.Error("expected FullSync to be called when no sync state exists")
	}
	if syncSinceCalled {
		t.Error("SyncSince should not be called when no sync state exists")
	}
}

func TestHandleSync_UsesSyncSinceWhenStateExists(t *testing.T) {
	syncSinceCalled := false
	var syncSinceArg time.Time

	mc := &mockZenClient{
		syncSinceFn: func(ctx context.Context, since time.Time) (models.Response, error) {
			syncSinceCalled = true
			syncSinceArg = since
			return models.Response{ServerTimestamp: 7000}, nil
		},
	}

	runtime := newTestRuntime(mc)
	// Pre-populate sync state.
	_ = runtime.zenStore.Save(&store.SyncState{
		ServerTimestamp: 5000,
		LastSyncAt:      time.Now(),
	})

	_, err := handleSync(context.Background(), runtime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !syncSinceCalled {
		t.Error("expected SyncSince to be called when sync state exists")
	}
	expected := time.Unix(5000, 0)
	if !syncSinceArg.Equal(expected) {
		t.Errorf("SyncSince called with %v, want %v", syncSinceArg, expected)
	}
}

func TestHandleFullSync_ResetsStateAndCallsFullSync(t *testing.T) {
	fullSyncCalled := false

	mc := &mockZenClient{
		fullSyncFn: func(ctx context.Context) (models.Response, error) {
			fullSyncCalled = true
			return models.Response{ServerTimestamp: 9999}, nil
		},
	}

	runtime := newTestRuntime(mc)
	// Pre-populate state; full_sync should discard it.
	_ = runtime.zenStore.Save(&store.SyncState{ServerTimestamp: 1234, LastSyncAt: time.Now()})

	result, err := handleFullSync(context.Background(), runtime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatal("expected successful result")
	}
	if !fullSyncCalled {
		t.Error("expected FullSync to be called")
	}
	// State should now reflect new timestamp.
	state, ok := runtime.zenStore.Get()
	if !ok || state.ServerTimestamp != 9999 {
		t.Errorf("expected server timestamp 9999, got %+v", state)
	}
}
