package tools

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// resultText extracts the text from the first content element of a tool result.
func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return tc.Text
}

// mcpReqWithArgs builds a minimal CallToolRequest with the given arguments.
func mcpReqWithArgs(args map[string]any) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	return req
}
