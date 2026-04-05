package tools

import (
	"errors"
	"testing"

	"github.com/galimru/zenmoney-mcp/client"
	"github.com/galimru/zenmoney-mcp/internal/config"
	"github.com/galimru/zenmoney-mcp/store"
)

func TestRuntimeProvider_ConfigCachedAfterSuccess(t *testing.T) {
	calls := 0
	p := &RuntimeProvider{
		loadConfig: func() (*config.Config, error) {
			calls++
			return &config.Config{TransactionLimit: 100, MaxBulkOperations: 20}, nil
		},
		newZenClient: func(token string) (client.ZenClient, error) {
			return &mockZenClient{}, nil
		},
		zenStore: store.New("/tmp/test-rp-sync.json"),
	}
	p.preparations.data = make(map[string]*PreparedBulk)

	// Two calls should only invoke loadConfig once.
	_, err := p.config()
	if err != nil {
		t.Fatalf("first config() failed: %v", err)
	}
	_, err = p.config()
	if err != nil {
		t.Fatalf("second config() failed: %v", err)
	}
	if calls != 1 {
		t.Errorf("loadConfig called %d times, want 1", calls)
	}
}

func TestRuntimeProvider_ConfigRetriedAfterFailure(t *testing.T) {
	calls := 0
	p := &RuntimeProvider{
		loadConfig: func() (*config.Config, error) {
			calls++
			if calls < 3 {
				return nil, errors.New("not ready yet")
			}
			return &config.Config{TransactionLimit: 100, MaxBulkOperations: 20}, nil
		},
		newZenClient: func(token string) (client.ZenClient, error) {
			return &mockZenClient{}, nil
		},
		zenStore: store.New("/tmp/test-rp-sync2.json"),
	}
	p.preparations.data = make(map[string]*PreparedBulk)

	// Fail twice, then succeed.
	_, err := p.config()
	if err == nil {
		t.Fatal("expected error on first call")
	}
	_, err = p.config()
	if err == nil {
		t.Fatal("expected error on second call")
	}
	cfg, err := p.config()
	if err != nil || cfg == nil {
		t.Fatalf("expected success on third call, got %v / %v", cfg, err)
	}
	if calls != 3 {
		t.Errorf("loadConfig called %d times, want 3", calls)
	}
}

func TestRuntimeProvider_ApiClientCachedAfterSuccess(t *testing.T) {
	clientCalls := 0
	p := &RuntimeProvider{
		loadConfig: func() (*config.Config, error) {
			return &config.Config{TransactionLimit: 100, MaxBulkOperations: 20}, nil
		},
		newZenClient: func(token string) (client.ZenClient, error) {
			clientCalls++
			return &mockZenClient{}, nil
		},
		zenStore: store.New("/tmp/test-rp-sync3.json"),
	}
	p.preparations.data = make(map[string]*PreparedBulk)

	_, _ = p.apiClient()
	_, _ = p.apiClient()

	if clientCalls != 1 {
		t.Errorf("newZenClient called %d times, want 1", clientCalls)
	}
}
