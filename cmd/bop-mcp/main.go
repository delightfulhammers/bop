package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/delightfulhammers/bop/internal/adapter/analytics"
	"github.com/delightfulhammers/bop/internal/adapter/feedback"
	"github.com/delightfulhammers/bop/internal/adapter/git"
	"github.com/delightfulhammers/bop/internal/adapter/github"
	"github.com/delightfulhammers/bop/internal/adapter/llm/provider"
	mcpadapter "github.com/delightfulhammers/bop/internal/adapter/mcp"
	"github.com/delightfulhammers/bop/internal/auth"
	"github.com/delightfulhammers/bop/internal/config"
	"github.com/delightfulhammers/bop/internal/determinism"
	"github.com/delightfulhammers/bop/internal/usecase/merge"
	"github.com/delightfulhammers/bop/internal/usecase/review"
	"github.com/delightfulhammers/bop/internal/usecase/triage"
	"github.com/delightfulhammers/bop/internal/version"
	platformcontracts "github.com/delightfulhammers/platform/contracts/analytics"
	platformanalytics "github.com/delightfulhammers/platform/pkg/analytics"

	"github.com/google/uuid"
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

	// Create legacy session-based service (for M3 write tools, currently unused).
	triageService := triage.NewService(triage.ServiceDeps{
		ReviewRepo:   nil,
		GitHubClient: nil,
		SessionStore: nil,
	})

	// Load config for reviewer and provider settings.
	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: defaultConfigPaths(),
		FileName:    "bop",
		EnvPrefix:   "BOP",
	})
	if err != nil {
		log.Printf("warning: config load failed: %v", err)
		cfg = config.Config{}
	}

	// Initialize platform auth if configured (Week 14)
	var authClient *auth.Client
	var tokenStore *auth.TokenStore
	platformMode := cfg.Auth.IsPlatformMode()

	if platformMode {
		if cfg.Auth.ServiceURL == "" {
			log.Printf("warning: platform auth mode requires auth.serviceUrl - falling back to legacy mode")
			platformMode = false
		} else {
			productID := cfg.Auth.ProductID
			if productID == "" {
				productID = "bop"
			}
			var err error
			authClient, err = auth.NewClient(auth.ClientConfig{
				BaseURL:   cfg.Auth.ServiceURL,
				ProductID: productID,
			})
			if err != nil {
				log.Printf("warning: %v - falling back to legacy mode", err)
				platformMode = false
			}

			// Only initialize token store if we're actually in platform mode
			if platformMode {
				tokenStore, err = auth.NewTokenStore()
				if err != nil {
					log.Printf("warning: failed to initialize token store: %v - falling back to legacy mode", err)
					platformMode = false
				}
			}
		}
	}

	// Create feedback client if platform mode is enabled (Week 15)
	var feedbackClient *feedback.Client
	if platformMode && tokenStore != nil && cfg.Auth.ServiceURL != "" {
		feedbackClient = feedback.NewClient(
			cfg.Auth.ServiceURL,
			tokenStore,
			feedback.WithVersion(version.Value()),
		)
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

	// Build analytics emitter for usage telemetry (Week 15)
	analyticsEmitter := buildAnalyticsEmitter(cfg.Analytics)

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
			Analytics:            analyticsEmitter,
		})
		branchReviewer = orchestrator
		prReviewer = orchestrator
	}

	// Create and configure the MCP server.
	server := mcpadapter.NewServer(mcpadapter.ServerDeps{
		PRService:            prService,
		TriageService:        triageService,
		BranchReviewer:       branchReviewer,
		PRReviewer:           prReviewer,
		ProviderFactory:      providerFactory,
		Git:                  gitEngine,
		Merger:               merger,
		ReviewerRegistry:     reviewerRegistry,
		PersonaPromptBuilder: personaPromptBuilder,
		SeedGenerator:        determinism.GenerateSeed,
		// Week 14: Platform authentication
		AuthClient:   authClient,
		TokenStore:   tokenStore,
		PlatformMode: platformMode,
		// Week 15: Analytics and Feedback
		Analytics:      analyticsEmitter,
		FeedbackClient: feedbackClient,
	})

	// Run the server (blocks until context is cancelled or error occurs).
	return server.Run(ctx)
}

// defaultConfigPaths returns the default config file search paths.
func defaultConfigPaths() []string {
	home, _ := os.UserHomeDir()
	return []string{".", home + "/.config/bop"}
}

