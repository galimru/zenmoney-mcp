package client

import (
	"context"
	"fmt"
	"time"

	"github.com/nemirlev/zenmoney-go-sdk/v2/api"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

// ZenClient is the interface that all tool handlers depend on.
// The production implementation wraps the ZenMoney Go SDK; test code uses a mock.
type ZenClient interface {
	// FullSync fetches all data from ZenMoney, ignoring any cached sync state.
	FullSync(ctx context.Context) (models.Response, error)

	// SyncSince fetches only changes since the given timestamp (incremental sync).
	SyncSince(ctx context.Context, since time.Time) (models.Response, error)

	// Push sends new/updated/deleted entities to ZenMoney and receives the server's
	// response (which may include additional server-side changes). This is used for all
	// write operations: create, update, and delete.
	Push(ctx context.Context, req models.Request) (models.Response, error)

	// Suggest asks ZenMoney to suggest a category (tags, merchant) for a partial transaction.
	Suggest(ctx context.Context, tx models.Transaction) (models.Transaction, error)
}

// Client is the production ZenClient backed by the ZenMoney Go SDK.
type Client struct {
	sdk *api.Client
}

// New creates a ZenClient authenticated with the given token.
// token is typically read from the ZENMONEY_TOKEN environment variable by the caller.
func New(token string) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("ZENMONEY_TOKEN is not set")
	}
	sdkClient, err := api.NewClient(token)
	if err != nil {
		return nil, fmt.Errorf("create zenmoney client: %w", err)
	}
	return &Client{sdk: sdkClient}, nil
}

func (c *Client) FullSync(ctx context.Context) (models.Response, error) {
	return c.sdk.FullSync(ctx)
}

func (c *Client) SyncSince(ctx context.Context, since time.Time) (models.Response, error) {
	return c.sdk.SyncSince(ctx, since)
}

func (c *Client) Push(ctx context.Context, req models.Request) (models.Response, error) {
	return c.sdk.Sync(ctx, req)
}

func (c *Client) Suggest(ctx context.Context, tx models.Transaction) (models.Transaction, error) {
	return c.sdk.Suggest(ctx, tx)
}
