package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/delightfulhammers/bop/internal/adapter/analytics"
	"github.com/delightfulhammers/bop/internal/adapter/cli"
	dedupadapter "github.com/delightfulhammers/bop/internal/adapter/dedup"
	"github.com/delightfulhammers/bop/internal/adapter/feedback"
	"github.com/delightfulhammers/bop/internal/adapter/git"
	githubadapter "github.com/delightfulhammers/bop/internal/adapter/github"
	"github.com/delightfulhammers/bop/internal/adapter/llm/anthropic"
	"github.com/delightfulhammers/bop/internal/adapter/llm/gemini"
	llmhttp "github.com/delightfulhammers/bop/internal/adapter/llm/http"
	"github.com/delightfulhammers/bop/internal/adapter/llm/ollama"
	"github.com/delightfulhammers/bop/internal/adapter/llm/openai"
	"github.com/delightfulhammers/bop/internal/adapter/llm/simple"
	"github.com/delightfulhammers/bop/internal/adapter/llm/static"
	"github.com/delightfulhammers/bop/internal/adapter/observability"
	"github.com/delightfulhammers/bop/internal/adapter/output/json"
	"github.com/delightfulhammers/bop/internal/adapter/output/markdown"
	"github.com/delightfulhammers/bop/internal/adapter/output/sarif"
	"github.com/delightfulhammers/bop/internal/adapter/repository"
	storeAdapter "github.com/delightfulhammers/bop/internal/adapter/store"
	"github.com/delightfulhammers/bop/internal/adapter/store/sqlite"
	themeadapter "github.com/delightfulhammers/bop/internal/adapter/theme"
	verifyadapter "github.com/delightfulhammers/bop/internal/adapter/verify"
	"github.com/delightfulhammers/bop/internal/auth"
	"github.com/delightfulhammers/bop/internal/config"
	"github.com/delightfulhammers/bop/internal/determinism"
	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/redaction"
	usecasedeup "github.com/delightfulhammers/bop/internal/usecase/dedup"
	usecasegithub "github.com/delightfulhammers/bop/internal/usecase/github"
	"github.com/delightfulhammers/bop/internal/usecase/merge"
	"github.com/delightfulhammers/bop/internal/usecase/post"
	"github.com/delightfulhammers/bop/internal/usecase/review"
	usecasesession "github.com/delightfulhammers/bop/internal/usecase/session"
	usecaseverify "github.com/delightfulhammers/bop/internal/usecase/verify"
	"github.com/delightfulhammers/bop/internal/version"

	sessionstore "github.com/delightfulhammers/bop/internal/adapter/session"
	platformcontracts "github.com/delightfulhammers/platform/contracts/analytics"
	platformanalytics "github.com/delightfulhammers/platform/pkg/analytics"

	"github.com/google/uuid"
)

func main() {
	if err := run(); err != nil {
		// Redact API keys from URLs in error messages before logging
		log.Println(llmhttp.RedactURLSecrets(err.Error()))
		os.Exit(1)
	}
}