// buildAnalyticsEmitter creates an analytics emitter based on configuration.
// Returns a NopEmitter if analytics is disabled or configuration is incomplete.
// This enables graceful degradation: analytics won't be emitted if not configured.
func buildAnalyticsEmitter(cfg config.AnalyticsConfig) review.AnalyticsEmitter {
	// Skip analytics setup if disabled (nil or explicit false)
	if cfg.Enabled == nil || !*cfg.Enabled {
		return analyticsAdapter{emitter: analytics.NopEmitter{}}
	}

	// Analytics requires a service URL
	if cfg.ServiceURL == "" {
		log.Println("[WARN] Analytics enabled but analytics.serviceUrl not configured - analytics disabled")
		return analyticsAdapter{emitter: analytics.NopEmitter{}}
	}

	// Create platform HTTP emitter
	var opts []platformanalytics.EmitterOption

	// Use service key if configured for service-to-service auth
	if cfg.ServiceKey != "" {
		opts = append(opts, platformanalytics.WithServiceAuth("bop", cfg.ServiceKey))
	}

	platformEmitter := platformanalytics.NewHTTPEmitter(cfg.ServiceURL, opts...)

	// Wrap in bop-specific emitter with version info
	emitter := analytics.NewEmitter(platformEmitter,
		analytics.WithClientVersion(version.Value()),
	)

	return analyticsAdapter{emitter: emitter}
}

// analyticsAdapter bridges analytics.Emitter to review.AnalyticsEmitter.
// This adapts the adapter package interface to the usecase interface,
// handling string-to-UUID conversion for TenantID and UserID.
type analyticsAdapter struct {
	emitter analytics.Emitter
}

// toAnalyticsEventData converts review.AnalyticsEventData to analytics.ReviewEventData.
// Returns the converted data and whether the conversion was successful.
// If TenantID is invalid, returns empty data (analytics will be skipped).
func toAnalyticsEventData(data review.AnalyticsEventData) (analytics.ReviewEventData, bool) {
	// Parse TenantID (required)
	tenantID, err := uuid.Parse(data.TenantID)
	if err != nil || tenantID == uuid.Nil {
		// Invalid or missing TenantID - skip analytics
		return analytics.ReviewEventData{}, false
	}

	// Parse UserID (optional)
	var userID *uuid.UUID
	if data.UserID != "" {
		parsed, err := uuid.Parse(data.UserID)
		if err != nil {
			log.Printf("[WARN] Invalid UserID format for analytics: %s", data.UserID)
		} else if parsed != uuid.Nil {
			userID = &parsed
		}
	}

	// Map client type string to platform enum
	var clientType platformcontracts.ClientType
	switch data.ClientType {
	case "cli":
		clientType = platformcontracts.ClientTypeCLI
	case "mcp":
		clientType = platformcontracts.ClientTypeMCP
	case "action":
		clientType = platformcontracts.ClientTypeAction
	default:
		clientType = platformcontracts.ClientTypeMCP // Default to MCP for this entrypoint
	}

	return analytics.ReviewEventData{
		TenantID:   tenantID,
		UserID:     userID,
		SessionID:  data.SessionID,
		ClientType: clientType,
		Reviewers:  data.Reviewers,
		Provider:   data.Provider,
		Repository: data.Repository,
	}, true
}

func (a analyticsAdapter) EmitReviewStarted(ctx context.Context, data review.AnalyticsEventData) {
	converted, ok := toAnalyticsEventData(data)
	if !ok {
		return // Skip analytics if TenantID is invalid
	}
	a.emitter.EmitReviewStarted(ctx, converted)
}

func (a analyticsAdapter) EmitReviewCompleted(ctx context.Context, data review.AnalyticsEventData, result review.AnalyticsResult) {
	converted, ok := toAnalyticsEventData(data)
	if !ok {
		return
	}
	a.emitter.EmitReviewCompleted(ctx, converted, analytics.ReviewResult{
		DiffLines:     result.DiffLines,
		FilesReviewed: result.FilesReviewed,
		FindingsCount: result.FindingsCount,
		DurationMs:    result.DurationMs,
		PostedToGH:    result.PostedToGH,
	})
}

func (a analyticsAdapter) EmitReviewFailed(ctx context.Context, data review.AnalyticsEventData, errCode string) {
	converted, ok := toAnalyticsEventData(data)
	if !ok {
		return
	}
	a.emitter.EmitReviewFailed(ctx, converted, errCode)
}

func (a analyticsAdapter) EmitFindingsPosted(ctx context.Context, data review.AnalyticsEventData, findingsCount int) {
	converted, ok := toAnalyticsEventData(data)
	if !ok {
		return
	}
	a.emitter.EmitFindingsPosted(ctx, converted, findingsCount)
}

// Compile-time interface check for analytics adapter
var _ review.AnalyticsEmitter = analyticsAdapter{}
