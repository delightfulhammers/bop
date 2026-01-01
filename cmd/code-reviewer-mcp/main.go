package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bkyoung/code-reviewer/internal/adapter/git"
	"github.com/bkyoung/code-reviewer/internal/adapter/github"
	"github.com/bkyoung/code-reviewer/internal/adapter/llm/anthropic"
	"github.com/bkyoung/code-reviewer/internal/adapter/llm/openai"
	mcpadapter "github.com/bkyoung/code-reviewer/internal/adapter/mcp"
	"github.com/bkyoung/code-reviewer/internal/config"
	"github.com/bkyoung/code-reviewer/internal/usecase/merge"
	"github.com/bkyoung/code-reviewer/internal/usecase/review"
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

	// Load config for reviewer and provider settings.
	// Config is optional - failure just means review_branch won't be available.
	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: defaultConfigPaths(),
		FileName:    "cr",
		EnvPrefix:   "CR",
	})
	if err != nil {
		log.Printf("warning: config load failed, review_branch will be unavailable: %v", err)
		cfg = config.Config{}
	}

	// Build LLM providers from environment variables.
	providers := buildProvidersFromEnv(&cfg)

	// Create branch reviewer if providers are available.
	var branchReviewer mcpadapter.BranchReviewer
	if len(providers) > 0 {
		// Create minimal orchestrator for branch reviews.
		// Note: Merger takes nil store - precision priors will use defaults (same as CLI).
		merger := merge.NewIntelligentMerger(nil)
		reviewerRegistry, err := review.NewReviewerRegistry(&cfg)
		if err != nil {
			log.Printf("warning: reviewer registry creation failed, using defaults: %v", err)
		}

		orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
			Git:              gitEngine,
			Providers:        providers,
			Merger:           merger,
			ReviewerRegistry: reviewerRegistry,
		})
		branchReviewer = orchestrator
	}

	// Create and configure the MCP server.
	server := mcpadapter.NewServer(mcpadapter.ServerDeps{
		PRService:      prService,
		TriageService:  triageService,
		BranchReviewer: branchReviewer,
	})

	// Run the server (blocks until context is cancelled or error occurs).
	return server.Run(ctx)
}

// defaultConfigPaths returns the default config file search paths.
func defaultConfigPaths() []string {
	home, _ := os.UserHomeDir()
	return []string{".", home + "/.config/cr"}
}

// buildProvidersFromEnv creates LLM providers from environment variables.
// Returns an empty map if no API keys are configured.
func buildProvidersFromEnv(cfg *config.Config) map[string]review.Provider {
	providers := make(map[string]review.Provider)

	// Anthropic provider
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		model := "claude-sonnet-4-20250514" // default
		providerCfg := config.ProviderConfig{}
		if cfg != nil {
			if pc, ok := cfg.Providers["anthropic"]; ok {
				providerCfg = pc
				if pc.Model != "" {
					model = pc.Model
				}
			}
		}
		client := anthropic.NewHTTPClient(key, model, providerCfg, cfg.HTTP)
		providers["anthropic"] = anthropic.NewProvider(model, client)
	}

	// OpenAI provider
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		model := "gpt-4o" // default
		providerCfg := config.ProviderConfig{}
		if cfg != nil {
			if pc, ok := cfg.Providers["openai"]; ok {
				providerCfg = pc
				if pc.Model != "" {
					model = pc.Model
				}
			}
		}
		client := openai.NewHTTPClient(key, model, providerCfg, cfg.HTTP)
		providers["openai"] = openai.NewProvider(model, client)
	}

	return providers
}
