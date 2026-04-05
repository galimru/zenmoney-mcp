package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

// RegisterSyncTools adds the sync and full_sync tools to the MCP server.
func RegisterSyncTools(s *server.MCPServer, runtime *RuntimeProvider) {
	s.AddTool(
		mcp.NewTool("sync",
			mcp.WithDescription("Perform an incremental diff sync with ZenMoney and return counts of entities changed since the last sync."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleSync(ctx, runtime)
		},
	)

	s.AddTool(
		mcp.NewTool("full_sync",
			mcp.WithDescription("Discard cached sync state, perform a full ZenMoney sync, and return counts for the full fetched dataset. Use this to resolve inconsistencies or on first run."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleFullSync(ctx, runtime)
		},
	)
}

func handleSync(ctx context.Context, runtime *RuntimeProvider) (*mcp.CallToolResult, error) {
	resp, _, err := runtime.incrementalSync(ctx)
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
	resp, err := runtime.fullSync(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
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
