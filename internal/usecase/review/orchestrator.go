package review

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bkyoung/code-reviewer/internal/domain"
)

// GitEngine abstracts git operations for code review.
type GitEngine interface {
	// GetCumulativeDiff returns the diff between two refs (branches or commits).
	GetCumulativeDiff(ctx context.Context, baseRef, targetRef string, includeUncommitted bool) (domain.Diff, error)

	// GetIncrementalDiff returns the diff between two specific commits.
	// Used for incremental reviews where we only want changes since the last reviewed commit.
	GetIncrementalDiff(ctx context.Context, fromCommit, toCommit string) (domain.Diff, error)

	// CommitExists checks if a commit SHA exists in the repository.
	// Used for force-push detection - if the last reviewed commit no longer exists,
	// we fall back to full diff.
	// Returns (false, nil) if the commit genuinely doesn't exist.
	// Returns (false, error) if there was an error checking (e.g., repo access failure).
	CommitExists(ctx context.Context, commitSHA string) (bool, error)

	// CurrentBranch returns the name of the checked-out branch.
	CurrentBranch(ctx context.Context) (string, error)
}

// Provider defines the outbound port for LLM reviews.
type Provider interface {
	Review(ctx context.Context, req ProviderRequest) (domain.Review, error)

	// EstimateTokens returns an estimated token count for the given text.
	// Used for size budgeting to prevent context overflow.
	EstimateTokens(text string) int
}

// Merger defines the outbound port for merging reviews.
type Merger interface {
	Merge(ctx context.Context, reviews []domain.Review) domain.Review
}

// MarkdownWriter persists provider output to disk.
type MarkdownWriter interface {
	Write(ctx context.Context, artifact domain.MarkdownArtifact) (string, error)
}

// JSONWriter persists provider output to disk.
type JSONWriter interface {
	Write(ctx context.Context, artifact domain.JSONArtifact) (string, error)
}

// SARIFWriter persists provider output to disk in SARIF format.
type SARIFWriter interface {
	Write(ctx context.Context, artifact SARIFArtifact) (string, error)
}

// SARIFArtifact encapsulates the SARIF generation inputs.
type SARIFArtifact struct {
	OutputDir    string
	Repository   string
	BaseRef      string
	TargetRef    string
	Review       domain.Review
	ProviderName string
}

// SeedFunc generates deterministic seeds per review scope.
type SeedFunc func(baseRef, targetRef string) uint64

// PromptBuilder constructs the provider request payload with project context.
type PromptBuilder func(ctx ProjectContext, diff domain.Diff, req BranchRequest, providerName string) (ProviderRequest, error)

// Redactor defines the outbound port for secret redaction.
type Redactor interface {
	Redact(input string) (string, error)
}

// Store defines the outbound port for persisting review history.
type Store interface {
	CreateRun(ctx context.Context, run StoreRun) error
	UpdateRunCost(ctx context.Context, runID string, totalCost float64) error
	SaveReview(ctx context.Context, review StoreReview) error
	SaveFindings(ctx context.Context, findings []StoreFinding) error
	GetPrecisionPriors(ctx context.Context) (map[string]map[string]StorePrecisionPrior, error)
	Close() error
}

// GitHubPoster defines the outbound port for posting reviews to GitHub PRs.
type GitHubPoster interface {
	PostReview(ctx context.Context, req GitHubPostRequest) (*GitHubPostResult, error)
}

// Verifier verifies candidate findings before reporting.
// When enabled, the discovery findings from LLM providers are converted to candidates
// and verified by an agent before being included in the final review.
type Verifier interface {
	// VerifyBatch verifies multiple candidates, potentially in parallel.
	// Returns results in the same order as the input candidates.
	VerifyBatch(ctx context.Context, candidates []domain.CandidateFinding) ([]domain.VerificationResult, error)
}

