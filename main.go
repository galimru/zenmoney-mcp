package main

import (
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/server"
	"github.com/galimru/zenmoney-mcp/tools"
)

// Populated by -ldflags during build; see Makefile.
var version = "dev"

func main() {
	s := server.NewMCPServer(
		"zenmoney-mcp",
		version,
		server.WithToolCapabilities(false),
	)

	runtime := tools.NewRuntimeProvider()
	tools.RegisterSyncTools(s, runtime)
	tools.RegisterAccountTools(s, runtime)
	tools.RegisterTransactionTools(s, runtime)
	tools.RegisterTagTools(s, runtime)
	tools.RegisterReadTools(s, runtime)
	tools.RegisterSearchTools(s, runtime)
	tools.RegisterBulkTools(s, runtime)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
