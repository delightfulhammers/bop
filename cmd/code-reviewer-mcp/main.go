package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bkyoung/code-reviewer/internal/adapter/git"
	"github.com/bkyoung/code-reviewer/internal/adapter/github"
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

	// Get configuration from environment.
	// GITHUB_TOKEN is required for API access to read PR data, comments, and annotations.
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable is required. " +
			"Set it to a GitHub personal access token with 'repo' scope. " +
			"See: https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens")
	}
	// Basic format validation: GitHub tokens are typically 40+ characters
	if len(githubToken) < 40 {
		return fmt.Errorf("GITHUB_TOKEN appears invalid (too short): " +
			"GitHub tokens are typically 40+ characters, check that you've set the full token value")
	}

	// Get repository directory (default to current directory).
	repoDir := os.Getenv("CODE_REVIEWER_REPO_DIR")
	if repoDir == "" {
		var err error
		repoDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
	}

	// Initialize GitHub client (implements PRReader, AnnotationReader, CommentReader).
	githubClient := github.NewClient(githubToken)

	// Initialize git engine (implements FileReader, DiffReader).
	gitEngine := git.NewEngine(repoDir)

	// Initialize suggestion extractor for parsing code suggestions from findings.
	suggestionExtractor := triage.NewSuggestionExtractor()

	// Create ReviewManager adapter (wraps Client to convert types for port interface).
	reviewManager := github.NewReviewManagerAdapter(githubClient)

	// Create PR-based triage service with all dependencies.
	prService := triage.NewPRService(triage.PRServiceDeps{
		// Read operations
		AnnotationReader:    githubClient,
		CommentReader:       githubClient,
		PRReader:            githubClient,
		FileReader:          gitEngine,
		DiffReader:          gitEngine,
		SuggestionExtractor: suggestionExtractor,
		// Write operations
		CommentWriter: githubClient,
		ReviewManager: reviewManager,
	})

	// Create legacy session-based service (for M3 write tools, currently unused).
	triageService := triage.NewService(triage.ServiceDeps{
		ReviewRepo:   nil,
		GitHubClient: nil,
		SessionStore: nil,
	})

	// Create and configure the MCP server.
	server := mcpadapter.NewServer(mcpadapter.ServerDeps{
		PRService:     prService,
		TriageService: triageService,
	})

	// Run the server (blocks until context is cancelled or error occurs).
	return server.Run(ctx)
}