// TriageContextFetcher retrieves prior triage context from a PR.
// This is used to inject previously-addressed findings into the LLM prompt
// so the reviewer doesn't re-raise concerns that have been acknowledged or disputed.
// (Issue #138 - reduce repeated findings by consuming prior triage context)
type TriageContextFetcher interface {
	// FetchTriagedFindings retrieves findings that have been acknowledged or disputed.
	// Returns nil if there are no triaged findings or if fetching fails.
	// Errors are logged but not returned to avoid blocking the review.
	FetchTriagedFindings(ctx context.Context, owner, repo string, prNumber int) *domain.TriagedFindingContext
}

// GitHubPostRequest contains all data needed to post a review to GitHub.
type GitHubPostRequest struct {
	Owner     string
	Repo      string
	PRNumber  int
	CommitSHA string
	Review    domain.Review
	Diff      domain.Diff // For calculating diff positions

	// ReviewActions configures the GitHub review action for each severity level.
	// Empty values use sensible defaults.
	ActionOnCritical    string
	ActionOnHigh        string
	ActionOnMedium      string
	ActionOnLow         string
	ActionOnClean       string
	ActionOnNonBlocking string

	// AlwaysBlockCategories lists finding categories that always trigger REQUEST_CHANGES.
	AlwaysBlockCategories []string

	// BotUsername is the bot username for auto-dismissing stale reviews.
	// If set, previous reviews from this user are dismissed AFTER the new
	// review posts successfully. This ensures the PR always has review signal.
	BotUsername string
}

// GitHubPostResult contains the result of posting a review.
type GitHubPostResult struct {
	ReviewID        int64
	CommentsPosted  int
	CommentsSkipped int
	HTMLURL         string
}

// StorePrecisionPrior represents precision tracking for a provider/category combination.
type StorePrecisionPrior struct {
	Provider string
	Category string
	Alpha    float64
	Beta     float64
}

// StoreRun represents a review run for persistence.
type StoreRun struct {
	RunID      string
	Timestamp  time.Time
	Scope      string
	ConfigHash string
	TotalCost  float64
	BaseRef    string
	TargetRef  string
	Repository string
}

// StoreReview represents a review record for persistence.
type StoreReview struct {
	ReviewID  string
	RunID     string
	Provider  string
	Model     string
	Summary   string
	CreatedAt time.Time
}

// StoreFinding represents a finding record for persistence.
type StoreFinding struct {
	FindingID   string
	ReviewID    string
	FindingHash string
	File        string
	LineStart   int
	LineEnd     int
	Category    string
	Severity    string
	Description string
	Suggestion  string
	Evidence    bool
}

// OrchestratorDeps captures the inbound dependencies for the orchestrator.
type OrchestratorDeps struct {
	Git           GitEngine
	Providers     map[string]Provider
	Merger        Merger
	Markdown      MarkdownWriter
	JSON          JSONWriter
	SARIF         SARIFWriter
	Redactor      Redactor
	SeedGenerator SeedFunc
	PromptBuilder PromptBuilder
	Store         Store          // Optional: persistence layer for review history
	Logger        Logger         // Optional: structured logging for warnings and info
	PlanningAgent *PlanningAgent // Optional: interactive planning agent (only works in TTY mode)
	RepoDir       string         // Repository directory for context gathering (optional)
	GitHubPoster  GitHubPoster   // Optional: posts review to GitHub PR with inline comments
	DiffComputer  *DiffComputer  // Optional: computes diffs (auto-created if nil)

	// Verification support (Epic #92)
	Verifier Verifier // Optional: verifies candidate findings before reporting

	// Prior triage context support (Issue #138)
	TriageContextFetcher TriageContextFetcher // Optional: fetches prior triage context from PR

	// ProviderMaxTokens allows per-provider max output token overrides.
	// Key is provider name, value is max output tokens.
	// If not set for a provider, the default from PromptBuilder is used.
	ProviderMaxTokens map[string]int
}

// ProviderRequest describes the payload the LLM provider expects.
type ProviderRequest struct {
	Prompt  string
	Seed    uint64
	MaxSize int
}

