package main

import (
	"fmt"
	"os"

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

	runtime := tools.NewRuntimeProvider()
	tools.RegisterAccountTools(s, runtime)
	tools.RegisterTransactionTools(s, runtime)
	tools.RegisterCategoryTools(s, runtime)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
