package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	mcpadapter "github.com/bkyoung/code-reviewer/internal/adapter/mcp"
	"github.com/bkyoung/code-reviewer/internal/usecase/triage"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Create context that cancels on interrupt signals.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Initialize dependencies.
	// M2: These will be replaced with real implementations.
	triageService := triage.NewService(triage.ServiceDeps{
		ReviewRepo:   nil, // M2: Implement ReviewRepository adapter
		GitHubClient: nil, // M2: Implement GitHubClient adapter
		SessionStore: nil, // M2: Implement SessionStore adapter
	})

	// Create and configure the MCP server.
	server := mcpadapter.NewServer(mcpadapter.ServerDeps{
		TriageService: triageService,
	})

	// Run the server (blocks until context is cancelled or error occurs).
	return server.Run(ctx)
}