// BranchRequest represents an inbound CLI request.
type BranchRequest struct {
	BaseRef            string
	TargetRef          string
	OutputDir          string
	Repository         string
	IncludeUncommitted bool
	CustomInstructions string   // Optional: custom review instructions
	ContextFiles       []string // Optional: additional context files to include
	NoArchitecture     bool     // Skip loading ARCHITECTURE.md
	NoAutoContext      bool     // Disable automatic context gathering (design docs, relevant docs)
	Interactive        bool     // Enable interactive planning mode (requires TTY)

	// GitHub integration fields (for posting inline review comments)
	PostToGitHub bool   // Enable posting review to GitHub PR
	GitHubOwner  string // Repository owner (user or org)
	GitHubRepo   string // Repository name
	PRNumber     int    // Pull request number
	CommitSHA    string // Head commit SHA for the review

	// Review action configuration (configures GitHub review action per severity)
	// Values: "approve", "comment", "request_changes" (case-insensitive)
	ActionOnCritical    string // Action for critical severity findings
	ActionOnHigh        string // Action for high severity findings
	ActionOnMedium      string // Action for medium severity findings
	ActionOnLow         string // Action for low severity findings
	ActionOnClean       string // Action when no findings in diff
	ActionOnNonBlocking string // Action when findings exist but none block

	// AlwaysBlockCategories lists finding categories that always trigger REQUEST_CHANGES
	// regardless of severity. This provides an additive override for specific categories
	// like "security" that should always block, even if severity-based config wouldn't.
	AlwaysBlockCategories []string

	// BotUsername is the bot username for auto-dismissing stale reviews.
	// If set, previous reviews from this user are dismissed AFTER the new
	// review posts successfully. This ensures the PR always has review signal.
	// Set to empty string to disable auto-dismiss (use "none" in config).
	// Default: "github-actions[bot]"
	BotUsername string

	// SkipVerification disables agent-based verification of findings.
	// When true, findings from LLM providers are reported directly without verification.
	// Use --no-verify flag to enable this from the CLI.
	SkipVerification bool

	// VerificationConfig holds verification-specific settings.
	// These are populated from the config file and can be overridden by CLI flags.
	VerificationConfig VerificationSettings
}

// VerificationSettings holds configuration for the verification stage.
type VerificationSettings struct {
	// Depth controls verification thoroughness: "minimal", "medium", or "thorough".
	Depth string

	// CostCeiling is the maximum USD to spend on verification per review.
	CostCeiling float64

	// ConfidenceCritical is the minimum confidence for critical findings.
	ConfidenceCritical int

	// ConfidenceHigh is the minimum confidence for high severity findings.
	ConfidenceHigh int

	// ConfidenceMedium is the minimum confidence for medium severity findings.
	ConfidenceMedium int

	// ConfidenceLow is the minimum confidence for low severity findings.
	ConfidenceLow int

	// ConfidenceDefault is the fallback minimum confidence when specific thresholds are not set.
	ConfidenceDefault int
}

// Result captures the orchestrator outcome.
type Result struct {
	MarkdownPaths map[string]string
	JSONPaths     map[string]string
	SARIFPaths    map[string]string
	Reviews       []domain.Review
	GitHubResult  *GitHubPostResult // Set when PostToGitHub is enabled
}

// Orchestrator implements the core review flow for Phase 1.
type Orchestrator struct {
	deps OrchestratorDeps
}

// NewOrchestrator wires the orchestrator dependencies.
// If DiffComputer is not provided but Git is, it will be auto-created.
func NewOrchestrator(deps OrchestratorDeps) *Orchestrator {
	// Auto-wire DiffComputer if not provided
	if deps.DiffComputer == nil && deps.Git != nil {
		deps.DiffComputer = NewDiffComputer(deps.Git)
	}
	return &Orchestrator{deps: deps}
}

