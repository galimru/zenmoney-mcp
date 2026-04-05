package tools

import (
	"context"
	"os"
	"time"

	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
	"github.com/galimru/zenmoney-mcp/client"
	"github.com/galimru/zenmoney-mcp/internal/config"
	"github.com/galimru/zenmoney-mcp/store"
)

// mockZenClient implements client.ZenClient for testing.
type mockZenClient struct {
	fullSyncFn   func(ctx context.Context) (models.Response, error)
	syncSinceFn  func(ctx context.Context, since time.Time) (models.Response, error)
	pushFn       func(ctx context.Context, req models.Request) (models.Response, error)
	suggestFn    func(ctx context.Context, tx models.Transaction) (models.Transaction, error)
}

func (m *mockZenClient) FullSync(ctx context.Context) (models.Response, error) {
	if m.fullSyncFn != nil {
		return m.fullSyncFn(ctx)
	}
	return models.Response{ServerTimestamp: 1000}, nil
}

func (m *mockZenClient) SyncSince(ctx context.Context, since time.Time) (models.Response, error) {
	if m.syncSinceFn != nil {
		return m.syncSinceFn(ctx, since)
	}
	// Default: fall back to FullSync behaviour so tests that call a handler multiple times
	// still see data on the second call (which triggers SyncSince after state is saved).
	if m.fullSyncFn != nil {
		return m.fullSyncFn(ctx)
	}
	return models.Response{ServerTimestamp: 2000}, nil
}

func (m *mockZenClient) Push(ctx context.Context, req models.Request) (models.Response, error) {
	if m.pushFn != nil {
		return m.pushFn(ctx, req)
	}
	return models.Response{ServerTimestamp: 3000}, nil
}

func (m *mockZenClient) Suggest(ctx context.Context, tx models.Transaction) (models.Transaction, error) {
	if m.suggestFn != nil {
		return m.suggestFn(ctx, tx)
	}
	return tx, nil
}

// newTestRuntime creates a RuntimeProvider wired to a mock ZenClient and a unique temp store.
// Each call creates a fresh store in a new temp directory to prevent cross-test contamination.
func newTestRuntime(mc *mockZenClient) *RuntimeProvider {
	dir, _ := os.MkdirTemp("", "zenmoney-mcp-test-*")
	p := &RuntimeProvider{
		loadConfig: func() (*config.Config, error) {
			return &config.Config{
				TransactionLimit:  100,
				MaxBulkOperations: 20,
			}, nil
		},
		newZenClient: func(token string) (client.ZenClient, error) {
			return mc, nil
		},
		zenStore: store.New(dir + "/sync_state.json"),
	}
	p.preparations.data = make(map[string]*PreparedBulk)
	return p
}
