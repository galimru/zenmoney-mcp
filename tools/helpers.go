package tools

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// runtimeError wraps a dependency initialization failure as a tool error result.
func runtimeError(err error) *mcp.CallToolResult {
	return mcp.NewToolResultError("zenmoney-mcp initialization failed: " + err.Error())
}

// structJSON marshals v as JSON and returns it as a tool result.
func structJSON(v any) (*mcp.CallToolResult, error) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return mcp.NewToolResultText(string(out)), nil
}