// validateDependencies checks that all required dependencies are present.
func (o *Orchestrator) validateDependencies() error {
	if o.deps.Git == nil {
		return errors.New("git engine is required")
	}
	if len(o.deps.Providers) == 0 {
		return errors.New("at least one provider is required")
	}
	if o.deps.Merger == nil {
		return errors.New("merger is required")
	}
	if o.deps.Markdown == nil {
		return errors.New("markdown writer is required")
	}
	if o.deps.JSON == nil {
		return errors.New("json writer is required")
	}
	if o.deps.SARIF == nil {
		return errors.New("sarif writer is required")
	}
	if o.deps.PromptBuilder == nil {
		return errors.New("prompt builder is required")
	}
	if o.deps.SeedGenerator == nil {
		return errors.New("seed generator is required")
	}
	if o.deps.DiffComputer == nil {
		return errors.New("diff computer is required (use NewOrchestrator for auto-wiring)")
	}
	// Redactor is optional
	// Store is optional
	return nil
}

// ReviewBranch executes a multi-provider review for a Git branch diff.
func (o *Orchestrator) ReviewBranch(ctx context.Context, req BranchRequest) (Result, error) {
	if err := o.validateDependencies(); err != nil {
		return Result{}, err
	}

	if err := validateRequest(req); err != nil {
		return Result{}, err
	}

	// Compute full diff (DiffComputer is auto-wired in NewOrchestrator when Git is provided)
	diff, err := o.deps.DiffComputer.ComputeDiffForReview(ctx, req)
	if err != nil {
		return Result{}, err
	}

	// Gather project context if RepoDir is configured
	projectContext := ProjectContext{}
	if o.deps.RepoDir != "" {
		gatherer := NewContextGatherer(o.deps.RepoDir)

		// Load architecture documentation (unless disabled)
		if !req.NoArchitecture {
			if architecture, err := gatherer.loadFile("ARCHITECTURE.md"); err == nil {
				projectContext.Architecture = architecture
			}
		}

		// Load README and design docs (unless auto-context is disabled)
		if !req.NoAutoContext {
			// Load README
			if readme, err := gatherer.loadFile("README.md"); err == nil {
				projectContext.README = readme
			}

			// Load design documents
			if designDocs, err := gatherer.loadDesignDocs(); err == nil {
				projectContext.DesignDocs = designDocs
			}

			// Detect change types and find relevant docs
			projectContext.ChangeTypes = gatherer.detectChangeTypes(diff)
			projectContext.ChangedPaths = make([]string, 0, len(diff.Files))
			for _, file := range diff.Files {
				projectContext.ChangedPaths = append(projectContext.ChangedPaths, file.Path)
			}

			if relevantDocs, err := gatherer.findRelevantDocs(projectContext.ChangedPaths, projectContext.ChangeTypes); err == nil {
				projectContext.RelevantDocs = relevantDocs
			}
		}

		// Load custom context files from request (always, regardless of flags)
		if len(req.ContextFiles) > 0 {
			contextFiles := make([]string, 0, len(req.ContextFiles))
			for _, file := range req.ContextFiles {
				content, err := gatherer.loadFile(file)
				if err != nil {
					return Result{}, fmt.Errorf("failed to load context file %s: %w", file, err)
				}
				contextFiles = append(contextFiles, fmt.Sprintf("=== %s ===\n%s", file, content))
			}
			projectContext.CustomContextFiles = contextFiles
		}
	}

	// Always set custom instructions from request (even if RepoDir is not configured)
	projectContext.CustomInstructions = req.CustomInstructions

	// Fetch prior triage context if posting to GitHub and fetcher is configured (Issue #138)
	// This injects previously-addressed findings into the prompt so LLMs don't re-raise them.
	//
	// Note: There's a theoretical race condition where GitHub state could change between
	// this fetch and when PostReview runs. This is acceptable because:
	// 1. The time window is very short (seconds during LLM review)
	// 2. The worst case is slightly stale triage context, not data corruption
	// 3. OUTPUT-SIDE deduplication in poster.go provides the safety net
	if req.PostToGitHub && o.deps.TriageContextFetcher != nil {
		triageCtx := o.deps.TriageContextFetcher.FetchTriagedFindings(
			ctx, req.GitHubOwner, req.GitHubRepo, req.PRNumber,
		)
		if triageCtx != nil && triageCtx.HasFindings() {
			projectContext.TriagedFindings = triageCtx
			if o.deps.Logger != nil {
				o.deps.Logger.LogInfo(ctx, "loaded prior triage context", map[string]interface{}{
					"prNumber":     req.PRNumber,
					"acknowledged": len(triageCtx.AcknowledgedFindings()),
					"disputed":     len(triageCtx.DisputedFindings()),
				})
			} else {
				log.Printf("Loaded prior triage context: %d acknowledged, %d disputed findings\n",
					len(triageCtx.AcknowledgedFindings()), len(triageCtx.DisputedFindings()))
			}
		}
	}

	// Planning Phase: Interactive clarifying questions (optional, only in TTY mode)
	if req.Interactive && IsInteractive() && o.deps.PlanningAgent != nil {
		planningResult, err := o.deps.PlanningAgent.Plan(ctx, projectContext, diff)
		if err != nil {
			// Planning failure shouldn't block the review - log warning and continue
			if o.deps.Logger != nil {
				o.deps.Logger.LogWarning(ctx, "planning phase failed", map[string]interface{}{
					"error": err.Error(),
				})
			} else {
				log.Printf("warning: planning phase failed: %v\n", err)
			}
		} else {
			// Use enhanced context from planning
			projectContext = planningResult.EnhancedContext
		}
	}

	// Generate run ID for potential store usage
	now := time.Now()
	var runID string
	if o.deps.Store != nil {
		runID = generateRunID(now, req.BaseRef, req.TargetRef)
	}

	seed := o.deps.SeedGenerator(req.BaseRef, req.TargetRef)

	// Create run record BEFORE launching provider goroutines so that reviews can reference it
	if o.deps.Store != nil && runID != "" {
		run := StoreRun{
			RunID:      runID,
			Timestamp:  now,
			Scope:      fmt.Sprintf("%s..%s", req.BaseRef, req.TargetRef),
			ConfigHash: calculateConfigHash(req),
			TotalCost:  0.0, // Will be updated after all reviews complete
			BaseRef:    req.BaseRef,
			TargetRef:  req.TargetRef,
			Repository: req.Repository,
		}

		if err := o.deps.Store.CreateRun(ctx, run); err != nil {
			// Log warning but continue - store failures shouldn't break reviews
			if o.deps.Logger != nil {
				o.deps.Logger.LogWarning(ctx, "failed to create run record", map[string]interface{}{
					"runID": runID,
					"error": err.Error(),
				})
			} else {
				log.Printf("warning: failed to create run record: %v\n", err)
			}
		}
	}

	var wg sync.WaitGroup
	resultsChan := make(chan struct {
		review    domain.Review
		path      string
		jsonPath  string
		sarifPath string
		err       error
	}, len(o.deps.Providers))

	for name, provider := range o.deps.Providers {
		wg.Add(1)
		go func(name string, provider Provider, runID string) {
			defer func() {
				if r := recover(); r != nil {
					resultsChan <- struct {
						review    domain.Review
						path      string
						jsonPath  string
						sarifPath string
						err       error
					}{err: fmt.Errorf("provider %s panicked: %v", name, r)}
				}
				wg.Done()
			}()

			// Filter binary files before building prompt (saves tokens, prevents impossible findings)
			textDiff, binaryFiles := FilterBinaryFiles(diff)
			if len(binaryFiles) > 0 {
				log.Printf("[%s] Filtered %d binary file(s) from review", name, len(binaryFiles))
			}

			// Build provider-specific prompt using filtered diff
			providerReq, err := o.deps.PromptBuilder(projectContext, textDiff, req, name)
			if err != nil {
				resultsChan <- struct {
					review    domain.Review
					path      string
					jsonPath  string
					sarifPath string
					err       error
				}{err: fmt.Errorf("prompt building failed for %s: %w", name, err)}
				return
			}
			if providerReq.Seed == 0 {
				providerReq.Seed = seed
			}

			// Apply per-provider MaxSize override if configured
			if maxTokens, ok := o.deps.ProviderMaxTokens[name]; ok && maxTokens > 0 {
				providerReq.MaxSize = maxTokens
			}

			// Apply redaction if redactor is available
			if o.deps.Redactor != nil {
				redactedPrompt, err := o.deps.Redactor.Redact(providerReq.Prompt)
				if err != nil {
					resultsChan <- struct {
						review    domain.Review
						path      string
						jsonPath  string
						sarifPath string
						err       error
					}{err: fmt.Errorf("redaction failed for %s: %w", name, err)}
					return
				}
				providerReq.Prompt = redactedPrompt
			}

			review, err := provider.Review(ctx, providerReq)
			if err != nil {
				resultsChan <- struct {
					review    domain.Review
					path      string
					jsonPath  string
					sarifPath string
					err       error
				}{err: fmt.Errorf("provider %s failed: %w", name, err)}
				return
			}

			markdownPath, err := o.deps.Markdown.Write(ctx, domain.MarkdownArtifact{
				OutputDir:    req.OutputDir,
				Repository:   req.Repository,
				BaseRef:      req.BaseRef,
				TargetRef:    req.TargetRef,
				Diff:         diff,
				Review:       review,
				ProviderName: review.ProviderName,
			})
			if err != nil {
				resultsChan <- struct {
					review    domain.Review
					path      string
					jsonPath  string
					sarifPath string
					err       error
				}{err: fmt.Errorf("markdown write failed for %s: %w", name, err)}
				return
			}

			jsonPath, err := o.deps.JSON.Write(ctx, domain.JSONArtifact{
				OutputDir:    req.OutputDir,
				Repository:   req.Repository,
				BaseRef:      req.BaseRef,
				TargetRef:    req.TargetRef,
				Review:       review,
				ProviderName: review.ProviderName,
			})
			if err != nil {
				resultsChan <- struct {
					review    domain.Review
					path      string
					jsonPath  string
					sarifPath string
					err       error
				}{err: fmt.Errorf("json write failed for %s: %w", name, err)}
				return
			}

			sarifPath, err := o.deps.SARIF.Write(ctx, SARIFArtifact{
				OutputDir:    req.OutputDir,
				Repository:   req.Repository,
				BaseRef:      req.BaseRef,
				TargetRef:    req.TargetRef,
				Review:       review,
				ProviderName: review.ProviderName,
			})
			if err != nil {
				resultsChan <- struct {
					review    domain.Review
					path      string
					jsonPath  string
					sarifPath string
					err       error
				}{err: fmt.Errorf("sarif write failed for %s: %w", name, err)}
				return
			}

			// Save review to store if available
			if runID != "" {
				if err := o.SaveReviewToStore(ctx, runID, review); err != nil {
					// Log warning but continue
					if o.deps.Logger != nil {
						o.deps.Logger.LogWarning(ctx, "failed to save review to store", map[string]interface{}{
							"runID":    runID,
							"provider": name,
							"error":    err.Error(),
						})
					} else {
						log.Printf("warning: failed to save review to store: %v\n", err)
					}
				}
			}

			resultsChan <- struct {
				review    domain.Review
				path      string
				jsonPath  string
				sarifPath string
				err       error
			}{review: review, path: markdownPath, jsonPath: jsonPath, sarifPath: sarifPath}
		}(name, provider, runID)
	}

	wg.Wait()
	close(resultsChan)

	var reviews []domain.Review
	markdownPaths := make(map[string]string)
	jsonPaths := make(map[string]string)
	sarifPaths := make(map[string]string)
	var errs []error
	var totalCost float64

	for res := range resultsChan {
		if res.err != nil {
			errs = append(errs, res.err)
		} else {
			reviews = append(reviews, res.review)
			markdownPaths[res.review.ProviderName] = res.path
			jsonPaths[res.review.ProviderName] = res.jsonPath
			sarifPaths[res.review.ProviderName] = res.sarifPath
			totalCost += res.review.Cost
		}
	}

	if len(errs) > 0 {
		// Aggregate all errors into a single error message
		var errMsgs []string
		for _, err := range errs {
			errMsgs = append(errMsgs, err.Error())
		}
		return Result{}, fmt.Errorf("%d provider(s) failed: %s", len(errs), strings.Join(errMsgs, "; "))
	}

	// Update run record with total cost now that all reviews are complete
	if o.deps.Store != nil && runID != "" {
		if err := o.deps.Store.UpdateRunCost(ctx, runID, totalCost); err != nil {
			// Log warning but continue - store failures shouldn't break reviews
			if o.deps.Logger != nil {
				o.deps.Logger.LogWarning(ctx, "failed to update run cost", map[string]interface{}{
					"runID":     runID,
					"totalCost": totalCost,
					"error":     err.Error(),
				})
			} else {
				log.Printf("warning: failed to update run cost: %v\n", err)
			}
		}
	}

	mergedReview := o.deps.Merger.Merge(ctx, reviews)
	mergedReview.Cost = totalCost // Merged review gets total cost from all providers

	// Verification stage: verify merged findings if enabled
	if o.deps.Verifier != nil && !req.SkipVerification && len(mergedReview.Findings) > 0 {
		candidates, verified, reportable, verifyErr := o.verifyFindings(
			ctx,
			mergedReview.Findings,
			mergedReview.ProviderName,
			req.VerificationConfig,
		)

		if verifyErr != nil {
			// Log warning but continue with unverified findings
			if o.deps.Logger != nil {
				o.deps.Logger.LogWarning(ctx, "verification failed, using unverified findings", map[string]interface{}{
					"error":    verifyErr.Error(),
					"findings": len(mergedReview.Findings),
				})
			} else {
				log.Printf("warning: verification failed, using unverified findings: %v\n", verifyErr)
			}
		} else {
			// Store verification results in the review
			mergedReview.DiscoveryFindings = candidates
			mergedReview.VerifiedFindings = verified
			mergedReview.ReportableFindings = reportable

			// Replace Findings with only the reportable ones for backward compatibility
			// This ensures GitHub poster and other consumers use filtered findings
			mergedReview.Findings = convertVerifiedToFindings(reportable)

			// Log detailed verification results for each finding
			logVerificationDetails(ctx, verified, reportable, req.VerificationConfig, o.deps.Logger)

			if o.deps.Logger != nil {
				o.deps.Logger.LogInfo(ctx, "verification complete", map[string]interface{}{
					"candidates": len(candidates),
					"verified":   len(verified),
					"reportable": len(reportable),
				})
			}
		}
	}

	mergedMarkdownPath, err := o.deps.Markdown.Write(ctx, domain.MarkdownArtifact{
		OutputDir:    req.OutputDir,
		Repository:   req.Repository,
		BaseRef:      req.BaseRef,
		TargetRef:    req.TargetRef,
		Diff:         diff,
		Review:       mergedReview,
		ProviderName: mergedReview.ProviderName,
	})
	if err != nil {
		return Result{}, fmt.Errorf("markdown write failed for merged review: %w", err)
	}

	mergedJSONPath, err := o.deps.JSON.Write(ctx, domain.JSONArtifact{
		OutputDir:    req.OutputDir,
		Repository:   req.Repository,
		BaseRef:      req.BaseRef,
		TargetRef:    req.TargetRef,
		Review:       mergedReview,
		ProviderName: mergedReview.ProviderName,
	})
	if err != nil {
		return Result{}, fmt.Errorf("json write failed for merged review: %w", err)
	}

	mergedSARIFPath, err := o.deps.SARIF.Write(ctx, SARIFArtifact{
		OutputDir:    req.OutputDir,
		Repository:   req.Repository,
		BaseRef:      req.BaseRef,
		TargetRef:    req.TargetRef,
		Review:       mergedReview,
		ProviderName: mergedReview.ProviderName,
	})
	if err != nil {
		return Result{}, fmt.Errorf("sarif write failed for merged review: %w", err)
	}

	// Save merged review to store if available
	if runID != "" {
		if err := o.SaveReviewToStore(ctx, runID, mergedReview); err != nil {
			// Log warning but continue
			if o.deps.Logger != nil {
				o.deps.Logger.LogWarning(ctx, "failed to save merged review to store", map[string]interface{}{
					"runID":    runID,
					"provider": "merged",
					"error":    err.Error(),
				})
			} else {
				log.Printf("warning: failed to save merged review to store: %v\n", err)
			}
		}
	}

	markdownPaths["merged"] = mergedMarkdownPath
	jsonPaths["merged"] = mergedJSONPath
	sarifPaths["merged"] = mergedSARIFPath

	// Post review to GitHub if enabled
	var githubResult *GitHubPostResult
	if req.PostToGitHub && o.deps.GitHubPoster != nil {
		result, err := o.deps.GitHubPoster.PostReview(ctx, GitHubPostRequest{
			Owner:                 req.GitHubOwner,
			Repo:                  req.GitHubRepo,
			PRNumber:              req.PRNumber,
			CommitSHA:             req.CommitSHA,
			Review:                mergedReview,
			Diff:                  diff,
			ActionOnCritical:      req.ActionOnCritical,
			ActionOnHigh:          req.ActionOnHigh,
			ActionOnMedium:        req.ActionOnMedium,
			ActionOnLow:           req.ActionOnLow,
			ActionOnClean:         req.ActionOnClean,
			ActionOnNonBlocking:   req.ActionOnNonBlocking,
			AlwaysBlockCategories: req.AlwaysBlockCategories,
			BotUsername:           req.BotUsername,
		})
		if err != nil {
			// Log warning but don't fail the review
			if o.deps.Logger != nil {
				o.deps.Logger.LogWarning(ctx, "failed to post review to GitHub", map[string]interface{}{
					"owner":    req.GitHubOwner,
					"repo":     req.GitHubRepo,
					"prNumber": req.PRNumber,
					"error":    err.Error(),
				})
			} else {
				log.Printf("warning: failed to post review to GitHub: %v\n", err)
			}
		} else {
			githubResult = result
			if o.deps.Logger != nil {
				o.deps.Logger.LogInfo(ctx, "posted review to GitHub", map[string]interface{}{
					"reviewID":        result.ReviewID,
					"commentsPosted":  result.CommentsPosted,
					"commentsSkipped": result.CommentsSkipped,
					"url":             result.HTMLURL,
				})
			} else {
				log.Printf("Posted review to GitHub: %d comments (%d skipped) - %s\n",
					result.CommentsPosted, result.CommentsSkipped, result.HTMLURL)
			}
		}
	}

	return Result{
		MarkdownPaths: markdownPaths,
		JSONPaths:     jsonPaths,
		SARIFPaths:    sarifPaths,
		Reviews:       append(reviews, mergedReview),
		GitHubResult:  githubResult,
	}, nil
}