func run() error {
	// Create cancellable context with signal handling for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Parse --log-level flag early, before config loading and observability setup.
	// This uses pflag directly (which Cobra uses internally) to allow CLI override
	// of the log level before the logger is created.
	cliLogLevel := parseLogLevelFlag()

	cfg, err := config.Load(config.LoaderOptions{
		ConfigPaths: defaultConfigPaths(),
		FileName:    "bop",
		EnvPrefix:   "BOP",
	})
	if err != nil {
		return fmt.Errorf("config load failed: %w", err)
	}

	// Apply CLI log level override if provided
	if cliLogLevel != "" {
		cfg.Observability.Logging.Level = cliLogLevel
	}

	repoDir := cfg.Git.RepositoryDir
	if repoDir == "" {
		repoDir = "."
	}

	repoName := repositoryName(repoDir)
	gitEngine := git.NewEngine(repoDir)

	// Timestamp function for deterministic output file naming
	nowFunc := func() string {
		return time.Now().UTC().Format("20060102T150405Z")
	}

	markdownWriter := markdown.NewWriter(nowFunc)
	jsonWriter := json.NewWriter(nowFunc)
	sarifWriter := sarif.NewWriter(nowFunc)

	// Build observability components
	obs := buildObservability(cfg.Observability)

	// Create review logger adapter if logging is enabled
	var reviewLogger review.Logger
	if obs.logger != nil {
		reviewLogger = observability.NewReviewLogger(obs.logger)
	}

	providers := buildProviders(cfg.Providers, cfg.HTTP, obs)

	// Initialize store if enabled
	var reviewStore review.Store
	if cfg.Store.Enabled {
		// Create store directory if it doesn't exist
		storeDir := filepath.Dir(cfg.Store.Path)
		if err := os.MkdirAll(storeDir, 0755); err != nil {
			log.Printf("warning: failed to create store directory: %v", err)
		} else {
			// Initialize SQLite store
			sqliteStore, err := sqlite.NewStore(cfg.Store.Path)
			if err != nil {
				log.Printf("warning: failed to initialize store: %v", err)
			} else {
				// Wrap in adapter bridge
				reviewStore = storeAdapter.NewBridge(sqliteStore)
				// Ensure store is closed on exit
				defer func() {
					if err := reviewStore.Close(); err != nil {
						log.Printf("warning: failed to close review store: %v", err)
					}
				}()
			}
		}
	}

	// Use intelligent merger for better finding aggregation
	// Note: Pass nil for store for now - precision priors will use defaults
	// TODO: Wire up store adapter when precision prior tracking is needed
	merger := merge.NewIntelligentMerger(nil).
		WithReviewerWeighting(cfg.Merge.WeightByReviewer).
		WithRespectFocus(cfg.Merge.RespectFocus)

	// Wire up LLM-based summary synthesis using configured merge provider/model
	synthProvider := createMergeSynthesisProvider(&cfg, obs)
	if synthProvider != nil {
		wrapped := &providerWrapper{provider: synthProvider}
		synthAdapter := merge.NewSynthesisAdapter(wrapped)
		merger.WithSynthesisProvider(synthAdapter)
	}

	// Phase 3.2: Create ReviewerRegistry from config
	reviewerRegistry, err := review.NewReviewerRegistry(&cfg)
	if err != nil {
		log.Fatalf("Failed to create reviewer registry: %v\nPlease configure reviewers in your bop.yaml", err)
	}

	// Use enhanced prompt builder for richer context, wrapped with persona support
	basePromptBuilder := review.NewEnhancedPromptBuilder()
	personaPromptBuilder := review.NewPersonaPromptBuilder(basePromptBuilder)

	// Instantiate redaction engine if enabled
	var redactor review.Redactor
	if cfg.Redaction.Enabled {
		redactor = redaction.NewEngine()
	}

	// Create planning agent if configured and enabled
	//
	// Planning agent workflow:
	// 1. If planning.model is specified, create a dedicated provider instance for that model
	//    (e.g., use gpt-4o-mini for planning while using o3 for reviews)
	// 2. If no planning.model specified, reuse the existing provider from the providers map
	//    (maintains backward compatibility with simpler configurations)
	// 3. If provider creation fails (missing API key, etc.), planning is disabled with a warning
	//    and code review continues without the planning phase
	var planningAgent *review.PlanningAgent
	if cfg.Planning.Enabled && cfg.Planning.Provider != "" {
		planningProvider := createPlanningProvider(&cfg, providers, obs)

		if planningProvider != nil {
			// Parse timeout (default to 30s)
			timeout := 30 * time.Second
			if cfg.Planning.Timeout != "" {
				if parsed, err := time.ParseDuration(cfg.Planning.Timeout); err == nil {
					timeout = parsed
				} else {
					log.Printf("warning: invalid planning timeout %q, using default 30s", cfg.Planning.Timeout)
				}
			}

			// Max questions (default to 5)
			maxQuestions := cfg.Planning.MaxQuestions
			if maxQuestions == 0 {
				maxQuestions = 5
			}

			planningAgent = review.NewPlanningAgent(
				planningProvider,
				review.PlanningConfig{
					MaxQuestions: maxQuestions,
					Timeout:      timeout,
				},
				os.Stdin,
				os.Stdout,
			)
		}
	}

	// Create GitHub poster and triage context fetcher if token is available
	var githubPoster review.GitHubPoster
	var triageContextFetcher review.TriageContextFetcher
	if githubToken := os.Getenv("GITHUB_TOKEN"); githubToken != "" {
		githubClient := githubadapter.NewClient(githubToken)

		// Configure semantic deduplication (Issue #125)
		// Use DefaultConfig as base, then override with user config if provided
		var posterOpts []usecasegithub.ReviewPosterOption
		if semanticComparer := createSemanticComparer(cfg); semanticComparer != nil {
			defaults := usecasedeup.DefaultConfig()
			semanticConfig := usecasegithub.SemanticDedupConfig{
				LineThreshold: defaults.LineThreshold,
				MaxCandidates: defaults.MaxCandidates,
			}
			// Override with user config if explicitly set
			if cfg.Deduplication.Semantic.LineThreshold > 0 {
				semanticConfig.LineThreshold = cfg.Deduplication.Semantic.LineThreshold
			}
			if cfg.Deduplication.Semantic.MaxCandidates > 0 {
				semanticConfig.MaxCandidates = cfg.Deduplication.Semantic.MaxCandidates
			}
			posterOpts = append(posterOpts, usecasegithub.WithSemanticComparer(semanticComparer, semanticConfig))
		}

		// Enable posting out-of-diff findings as issue comments (Issue #259)
		// This makes them visible to MCP triage tools
		posterOpts = append(posterOpts, usecasegithub.WithIssueCommentClient(githubClient))

		reviewPoster := usecasegithub.NewReviewPoster(githubClient, posterOpts...)
		githubPoster = &githubPosterAdapter{poster: reviewPoster}

		// Create triage context fetcher for prior context injection (Issue #138)
		triageContextFetcher = usecasegithub.NewTriageContextFetcher(githubClient, cfg.Review.BotUsername)
	}

	// Create verification agent if enabled and a suitable provider is available
	// Uses the first available LLM provider (preferring anthropic > openai > gemini)
	var verifier review.Verifier
	if cfg.Verification.Enabled {
		verifier = createVerifier(cfg, providers, repoDir, obs)
	}

	// Create theme extractor for reducing thematic repetition (PR #75)
	// Uses any available LLM provider for extracting themes from prior findings
	themeExtractor := createThemeExtractor(cfg)

	// Build per-provider max tokens map from config
	providerMaxTokens := buildProviderMaxTokens(cfg.Providers)

	// Phase 3.5: Create RemoteGitHubClient for remote PR review
	// This shares the same GitHub client used for posting, enabling
	// cr review pr <identifier> to work without a local clone.
	var remoteGitHubClient review.RemoteGitHubClient
	if githubToken := os.Getenv("GITHUB_TOKEN"); githubToken != "" {
		remoteGitHubClient = githubadapter.NewClient(githubToken)
	}

	// Build analytics emitter for usage telemetry (Week 15)
	analyticsEmitter := buildAnalyticsEmitter(cfg.Analytics)

	orchestrator := review.NewOrchestrator(review.OrchestratorDeps{
		Git:                    gitEngine,
		Providers:              providers,
		Merger:                 merger,
		Markdown:               markdownWriter,
		JSON:                   jsonWriter,
		SARIF:                  sarifWriter,
		Redactor:               redactor,
		SeedGenerator:          determinism.GenerateSeed,
		Store:                  reviewStore,
		ReviewerRegistry:       reviewerRegistry,
		PersonaPromptBuilder:   personaPromptBuilder,
		Logger:                 reviewLogger,
		PlanningAgent:          planningAgent,
		RepoDir:                repoDir,
		GitHubPoster:           githubPoster,
		Verifier:               verifier,
		TriageContextFetcher:   triageContextFetcher,
		ThemeExtractor:         themeExtractor,
		ProviderMaxTokens:      providerMaxTokens,
		MaxConcurrentReviewers: cfg.Review.MaxConcurrentReviewers,
		RemoteGitHubClient:     remoteGitHubClient,                 // Phase 3.5: Remote PR review
		Analytics:              analyticsAdapter{analyticsEmitter}, // Week 15: Analytics
	})

	// Phase 3.5b: Create session manager for local session storage
	var sessionManager cli.SessionManager
	sessionStore, err := sessionstore.NewFileStore("")
	if err != nil {
		log.Printf("Warning: Failed to initialize session storage: %v. Session commands will be unavailable.", err)
	} else {
		sessionManager = usecasesession.NewService(sessionStore, gitEngine, repoDir)
	}

	// Create post service for 'cr post' command (requires GitHub token)
	var findingsPoster cli.FindingsPoster
	if remoteGitHubClient != nil && githubPoster != nil {
		findingsPoster = post.NewService(remoteGitHubClient, githubPoster)
	}

	// Initialize auth components for platform authentication (Week 14)
	authDeps := buildAuthDependencies(cfg.Auth)

	// Initialize feedback client for platform feedback (Week 15)
	feedbackClient := buildFeedbackClient(cfg.Auth, authDeps, version.Value())

	root := cli.NewRootCommand(cli.Dependencies{
		BranchReviewer:      orchestrator,
		PRReviewer:          orchestrator, // Phase 3.5: Remote PR review
		FindingsPoster:      findingsPoster,
		SessionManager:      sessionManager,
		AuthDeps:            authDeps,       // Week 14: Platform authentication
		FeedbackClient:      feedbackClient, // Week 15: Feedback
		DefaultOutput:       cfg.Output.Directory,
		DefaultRepo:         repoName,
		DefaultInstructions: cfg.Review.Instructions,
		DefaultReviewActions: cli.DefaultReviewActions{
			OnCritical:            cfg.Review.Actions.OnCritical,
			OnHigh:                cfg.Review.Actions.OnHigh,
			OnMedium:              cfg.Review.Actions.OnMedium,
			OnLow:                 cfg.Review.Actions.OnLow,
			OnClean:               cfg.Review.Actions.OnClean,
			OnNonBlocking:         cfg.Review.Actions.OnNonBlocking,
			AlwaysBlockCategories: cfg.Review.AlwaysBlockCategories,
		},
		DefaultBotUsername:   cfg.Review.BotUsername,
		DefaultPostOutOfDiff: cfg.Review.ShouldPostOutOfDiff(),
		DefaultVerification: cli.DefaultVerification{
			Enabled:            cfg.Verification.Enabled,
			Depth:              cfg.Verification.Depth,
			CostCeiling:        cfg.Verification.CostCeiling,
			ConfidenceDefault:  cfg.Verification.Confidence.Default,
			ConfidenceCritical: cfg.Verification.Confidence.Critical,
			ConfidenceHigh:     cfg.Verification.Confidence.High,
			ConfidenceMedium:   cfg.Verification.Confidence.Medium,
			ConfidenceLow:      cfg.Verification.Confidence.Low,
		},
		Version: version.Value(),
	})

	if err := root.ExecuteContext(ctx); err != nil {
		if errors.Is(err, cli.ErrVersionRequested) {
			return nil
		}
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}

func repositoryName(repoDir string) string {
	abs, err := filepath.Abs(repoDir)
	if err != nil {
		return "unknown"
	}
	return filepath.Base(abs)
}

func defaultConfigPaths() []string {
	// The config loader now handles standard paths internally:
	// 1. ~/.config/bop/ (user config, base)
	// 2. Current directory (project config, overlay)
	// This function is for custom override paths only.
	return nil
}

// parseLogLevelFlag scans os.Args for --log-level flag before Cobra processes commands.
// This is necessary because observability components (including the logger) are created before
// the CLI command runs. By extracting this flag early, we can override the config value.
//
// Returns the log level string if explicitly set, or empty string to use config/env default.
// Valid values: "trace", "debug", "info", "error"
func parseLogLevelFlag() string {
	// Scan args directly for --log-level to avoid partial pflag complexity
	// This is simpler and more explicit than creating a secondary flag set
	for i, arg := range os.Args[1:] {
		// Handle --log-level=value format
		if strings.HasPrefix(arg, "--log-level=") {
			value := strings.TrimPrefix(arg, "--log-level=")
			return validateLogLevel(value)
		}
		// Handle --log-level value format
		if arg == "--log-level" && i+1 < len(os.Args[1:]) {
			value := os.Args[i+2] // +2 because we're iterating from Args[1:]
			return validateLogLevel(value)
		}
	}
	return ""
}

// validateLogLevel checks if the log level is valid and returns it, or empty string with warning.
// Accepts case-insensitive values and trims whitespace.
func validateLogLevel(level string) string {
	// Defense in depth: reject excessively long values
	const maxLogLevelLen = 32
	if len(level) > maxLogLevelLen {
		log.Printf("[WARN] Log level value too long (max %d chars), using config default", maxLogLevelLen)
		return ""
	}

	// Normalize: trim whitespace and lowercase
	level = strings.TrimSpace(strings.ToLower(level))

	validLevels := map[string]bool{
		"trace": true,
		"debug": true,
		"info":  true,
		"error": true,
	}
	if !validLevels[level] {
		log.Printf("[WARN] Invalid --log-level %q, using config default. Valid values: trace, debug, info, error", level)
		return ""
	}
	return level
}

// observabilityComponents holds shared observability instances
type observabilityComponents struct {
	logger  llmhttp.Logger
	metrics llmhttp.Metrics
	pricing llmhttp.Pricing
}

// buildAnalyticsEmitter creates an analytics emitter based on configuration.
// Returns a NopEmitter if analytics is disabled or configuration is incomplete.
// This enables graceful degradation: analytics won't be emitted if not configured.
func buildAnalyticsEmitter(cfg config.AnalyticsConfig) analytics.Emitter {
	// Skip analytics setup if disabled (nil or explicit false)
	if cfg.Enabled == nil || !*cfg.Enabled {
		return analytics.NopEmitter{}
	}

	// Analytics requires a service URL
	if cfg.ServiceURL == "" {
		log.Println("[WARN] Analytics enabled but analytics.serviceUrl not configured - analytics disabled")
		return analytics.NopEmitter{}
	}

	// Create platform HTTP emitter
	var opts []platformanalytics.EmitterOption

	// Use service key if configured for service-to-service auth
	if cfg.ServiceKey != "" {
		opts = append(opts, platformanalytics.WithServiceAuth("bop", cfg.ServiceKey))
	}

	platformEmitter := platformanalytics.NewHTTPEmitter(cfg.ServiceURL, opts...)

	// Wrap in bop-specific emitter with version info
	return analytics.NewEmitter(platformEmitter,
		analytics.WithClientVersion(version.Value()),
	)
}

// buildAuthDependencies creates auth components based on configuration.
// Returns empty AuthDependencies if auth mode is "legacy" or configuration is incomplete.
// This enables graceful degradation: auth commands won't be available in legacy mode.
func buildAuthDependencies(cfg config.AuthConfig) cli.AuthDependencies {
	// Skip auth setup in legacy mode (default, backward compatible)
	if cfg.IsLegacyMode() {
		return cli.AuthDependencies{}
	}

	// Platform mode requires service URL
	if cfg.ServiceURL == "" {
		log.Println("[WARN] Platform auth mode enabled but auth.serviceUrl not configured - auth commands unavailable")
		return cli.AuthDependencies{}
	}

	// Create token store (always needed for auth commands)
	tokenStore, err := auth.NewTokenStore()
	if err != nil {
		log.Printf("[WARN] Failed to initialize token store: %v - auth commands unavailable", err)
		return cli.AuthDependencies{}
	}

	// Create auth client for platform authentication
	productID := cfg.ProductID
	if productID == "" {
		productID = "bop"
	}
	authClient, err := auth.NewClient(auth.ClientConfig{
		BaseURL:   cfg.ServiceURL,
		ProductID: productID,
	})
	if err != nil {
		log.Printf("[WARN] %v - auth commands unavailable", err)
		return cli.AuthDependencies{}
	}

	return cli.AuthDependencies{
		Client:       authClient,
		TokenStore:   tokenStore,
		PlatformMode: true,
	}
}

// buildFeedbackClient creates a feedback client based on auth configuration.
// Returns nil if auth mode is "legacy" or auth dependencies are not fully configured.
// This requires platform auth to be enabled since feedback requires authentication.
func buildFeedbackClient(cfg config.AuthConfig, authDeps cli.AuthDependencies, version string) cli.FeedbackClient {
	// Feedback requires platform auth mode
	if !authDeps.PlatformMode || authDeps.TokenStore == nil {
		return nil
	}

	// Feedback service URL defaults to auth service URL (same platform)
	serviceURL := cfg.ServiceURL
	if serviceURL == "" {
		return nil
	}

	// Create the feedback client
	return feedback.NewClient(
		serviceURL,
		authDeps.TokenStore,
		feedback.WithVersion(version),
	)
}

// buildObservability creates observability components based on configuration
func buildObservability(cfg config.ObservabilityConfig) observabilityComponents {
	var logger llmhttp.Logger
	var metrics llmhttp.Metrics
	var pricing llmhttp.Pricing

	// Create logger if enabled
	if cfg.Logging.Enabled {
		logLevel := llmhttp.LogLevelInfo
		switch cfg.Logging.Level {
		case "trace":
			logLevel = llmhttp.LogLevelTrace
			// Warn about trace level exposing potentially sensitive content
			log.Println("[WARN] Trace logging enabled - prompts and responses will be logged. This may expose sensitive code in logs.")
		case "debug":
			logLevel = llmhttp.LogLevelDebug
		case "error":
			logLevel = llmhttp.LogLevelError
		}

		logFormat := llmhttp.LogFormatHuman
		if cfg.Logging.Format == "json" {
			logFormat = llmhttp.LogFormatJSON
		}

		defaultLogger := llmhttp.NewDefaultLogger(logLevel, logFormat, cfg.Logging.RedactAPIKeys)
		// Always apply config value - 0 means unlimited, positive values set the limit
		defaultLogger = defaultLogger.WithMaxContentBytes(cfg.Logging.MaxContentBytes)
		logger = defaultLogger
	}

	// Create metrics tracker if enabled
	if cfg.Metrics.Enabled {
		metrics = llmhttp.NewDefaultMetrics()
	}

	// Always create pricing calculator (used for cost tracking)
	pricing = llmhttp.NewDefaultPricing()

	return observabilityComponents{
		logger:  logger,
		metrics: metrics,
		pricing: pricing,
	}
}

// createPlanningProvider creates a dedicated provider instance for the planning agent.
// If a specific planning model is configured, it creates a new provider instance for that model.
// Otherwise, it reuses the existing provider from the providers map.
//
// This allows using a cheaper/faster model for planning (e.g., gpt-4o-mini) while using
// more powerful models for the actual code review.
//
// Returns nil if the provider cannot be created (missing config, API key, etc.).
func createPlanningProvider(cfg *config.Config, providers map[string]review.Provider, obs observabilityComponents) review.Provider {
	providerName := cfg.Planning.Provider
	model := cfg.Planning.Model

	// If a specific planning model is configured, create a dedicated provider instance
	if model != "" {
		// Get the provider config for API key and other settings
		providerCfg, ok := cfg.Providers[providerName]
		if !ok {
			log.Printf("warning: planning provider %q not configured in providers section, planning disabled. Add a '%s' provider configuration to enable planning.", providerName, providerName)
			return nil
		}

		// Create provider based on type
		switch providerName {
		case "openai":
			if providerCfg.APIKey == "" {
				log.Printf("warning: planning provider %q missing API key (set OPENAI_API_KEY or providers.openai.apiKey), planning disabled", providerName)
				return nil
			}
			client := openai.NewHTTPClient(providerCfg.APIKey, model, providerCfg, cfg.HTTP)
			llmhttp.WireObservability(client, obs.logger, obs.metrics, obs.pricing)
			return openai.NewProvider(model, client)

		case "anthropic":
			if providerCfg.APIKey == "" {
				log.Printf("warning: planning provider %q missing API key (set ANTHROPIC_API_KEY or providers.anthropic.apiKey), planning disabled", providerName)
				return nil
			}
			client := anthropic.NewHTTPClient(providerCfg.APIKey, model, providerCfg, cfg.HTTP)
			llmhttp.WireObservability(client, obs.logger, obs.metrics, obs.pricing)
			return anthropic.NewProvider(model, client)

		case "gemini":
			if providerCfg.APIKey == "" {
				log.Printf("warning: planning provider %q missing API key (set GEMINI_API_KEY or providers.gemini.apiKey), planning disabled", providerName)
				return nil
			}
			client := gemini.NewHTTPClient(providerCfg.APIKey, model, providerCfg, cfg.HTTP)
			llmhttp.WireObservability(client, obs.logger, obs.metrics, obs.pricing)
			return gemini.NewProvider(model, client)

		case "ollama":
			// Ollama doesn't require API key, uses host instead
			host := os.Getenv("OLLAMA_HOST")
			if host == "" {
				host = "http://localhost:11434"
			}
			client := ollama.NewHTTPClient(host, model, providerCfg, cfg.HTTP)
			llmhttp.WireObservability(client, obs.logger, obs.metrics, obs.pricing)
			return ollama.NewProvider(model, client)

		default:
			log.Printf("warning: unsupported planning provider %q, planning disabled. Supported providers: openai, anthropic, gemini, ollama", providerName)
			return nil
		}
	}

	// Reuse existing provider if no specific model configured
	planningProvider, ok := providers[providerName]
	if !ok {
		log.Printf("warning: planning provider %q not found in enabled providers, planning disabled. Enable the provider in your configuration or set planning.model to use a dedicated model.", providerName)
		return nil
	}
	return planningProvider
}

// createMergeSynthesisProvider creates a provider for merge summary synthesis.
// Uses merge.provider and merge.model from config (defaults: anthropic/claude-haiku-4-5).
func createMergeSynthesisProvider(cfg *config.Config, obs observabilityComponents) review.Provider {
	providerName := cfg.Merge.Provider
	if providerName == "" {
		providerName = "anthropic"
	}
	model := cfg.Merge.Model
	if model == "" {
		model = "claude-haiku-4-5"
	}

	providerCfg, ok := cfg.Providers[providerName]
	if !ok {
		log.Printf("warning: merge synthesis provider %q not configured, using rule-based merge only", providerName)
		return nil
	}

	switch providerName {
	case "openai":
		if providerCfg.APIKey == "" {
			log.Printf("warning: merge provider %q missing API key, using rule-based merge only", providerName)
			return nil
		}
		client := openai.NewHTTPClient(providerCfg.APIKey, model, providerCfg, cfg.HTTP)
		llmhttp.WireObservability(client, obs.logger, obs.metrics, obs.pricing)
		log.Printf("Merge synthesis using %s/%s", providerName, model)
		return openai.NewProvider(model, client)

	case "anthropic":
		if providerCfg.APIKey == "" {
			log.Printf("warning: merge provider %q missing API key, using rule-based merge only", providerName)
			return nil
		}
		client := anthropic.NewHTTPClient(providerCfg.APIKey, model, providerCfg, cfg.HTTP)
		llmhttp.WireObservability(client, obs.logger, obs.metrics, obs.pricing)
		log.Printf("Merge synthesis using %s/%s", providerName, model)
		return anthropic.NewProvider(model, client)

	case "gemini":
		if providerCfg.APIKey == "" {
			log.Printf("warning: merge provider %q missing API key, using rule-based merge only", providerName)
			return nil
		}
		client := gemini.NewHTTPClient(providerCfg.APIKey, model, providerCfg, cfg.HTTP)
		llmhttp.WireObservability(client, obs.logger, obs.metrics, obs.pricing)
		log.Printf("Merge synthesis using %s/%s", providerName, model)
		return gemini.NewProvider(model, client)

	case "ollama":
		host := os.Getenv("OLLAMA_HOST")
		if host == "" {
			host = "http://localhost:11434"
		}
		client := ollama.NewHTTPClient(host, model, providerCfg, cfg.HTTP)
		llmhttp.WireObservability(client, obs.logger, obs.metrics, obs.pricing)
		log.Printf("Merge synthesis using %s/%s", providerName, model)
		return ollama.NewProvider(model, client)

	default:
		log.Printf("warning: unsupported merge provider %q, using rule-based merge only", providerName)
		return nil
	}
}

func buildProviders(providersConfig map[string]config.ProviderConfig, httpConfig config.HTTPConfig, obs observabilityComponents) map[string]review.Provider {
	providers := make(map[string]review.Provider)

	// OpenAI provider
	if cfg, ok := providersConfig["openai"]; ok && isProviderEnabled(cfg) {
		model := cfg.GetDefaultModel()
		if model == "" {
			model = "gpt-5.2"
		}
		// Use real HTTP client if API key is provided
		apiKey := cfg.APIKey
		if apiKey == "" {
			// Fallback to static client if no API key
			log.Println("OpenAI: No API key provided, using static client")
			providers["openai"] = openai.NewProvider(model, openai.NewStaticClient())
		} else {
			client := openai.NewHTTPClient(apiKey, model, cfg, httpConfig)
			llmhttp.WireObservability(client, obs.logger, obs.metrics, obs.pricing)
			providers["openai"] = openai.NewProvider(model, client)
		}
	}

	// Anthropic/Claude provider
	if cfg, ok := providersConfig["anthropic"]; ok && isProviderEnabled(cfg) {
		model := cfg.GetDefaultModel()
		if model == "" {
			model = "claude-sonnet-4-5"
		}
		// Use real HTTP client if API key is provided
		apiKey := cfg.APIKey
		if apiKey == "" {
			log.Println("Anthropic: No API key provided, skipping provider")
		} else {
			client := anthropic.NewHTTPClient(apiKey, model, cfg, httpConfig)
			llmhttp.WireObservability(client, obs.logger, obs.metrics, obs.pricing)
			providers["anthropic"] = anthropic.NewProvider(model, client)
		}
	}

	// Google Gemini provider
	if cfg, ok := providersConfig["gemini"]; ok && isProviderEnabled(cfg) {
		model := cfg.GetDefaultModel()
		if model == "" {
			model = "gemini-3-pro-preview"
		}
		// Use real HTTP client if API key is provided
		apiKey := cfg.APIKey
		if apiKey == "" {
			log.Println("Gemini: No API key provided, skipping provider")
		} else {
			client := gemini.NewHTTPClient(apiKey, model, cfg, httpConfig)
			llmhttp.WireObservability(client, obs.logger, obs.metrics, obs.pricing)
			providers["gemini"] = gemini.NewProvider(model, client)
		}
	}

	// Ollama provider (local LLM)
	if cfg, ok := providersConfig["ollama"]; ok && isProviderEnabled(cfg) {
		model := cfg.GetDefaultModel()
		if model == "" {
			model = "codellama"
		}
		// Use configured host or default to localhost
		host := os.Getenv("OLLAMA_HOST")
		if host == "" {
			host = "http://localhost:11434"
		}
		client := ollama.NewHTTPClient(host, model, cfg, httpConfig)
		llmhttp.WireObservability(client, obs.logger, obs.metrics, obs.pricing)
		providers["ollama"] = ollama.NewProvider(model, client)
	}

	// Static provider (for testing)
	if cfg, ok := providersConfig["static"]; ok && isProviderEnabled(cfg) {
		model := cfg.GetDefaultModel()
		if model == "" {
			model = "static-model"
		}
		providers["static"] = static.NewProvider(model)
	}

	return providers
}

// providerWrapper adapts review.Provider to merge.ReviewProvider.
// This is needed because the types are structurally identical but defined in different packages.
type providerWrapper struct {
	provider review.Provider
}

func (w *providerWrapper) Review(ctx context.Context, req merge.ProviderRequest) (domain.Review, error) {
	// Convert merge.ProviderRequest to review.ProviderRequest
	reviewReq := review.ProviderRequest{
		Prompt:  req.Prompt,
		Seed:    req.Seed,
		MaxSize: req.MaxSize,
	}
	return w.provider.Review(ctx, reviewReq)
}

// Compile-time interface compliance checks
var _ review.GitEngine = (*git.Engine)(nil)
var _ review.Provider = (*openai.Provider)(nil)
var _ review.Provider = (*anthropic.Provider)(nil)
var _ review.Provider = (*gemini.Provider)(nil)
var _ review.Provider = (*ollama.Provider)(nil)
var _ review.Provider = (*static.Provider)(nil)
var _ review.Merger = (*merge.Service)(nil)
var _ review.MarkdownWriter = (*markdown.Writer)(nil)
var _ review.JSONWriter = (*json.Writer)(nil)
var _ review.SARIFWriter = (*sarif.Writer)(nil)
var _ review.Redactor = (*redaction.Engine)(nil)
var _ review.GitHubPoster = (*githubPosterAdapter)(nil)
var _ review.RemoteGitHubClient = (*githubadapter.Client)(nil)

// githubPosterAdapter bridges review.GitHubPoster to the underlying GitHub client.
// It handles diff position calculation and maps between usecase types.
type githubPosterAdapter struct {
	poster *usecasegithub.ReviewPoster
}

// PostReview implements review.GitHubPoster.
func (a *githubPosterAdapter) PostReview(ctx context.Context, req review.GitHubPostRequest) (*review.GitHubPostResult, error) {
	// Map findings to positioned findings with diff positions
	positionedFindings := githubadapter.MapFindings(req.Review.Findings, req.Diff)

	// Build review actions config for determining attention severities
	reviewActions := githubadapter.ReviewActions{
		OnCritical:            req.ActionOnCritical,
		OnHigh:                req.ActionOnHigh,
		OnMedium:              req.ActionOnMedium,
		OnLow:                 req.ActionOnLow,
		OnClean:               req.ActionOnClean,
		OnNonBlocking:         req.ActionOnNonBlocking,
		AlwaysBlockCategories: req.AlwaysBlockCategories,
	}

	// Note: Summary is now built AFTER deduplication in the poster (Issue #125).
	// We pass the Diff so the poster can rebuild the summary with accurate counts.
	// The Review.Summary field is used as fallback if Diff is nil.

	// Build the post request with review action configuration
	// Pass Diff to enable post-deduplication summary generation
	postReq := usecasegithub.PostReviewRequest{
		Owner:                   req.Owner,
		Repo:                    req.Repo,
		PullNumber:              req.PRNumber,
		CommitSHA:               req.CommitSHA,
		Review:                  req.Review, // Use original review; poster will build summary from Diff
		Findings:                positionedFindings,
		Diff:                    &req.Diff, // Pass diff for post-dedup summary generation
		ReviewActions:           reviewActions,
		BotUsername:             req.BotUsername,
		Cost:                    req.Cost,
		PostOutOfDiffAsComments: req.PostOutOfDiffAsComments,
	}

	// Post the review
	result, err := a.poster.PostReview(ctx, postReq)
	if err != nil {
		return nil, err
	}

	return &review.GitHubPostResult{
		ReviewID:                  result.ReviewID,
		CommentsPosted:            result.CommentsPosted,
		CommentsSkipped:           result.CommentsSkipped,
		DuplicatesSkipped:         result.DuplicatesSkipped,
		SemanticDuplicatesSkipped: result.SemanticDuplicatesSkipped,
		HTMLURL:                   result.HTMLURL,
		CurrentCost:               result.CurrentCost,
		PriorCost:                 result.PriorCost,
		CumulativeCost:            result.CumulativeCost,
		OutOfDiffPosted:           result.OutOfDiffPosted,
		OutOfDiffSkipped:          result.OutOfDiffSkipped,
	}, nil
}

// createVerifier creates a batch verifier using the configured LLM provider.
// Uses verification.provider and verification.model from config, with fallback to other providers.
// Returns nil if no suitable provider is available.
func createVerifier(cfg config.Config, providers map[string]review.Provider, repoDir string, obs observabilityComponents) review.Verifier {
	var llmClient verifyadapter.LLMClient
	var providerName string
	var modelName string

	// Get configured provider and model, with defaults
	configuredProvider := cfg.Verification.Provider
	if configuredProvider == "" {
		configuredProvider = "gemini"
	}
	configuredModel := cfg.Verification.Model
	if configuredModel == "" {
		configuredModel = "gemini-3-flash-preview"
	}

	// MaxTokens from config, default to 64000
	maxTokens := cfg.Verification.MaxTokens
	if maxTokens == 0 {
		maxTokens = 64000
	}

	// Try the configured provider first
	providerOrder := []string{configuredProvider}
	// Add fallbacks (excluding the configured provider to avoid duplicates)
	for _, fallback := range []string{"gemini", "anthropic", "openai"} {
		if fallback != configuredProvider {
			providerOrder = append(providerOrder, fallback)
		}
	}

	for _, name := range providerOrder {
		providerCfg, ok := cfg.Providers[name]
		if !isProviderUsable(providerCfg, ok) {
			continue
		}

		// Use configured model only for the configured provider, otherwise use provider's model
		model := providerCfg.Model
		if name == configuredProvider {
			model = configuredModel
		}
		// Ensure we have a model - use defaults if empty
		if model == "" {
			model = defaultVerificationModel(name)
		}

		switch name {
		case "gemini":
			client := gemini.NewHTTPClient(providerCfg.APIKey, model, providerCfg, cfg.HTTP)
			llmhttp.WireObservability(client, obs.logger, obs.metrics, obs.pricing)
			llmClient = &geminiLLMAdapter{client: client, maxTokens: maxTokens}
		case "anthropic":
			client := anthropic.NewHTTPClient(providerCfg.APIKey, model, providerCfg, cfg.HTTP)
			llmhttp.WireObservability(client, obs.logger, obs.metrics, obs.pricing)
			llmClient = &anthropicLLMAdapter{client: client, maxTokens: maxTokens}
		case "openai":
			client := openai.NewHTTPClient(providerCfg.APIKey, model, providerCfg, cfg.HTTP)
			llmhttp.WireObservability(client, obs.logger, obs.metrics, obs.pricing)
			llmClient = &openaiLLMAdapter{client: client, maxTokens: maxTokens}
		}

		if llmClient != nil {
			providerName = name
			modelName = model
			break
		}
	}

	if llmClient == nil {
		log.Println("Verification disabled: no suitable LLM provider available")
		return nil
	}

	log.Printf("Verification enabled using %s/%s (batch mode, maxTokens=%d)", providerName, modelName, maxTokens)

	// Create repository adapter for file access
	repo := repository.NewLocalRepository(repoDir)

	// Create cost tracker with configured ceiling
	costTracker := usecaseverify.NewCostTracker(cfg.Verification.CostCeiling)

	// Build batch config from verification settings
	batchConfig := verifyadapter.BatchConfig{
		Confidence: config.ConfidenceThresholds{
			Default:  cfg.Verification.Confidence.Default,
			Critical: cfg.Verification.Confidence.Critical,
			High:     cfg.Verification.Confidence.High,
			Medium:   cfg.Verification.Confidence.Medium,
			Low:      cfg.Verification.Confidence.Low,
		},
	}

	return verifyadapter.NewBatchVerifier(llmClient, repo, costTracker, batchConfig)
}

// defaultVerificationModel returns a default model for verification when provider's model is empty.
func defaultVerificationModel(provider string) string {
	switch provider {
	case "gemini":
		return "gemini-3-flash-preview"
	case "anthropic":
		return "claude-haiku-4-5"
	case "openai":
		return "gpt-5.2-mini"
	default:
		return ""
	}
}

// createSemanticComparer creates a semantic comparer for deduplication.
// Returns nil if semantic deduplication is disabled or no suitable provider is available.
// Supports Anthropic, OpenAI, and Gemini providers.
func createSemanticComparer(cfg config.Config) usecasedeup.SemanticComparer {
	semanticCfg := cfg.Deduplication.Semantic

	// Check if semantic dedup is enabled (defaults to true if not specified)
	if semanticCfg.Enabled != nil && !*semanticCfg.Enabled {
		return nil
	}

	// Start with defaults, override with user config if provided
	defaults := usecasedeup.DefaultConfig()

	maxTokens := defaults.MaxTokens
	if semanticCfg.MaxTokens > 0 {
		maxTokens = semanticCfg.MaxTokens
	}

	// Try to create a client for the configured provider (or first available)
	simpleClient, actualProvider := createSimpleClient(cfg, semanticCfg.Provider, semanticCfg.Model)
	if simpleClient == nil {
		// Try fallback providers in order of preference
		fallbackProviders := []string{"anthropic", "openai", "gemini"}
		for _, fallback := range fallbackProviders {
			if fallback == semanticCfg.Provider {
				continue // Already tried this one
			}
			simpleClient, actualProvider = createSimpleClient(cfg, fallback, "")
			if simpleClient != nil {
				break
			}
		}
	}

	if simpleClient == nil {
		log.Println("[INFO] Semantic deduplication disabled: no LLM provider API key available")
		return nil
	}

	log.Printf("[INFO] Semantic deduplication enabled (provider=%s)", actualProvider)

	// Adapt simple.Client to dedup.Client and create the comparer
	dedupClient := dedupadapter.NewSimpleClientAdapter(simpleClient)
	return dedupadapter.NewComparer(dedupClient, maxTokens)
}

// createThemeExtractor creates a theme extractor for reducing thematic repetition.
// Returns nil if theme extraction is disabled or no suitable provider is available.
// Supports Anthropic, OpenAI, and Gemini providers.
func createThemeExtractor(cfg config.Config) review.ThemeExtractor {
	themeCfg := cfg.ThemeExtraction

	// Check if theme extraction is enabled (defaults to true if not specified)
	if themeCfg.Enabled != nil && !*themeCfg.Enabled {
		return nil
	}

	// Get defaults from the review package
	defaults := review.DefaultThemeExtractionConfig()

	maxTokens := defaults.MaxTokens
	if themeCfg.MaxTokens > 0 {
		maxTokens = themeCfg.MaxTokens
	}

	minFindings := defaults.MinFindingsForTheme
	if themeCfg.MinFindingsForTheme > 0 {
		minFindings = themeCfg.MinFindingsForTheme
	}

	maxThemes := defaults.MaxThemes
	if themeCfg.MaxThemes > 0 {
		maxThemes = themeCfg.MaxThemes
	}

	// Try to create a client for the configured provider (or first available)
	client, actualProvider := createSimpleClient(cfg, themeCfg.Provider, themeCfg.Model)
	if client == nil {
		// Try fallback providers in order of preference
		fallbackProviders := []string{"anthropic", "openai", "gemini"}
		for _, fallback := range fallbackProviders {
			if fallback == themeCfg.Provider {
				continue // Already tried this one
			}
			client, actualProvider = createSimpleClient(cfg, fallback, "")
			if client != nil {
				break
			}
		}
	}

	if client == nil {
		log.Println("[INFO] Theme extraction disabled: no LLM provider API key available")
		return nil
	}

	// Parse strategy from config (default to comprehensive)
	strategy := review.StrategyComprehensive
	switch themeCfg.Strategy {
	case "abstract":
		strategy = review.StrategyAbstract
	case "specific":
		strategy = review.StrategySpecific
	case "comprehensive", "":
		strategy = review.StrategyComprehensive
	default:
		log.Printf("[WARN] Unknown theme extraction strategy %q, using comprehensive", themeCfg.Strategy)
	}

	log.Printf("[INFO] Theme extraction enabled (provider=%s, strategy=%s)", actualProvider, strategy)

	// Create and return the theme extractor
	extractorConfig := review.ThemeExtractionConfig{
		Strategy:            strategy,
		MaxThemes:           maxThemes,
		MinFindingsForTheme: minFindings,
		MaxTokens:           maxTokens,
	}

	return themeadapter.NewExtractor(client, extractorConfig)
}

// createSimpleClient creates a simple.Client for the specified provider.
// Returns nil if the provider is not available (no API key).
// Returns the client and the actual provider name (including model).
func createSimpleClient(cfg config.Config, provider, model string) (simple.Client, string) {
	// If no provider specified, return nil to try fallbacks
	if provider == "" {
		return nil, ""
	}

	switch provider {
	case "anthropic":
		apiKey := getAPIKey(cfg.Providers, "anthropic", "ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, ""
		}
		if model == "" {
			model = "claude-haiku-4-5" // Default to fast, cheap model for theme extraction
		}
		providerCfg := cfg.Providers["anthropic"]
		client := simple.NewAnthropicClient(apiKey, model, providerCfg, cfg.HTTP)
		return client, fmt.Sprintf("anthropic/%s", model)

	case "openai":
		apiKey := getAPIKey(cfg.Providers, "openai", "OPENAI_API_KEY")
		if apiKey == "" {
			return nil, ""
		}
		if model == "" {
			model = "gpt-4o-mini" // Default to fast, cost-effective model
		}
		providerCfg := cfg.Providers["openai"]
		client := simple.NewOpenAIClient(apiKey, model, providerCfg, cfg.HTTP)
		return client, fmt.Sprintf("openai/%s", model)

	case "gemini":
		apiKey := getAPIKey(cfg.Providers, "gemini", "GEMINI_API_KEY")
		if apiKey == "" {
			return nil, ""
		}
		if model == "" {
			model = "gemini-2.0-flash" // Default to fast model
		}
		providerCfg := cfg.Providers["gemini"]
		client := simple.NewGeminiClient(apiKey, model, providerCfg, cfg.HTTP)
		return client, fmt.Sprintf("gemini/%s", model)

	default:
		log.Printf("warning: theme extraction provider %q is not supported (only anthropic, openai, gemini); trying fallback", provider)
		return nil, ""
	}
}

// getAPIKey retrieves an API key from config or environment variable.
func getAPIKey(providers map[string]config.ProviderConfig, providerName, envVar string) string {
	if providerCfg, ok := providers[providerName]; ok && providerCfg.APIKey != "" {
		return providerCfg.APIKey
	}
	return os.Getenv(envVar)
}

// buildProviderMaxTokens extracts per-provider MaxOutputTokens overrides from config.
// Returns a map of provider name -> max output tokens for providers that have overrides.
func buildProviderMaxTokens(providers map[string]config.ProviderConfig) map[string]int {
	result := make(map[string]int)
	for name, cfg := range providers {
		if cfg.MaxOutputTokens != nil && *cfg.MaxOutputTokens > 0 {
			result[name] = *cfg.MaxOutputTokens
		}
	}
	return result
}

// isProviderEnabled checks if a provider should be used based on its configuration.
// The logic handles three cases for the Enabled field:
//   - nil (not set): provider is enabled if it has an API key (backward compatible)
//   - false: provider is explicitly disabled, regardless of API key
//   - true: provider is explicitly enabled (required for keyless providers like Ollama)
func isProviderEnabled(cfg config.ProviderConfig) bool {
	// If explicitly disabled, respect that regardless of API key
	if cfg.Enabled != nil && !*cfg.Enabled {
		return false
	}
	// If explicitly enabled, use it
	if cfg.Enabled != nil && *cfg.Enabled {
		return true
	}
	// Not set (nil): enable if API key is present (backward compatible)
	return cfg.APIKey != ""
}

// isProviderUsable checks if a provider configuration is usable for verification.
// Wraps isProviderEnabled with an existence check.
func isProviderUsable(cfg config.ProviderConfig, exists bool) bool {
	if !exists {
		return false
	}
	return isProviderEnabled(cfg)
}

// openaiLLMAdapter adapts openai.HTTPClient to verifyadapter.LLMClient.
type openaiLLMAdapter struct {
	client    *openai.HTTPClient
	maxTokens int
}

func (a *openaiLLMAdapter) Call(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
	resp, err := a.client.Call(ctx, userPrompt, openai.CallOptions{
		System:      systemPrompt,
		Temperature: 0.0, // Deterministic for verification
		MaxTokens:   a.maxTokens,
	})
	if err != nil {
		return "", 0, 0, 0, err
	}
	return resp.Text, resp.TokensIn, resp.TokensOut, resp.Cost, nil
}

// anthropicLLMAdapter adapts anthropic.HTTPClient to verifyadapter.LLMClient.
type anthropicLLMAdapter struct {
	client    *anthropic.HTTPClient
	maxTokens int
}

func (a *anthropicLLMAdapter) Call(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
	resp, err := a.client.Call(ctx, userPrompt, anthropic.CallOptions{
		System:      systemPrompt,
		Temperature: 0.0,
		MaxTokens:   a.maxTokens,
	})
	if err != nil {
		return "", 0, 0, 0, err
	}
	return resp.Text, resp.TokensIn, resp.TokensOut, resp.Cost, nil
}

// geminiLLMAdapter adapts gemini.HTTPClient to verifyadapter.LLMClient.
type geminiLLMAdapter struct {
	client    *gemini.HTTPClient
	maxTokens int
}

func (a *geminiLLMAdapter) Call(ctx context.Context, systemPrompt, userPrompt string) (string, int, int, float64, error) {
	// Pass system prompt as Gemini's system instruction for proper handling
	resp, err := a.client.Call(ctx, userPrompt, gemini.CallOptions{
		Temperature:       0.0,
		MaxTokens:         a.maxTokens,
		SystemInstruction: &systemPrompt,
	})
	if err != nil {
		return "", 0, 0, 0, err
	}
	return resp.Text, resp.TokensIn, resp.TokensOut, resp.Cost, nil
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
		clientType = platformcontracts.ClientTypeCLI
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

// Compile-time interface checks for analytics adapter
var _ review.AnalyticsEmitter = analyticsAdapter{}
