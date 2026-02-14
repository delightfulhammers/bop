package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/delightfulhammers/bop/internal/adapter/git"
	"github.com/delightfulhammers/bop/internal/adapter/github"
	"github.com/delightfulhammers/bop/internal/adapter/llm/provider"
	mcpadapter "github.com/delightfulhammers/bop/internal/adapter/mcp"
	"github.com/delightfulhammers/bop/internal/config"
	"github.com/delightfulhammers/bop/internal/determinism"
	"github.com/delightfulhammers/bop/internal/usecase/merge"
	"github.com/delightfulhammers/bop/internal/usecase/review"
	"github.com/delightfulhammers/bop/internal/usecase/triage"
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
	repoDir := os.Getenv("BOP_REPO_DIR")
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
		CommentWriter:      githubClient,
		IssueCommentWriter: githubClient, // Required for replying to out-of-diff findings
		ReviewManager:      reviewManager,
	})

	// Load embedded config as baseline (provides sensible defaults).
	cfg, err := config.LoadEmbedded()
	if err != nil {
		log.Printf("warning: embedded config load failed: %v", err)
		cfg = config.Config{}
	}

	// Try to load local config and merge over embedded config.
	// Local config takes precedence, allowing user customization.
	localCfg, localErr := config.Load(config.LoaderOptions{
		ConfigPaths: defaultConfigPaths(),
		FileName:    "bop",
		EnvPrefix:   "BOP",
	})
	if localErr != nil {
		// Only warn if it's not a simple "no config files found" case
		if config.HasLocalConfig() {
			log.Printf("warning: local config load failed: %v", localErr)
		}
	} else {
		merged, mergeErr := config.Merge(cfg, localCfg)
		if mergeErr != nil {
			log.Printf("warning: config merge failed: %v", mergeErr)
		} else {
			cfg = merged
		}
	}

	// Create provider factory - builds direct providers from environment variables
	// and supports sampling fallback for zero-config usage.
	providerFactory := provider.NewFactory(provider.FactoryOptions{
		Config: &cfg,
	})

	// Create merger and reviewer registry for reviews.
	merger := merge.NewIntelligentMerger(nil)
	reviewerRegistry, err := review.NewReviewerRegistry(&cfg)
	if err != nil {
		log.Printf("warning: reviewer registry creation failed, using defaults: %v", err)
	}

	// Create persona prompt builder for reviewer personas.
	basePromptBuilder := review.NewEnhancedPromptBuilder()
	personaPromptBuilder := review.NewPersonaPromptBuilder(basePromptBuilder)

	// Create branch/PR reviewer if direct providers are available.
	// If not available, the server will fall back to per-request creation using
	// the factory (which may use sampling if the client supports it).
	var branchReviewer mcpadapter.BranchReviewer
	var prReviewer mcpadapter.PRReviewer
	if providerFactory.HasDirectProviders() {
		orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
			Git:                  gitEngine,
			Providers:            providerFactory.DirectProviders(),
			Merger:               merger,
			ReviewerRegistry:     reviewerRegistry,
			PersonaPromptBuilder: personaPromptBuilder,
			SeedGenerator:        determinism.GenerateSeed,
		})
		branchReviewer = orchestrator
		prReviewer = orchestrator
	}

	// Create and configure the MCP server.
	server := mcpadapter.NewServer(mcpadapter.ServerDeps{
		PRService:            prService,
		BranchReviewer:       branchReviewer,
		PRReviewer:           prReviewer,
		ProviderFactory:      providerFactory,
		Git:                  gitEngine,
		Merger:               merger,
		ReviewerRegistry:     reviewerRegistry,
		PersonaPromptBuilder: personaPromptBuilder,
		SeedGenerator:        determinism.GenerateSeed,
	})

	// Run the server (blocks until context is cancelled or error occurs).
	return server.Run(ctx)
}

// defaultConfigPaths returns the default config file search paths.
func defaultConfigPaths() []string {
	home, _ := os.UserHomeDir()
	return []string{".", home + "/.config/bop"}
}