// CurrentBranch returns the checked-out branch name.
func (o *Orchestrator) CurrentBranch(ctx context.Context) (string, error) {
	if o.deps.Git == nil {
		return "", errors.New("orchestrator dependencies missing")
	}
	return o.deps.Git.CurrentBranch(ctx)
}

func validateRequest(req BranchRequest) error {
	if strings.TrimSpace(req.BaseRef) == "" {
		return errors.New("base ref is required")
	}
	if strings.TrimSpace(req.TargetRef) == "" {
		return errors.New("target ref is required")
	}
	if strings.TrimSpace(req.OutputDir) == "" {
		return errors.New("output directory is required")
	}
	return nil
}

// FilterBinaryFiles separates a diff into text files and binary files.
// The text diff is suitable for sending to LLMs (excludes binary files to save tokens).
// The full diff (with binary files) should be used for GitHub posting so binary
// file changes are visible in the summary.
func FilterBinaryFiles(diff domain.Diff) (textDiff domain.Diff, binaryFiles []domain.FileDiff) {
	textFiles := make([]domain.FileDiff, 0, len(diff.Files))
	binaryFiles = make([]domain.FileDiff, 0)

	for _, f := range diff.Files {
		if f.IsBinary {
			binaryFiles = append(binaryFiles, f)
		} else {
			textFiles = append(textFiles, f)
		}
	}

	textDiff = domain.Diff{
		FromCommitHash: diff.FromCommitHash,
		ToCommitHash:   diff.ToCommitHash,
		Files:          textFiles,
	}

	return textDiff, binaryFiles
}
