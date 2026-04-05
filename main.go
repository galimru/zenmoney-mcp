package main

import (
	"fmt"
	"os"

	"github.com/galimru/zenmoney-mcp/internal/runtime"
	"github.com/galimru/zenmoney-mcp/tools"
	"github.com/mark3labs/mcp-go/server"
)

// Populated by -ldflags during build; see Makefile.
var version = "dev"

func main() {
	s := server.NewMCPServer(
		"zenmoney-mcp",
		version,
		server.WithToolCapabilities(false),
	)

	p := runtime.NewProvider()
	tools.RegisterAccountTools(s, p)
	tools.RegisterTransactionTools(s, p)
	tools.RegisterImportTools(s, p)
	tools.RegisterCategoryTools(s, p)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
