package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
	"github.com/galimru/zenmoney-mcp/store"
)

// RegisterSyncTools adds the sync and full_sync tools to the MCP server.
func RegisterSyncTools(s *server.MCPServer, runtime *RuntimeProvider) {
	s.AddTool(
		mcp.NewTool("sync",
			mcp.WithDescription("Perform an incremental sync with ZenMoney, fetching only changes since the last sync. Returns counts of received entities."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleSync(ctx, runtime)
		},
	)

	s.AddTool(
		mcp.NewTool("full_sync",
			mcp.WithDescription("Perform a full sync, discarding cached sync state and re-downloading all ZenMoney data. Use this to resolve inconsistencies or on first run."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleFullSync(ctx, runtime)
		},
	)
}

func handleSync(ctx context.Context, runtime *RuntimeProvider) (*mcp.CallToolResult, error) {
	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	resp, _, err := fetchSyncResponse(ctx, c, runtime.zenStore)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("sync failed: %v", err)), nil
	}

	result, err := structJSON(buildSyncCountResult(resp))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return result, nil
}

func handleFullSync(ctx context.Context, runtime *RuntimeProvider) (*mcp.CallToolResult, error) {
	c, err := runtime.apiClient()
	if err != nil {
		return runtimeError(err), nil
	}

	if err := runtime.zenStore.Reset(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("reset sync state: %v", err)), nil
	}

	resp, err := c.FullSync(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("full sync failed: %v", err)), nil
	}

	if err := runtime.zenStore.Save(&store.SyncState{
		ServerTimestamp: resp.ServerTimestamp,
		LastSyncAt:      time.Now(),
	}); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("save sync state: %v", err)), nil
	}

	result, err := structJSON(buildSyncCountResult(resp))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return result, nil
}

// pushRequest builds a models.Request for pushing entities.
// serverTimestamp should be the last known server timestamp (0 if unknown).
func pushRequest(serverTimestamp int, transactions []models.Transaction) models.Request {
	return models.Request{
		CurrentClientTimestamp: int(time.Now().Unix()),
		ServerTimestamp:        serverTimestamp,
		Transaction:            transactions,
	}
}

// pushTagRequest builds a models.Request for pushing tags.
func pushTagRequest(serverTimestamp int, tags []models.Tag) models.Request {
	return models.Request{
		CurrentClientTimestamp: int(time.Now().Unix()),
		ServerTimestamp:        serverTimestamp,
		Tag:                    tags,
	}
}

// currentServerTimestamp returns the cached server timestamp, or 0 if unavailable.
func currentServerTimestamp(st *store.Store) int {
	if state, ok := st.Get(); ok && state != nil {
		return state.ServerTimestamp
	}
	return 0
}
