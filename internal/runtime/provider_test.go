package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/galimru/zenmoney-mcp/client"
	"github.com/galimru/zenmoney-mcp/internal/config"
	"github.com/galimru/zenmoney-mcp/store"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

type stubClient struct{}

func (stubClient) FullSync(context.Context) (models.Response, error) {
	return models.Response{}, nil
}

func (stubClient) SyncSince(context.Context, time.Time) (models.Response, error) {
	return models.Response{}, nil
}

func (stubClient) Sync(context.Context, models.Request) (models.Response, error) {
	return models.Response{}, nil
}

func (stubClient) Push(context.Context, models.Request) (models.Response, error) {
	return models.Response{}, nil
}

func (stubClient) Suggest(context.Context, models.Transaction) (models.Transaction, error) {
	return models.Transaction{}, nil
}

var _ client.ZenClient = stubClient{}

func TestProviderConfigCachedAfterSuccess(t *testing.T) {
	calls := 0
	p := NewProviderWithDeps(
		func() (*config.Config, error) {
			calls++
			return &config.Config{TransactionLimit: 100}, nil
		},
		func(string) (client.ZenClient, error) { return stubClient{}, nil },
		store.New(t.TempDir()+"/sync.json"),
		nil,
	)

	_, err := p.Config()
	if err != nil {
		t.Fatalf("first Config() failed: %v", err)
	}
	_, err = p.Config()
	if err != nil {
		t.Fatalf("second Config() failed: %v", err)
	}
	if calls != 1 {
		t.Fatalf("loadConfig calls = %d, want 1", calls)
	}
}

func TestProviderConfigRetriedAfterFailure(t *testing.T) {
	calls := 0
	p := NewProviderWithDeps(
		func() (*config.Config, error) {
			calls++
			if calls < 3 {
				return nil, errors.New("not ready")
			}
			return &config.Config{TransactionLimit: 100}, nil
		},
		func(string) (client.ZenClient, error) { return stubClient{}, nil },
		store.New(t.TempDir()+"/sync.json"),
		nil,
	)

	if _, err := p.Config(); err == nil {
		t.Fatal("expected first Config() to fail")
	}
	if _, err := p.Config(); err == nil {
		t.Fatal("expected second Config() to fail")
	}
	cfg, err := p.Config()
	if err != nil || cfg == nil {
		t.Fatalf("third Config() = %v / %v, want success", cfg, err)
	}
	if calls != 3 {
		t.Fatalf("loadConfig calls = %d, want 3", calls)
	}
}

func TestProviderClientCachedAfterSuccess(t *testing.T) {
	clientCalls := 0
	p := NewProviderWithDeps(
		func() (*config.Config, error) {
			return &config.Config{TransactionLimit: 100}, nil
		},
		func(string) (client.ZenClient, error) {
			clientCalls++
			return stubClient{}, nil
		},
		store.New(t.TempDir()+"/sync.json"),
		nil,
	)

	_, _ = p.Client()
	_, _ = p.Client()

	if clientCalls != 1 {
		t.Fatalf("newZenClient calls = %d, want 1", clientCalls)
	}
}
