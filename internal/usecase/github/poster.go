// Package github provides use cases for interacting with GitHub.
package github

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/delightfulhammers/bop/internal/adapter/github"
	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/dedup"
)

// ReviewClient defines the interface for interacting with GitHub reviews.
// This interface allows for mocking in tests.
type ReviewClient interface {
	CreateReview(ctx context.Context, input github.CreateReviewInput) (*github.CreateReviewResponse, error)
	ListReviews(ctx context.Context, owner, repo string, pullNumber int) ([]github.ReviewSummary, error)
	DismissReview(ctx context.Context, owner, repo string, pullNumber int, reviewID int64, message string) (*github.DismissReviewResponse, error)
	ListPullRequestComments(ctx context.Context, owner, repo string, pullNumber int) ([]github.PullRequestComment, error)
}

// ReviewPoster orchestrates posting code review findings to GitHub as PR reviews.
// It determines the appropriate review event based on finding severities,
// filters out findings that are not in the diff, and delegates the actual
// API call to the ReviewClient.
type ReviewPoster struct {
	client           ReviewClient
	semanticComparer dedup.SemanticComparer
	semanticConfig   SemanticDedupConfig
}

// SemanticDedupConfig configures semantic deduplication behavior.
type SemanticDedupConfig struct {
	// LineThreshold is the maximum line distance for candidate pairing.
	LineThreshold int

	// MaxCandidates limits the number of candidates per review (cost guard).
	MaxCandidates int
}

// DefaultSemanticDedupConfig returns sensible defaults for semantic deduplication.
func DefaultSemanticDedupConfig() SemanticDedupConfig {
	return SemanticDedupConfig{
		LineThreshold: 50, // Increased from 10 to catch more duplicates (Issue #165)
		MaxCandidates: 50,
	}
}

// ReviewPosterOption configures a ReviewPoster.
type ReviewPosterOption func(*ReviewPoster)

// WithSemanticComparer sets the semantic comparer for LLM-based deduplication.
func WithSemanticComparer(comparer dedup.SemanticComparer, config SemanticDedupConfig) ReviewPosterOption {
	return func(p *ReviewPoster) {
		p.semanticComparer = comparer
		p.semanticConfig = config
	}
}

// NewReviewPoster creates a new ReviewPoster with the given client.
func NewReviewPoster(client ReviewClient, opts ...ReviewPosterOption) *ReviewPoster {
	p := &ReviewPoster{
		client:         client,
		semanticConfig: DefaultSemanticDedupConfig(),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// PostReviewRequest contains all data needed to post a review.
type PostReviewRequest struct {
	// Owner is the GitHub repository owner (user or organization).
	Owner string

	// Repo is the GitHub repository name.
	Repo string

	// PullNumber is the PR number.
	PullNumber int

	// CommitSHA is the head commit SHA of the PR.
	CommitSHA string

	// Review contains the summary and other metadata.
	Review domain.Review

	// Findings are the positioned findings to post as inline comments.
	Findings []github.PositionedFinding

	// Diff is the parsed diff for summary generation (Issue #125).
	// If provided, the summary is rebuilt after deduplication to show accurate counts.
	Diff *domain.Diff

	// OverrideEvent optionally overrides the automatically determined event.
	// If set, this event will be used instead of determining from severities.
	OverrideEvent github.ReviewEvent

	// ReviewActions configures the review action for each finding severity.
	// If empty, sensible defaults are used (critical/high → request_changes,
	// medium/low → comment, clean → approve).
	ReviewActions github.ReviewActions

	// BotUsername is the username to match for auto-dismissing stale reviews.
	// If empty, no reviews are dismissed. Example: "github-actions[bot]"
	BotUsername string

	// Cost is the total cost of this review in USD.
	// Used for cost tracking in the summary.
	Cost float64
}

// PostReviewResult contains the result of posting a review.
type PostReviewResult struct {
	// ReviewID is the GitHub review ID.
	ReviewID int64

	// CommentsPosted is the number of inline comments we attempted to post.
	// This is the count of in-diff findings after deduplication.
	CommentsPosted int

	// CommentsSkipped is the number of findings skipped (not in diff).
	CommentsSkipped int

	// DuplicatesSkipped is the number of findings skipped because they were
	// already posted in previous reviews (exact fingerprint match).
	DuplicatesSkipped int

	// SemanticDuplicatesSkipped is the number of findings skipped because they
	// were determined to be semantic duplicates of existing findings (LLM-based).
	SemanticDuplicatesSkipped int

	// Event is the review event that was used.
	Event github.ReviewEvent

	// HTMLURL is the URL to view the review on GitHub.
	HTMLURL string

	// DismissedCount is the number of previous bot reviews that were dismissed.
	DismissedCount int

	// Status counts from reply analysis (Issue #108)
	// AcknowledgedCount is the number of existing findings with acknowledgment replies.
	AcknowledgedCount int

	// DisputedCount is the number of existing findings with dispute replies.
	DisputedCount int

	// OpenCount is the number of existing findings with no status-changing replies.
	OpenCount int

	// Comment verification fields (Issue #129)
	// CommentsVerified is the actual number of comments GitHub accepted for this review.
	// A value of -1 indicates verification failed (e.g., API error fetching comments).
	// A value of 0 when CommentsPosted > 0 may indicate GitHub silently dropped comments.
	CommentsVerified int

	// CommentMismatch is true when CommentsPosted != CommentsVerified and verification succeeded.
	// This indicates GitHub silently rejected some comments (e.g., stale diff positions).
	CommentMismatch bool

	// Cost tracking fields
	// CurrentCost is the cost of this review in USD.
	CurrentCost float64

	// PriorCost is the sum of costs from previous bot reviews on this PR.
	PriorCost float64

	// CumulativeCost is the total cost across all reviews (CurrentCost + PriorCost).
	CumulativeCost float64
}

// PostReview posts a code review to GitHub.
// It converts domain findings to GitHub review comments, determines the
// appropriate review event based on severity, and posts the review.
//
// ## Two-Stage Deduplication (Issue #138)
//
// This function implements OUTPUT-SIDE deduplication - filtering findings AFTER
// the LLM generates them. This is one of two complementary deduplication stages:
//
//  1. INPUT-SIDE (prompt injection): Prior triage context is injected into the LLM
//     prompt via TriageContextFetcher, instructing it not to re-raise acknowledged
//     or disputed findings. This is a best-effort prevention mechanism.
//
//  2. OUTPUT-SIDE (this function): Fingerprint and semantic deduplication catch any
//     duplicates that slip through. This is the safety net ensuring we never actually
//     post duplicate comments to GitHub.
//
// Both stages are necessary: input-side reduces noise and saves tokens, but LLMs
// may still generate similar findings. Output-side guarantees no duplicates are posted.
//
// ## Behavior when BotUsername is set:
//   - Existing bot comments are fetched to deduplicate findings (Issue #107)
//   - Reply statuses are analyzed to determine acknowledged/disputed findings (Issue #108)
//   - Acknowledged/disputed findings don't count toward blocking
//   - Previous reviews from that bot are dismissed AFTER posting succeeds
//
// This ensures the PR always has at least one review signal - if posting fails,
// previous reviews are preserved. Dismiss failures are logged but do not affect
// the result.
//
// Findings without a DiffPosition (not in diff) are silently skipped and
// counted in CommentsSkipped. Findings already posted (matching fingerprint)
// are counted in DuplicatesSkipped.
func (p *ReviewPoster) PostReview(ctx context.Context, req PostReviewRequest) (*PostReviewResult, error) {
	// Validate and normalize OverrideEvent if set (Issue #36)
	if req.OverrideEvent != "" {
		normalized, valid := github.NormalizeAction(string(req.OverrideEvent))
		if !valid {
			return nil, fmt.Errorf("invalid OverrideEvent: %q (must be APPROVE, REQUEST_CHANGES, or COMMENT)", req.OverrideEvent)
		}
		req.OverrideEvent = normalized
	}

	findings := req.Findings
	var duplicatesSkipped int
	var semanticDuplicatesSkipped int
	var semanticDupMap SemanticDuplicateMap
	var existingStatuses map[domain.FindingFingerprint]domain.FindingStatus
	var statusCounts StatusCounts
	var priorCost float64
	var existingReviews []github.ReviewSummary

	// Analyze existing data if BotUsername is set
	if req.BotUsername != "" {
		var comments []github.PullRequestComment
		var err error

		// Fetch comments for deduplication and status analysis
		comments, err = p.client.ListPullRequestComments(ctx, req.Owner, req.Repo, req.PullNumber)
		if err != nil {
			log.Printf("warning: failed to fetch comments: %v", err)
		} else {
			// Stage 1: Fingerprint deduplication (exact match)
			findings, duplicatesSkipped = filterDuplicateFindings(req.Findings, comments, req.BotUsername)

			// Stage 2: Semantic deduplication (LLM-based) - Issue #111
			// Also returns a mapping from new fingerprints to existing fingerprints for status inheritance
			if p.semanticComparer != nil && len(findings) > 0 {
				findings, semanticDuplicatesSkipped, semanticDupMap = p.filterSemanticDuplicates(ctx, findings, comments, req.BotUsername)
			}

			// Analyze reply statuses (Issue #108)
			existingStatuses, statusCounts = analyzeFindingStatuses(comments, req.BotUsername)
		}

		// Fetch reviews for prior cost calculation and later dismissal
		existingReviews, err = p.client.ListReviews(ctx, req.Owner, req.Repo, req.PullNumber)
		if err != nil {
			log.Printf("warning: failed to fetch reviews for cost tracking: %v", err)
		} else {
			priorCost = extractPriorCosts(existingReviews, req.BotUsername)
		}
	}

	// Count in-diff vs out-of-diff findings (after deduplication)
	inDiffCount := github.CountInDiffFindings(findings)
	skippedCount := len(findings) - inDiffCount

	// Determine review event considering reply statuses.
	// Acknowledged/disputed findings don't count toward blocking.
	// NOTE: We use req.Findings (original, unfiltered) rather than the deduplicated
	// `findings` because even duplicated high-severity findings should still block
	// the PR if they haven't been acknowledged/disputed. The deduplication only
	// affects what comments are posted, not the blocking decision.
	// However, semantic duplicates are handled specially via semanticDupMap - they
	// inherit the status of the original finding they duplicate (fix for bug #149).
	var event github.ReviewEvent
	if req.OverrideEvent != "" {
		event = req.OverrideEvent
	} else {
		event = determineEffectiveEvent(req.Findings, existingStatuses, semanticDupMap, req.ReviewActions)
	}

	// Build dedup stats for summary and logging (Issue #126, #127)
	dedupStats := DedupStats{
		TotalFindings:         len(req.Findings),
		FingerprintDuplicates: duplicatesSkipped,
		SemanticDuplicates:    semanticDuplicatesSkipped,
		NewFindings:           len(findings),
	}

	// Log deduplication breakdown (Issue #127) - always log so users know dedup was checked
	log.Printf("deduplication: %d findings → %d new (%d fingerprint, %d semantic duplicates)",
		dedupStats.TotalFindings, dedupStats.NewFindings,
		dedupStats.FingerprintDuplicates, dedupStats.SemanticDuplicates)

	// Build cost stats for summary
	costStats := CostStats{
		CurrentCost:    req.Cost,
		PriorCost:      priorCost,
		CumulativeCost: req.Cost + priorCost,
	}

	// Build the summary AFTER deduplication so counts reflect what's actually posted (Issue #125).
	// If Diff is provided, rebuild the programmatic summary with deduplicated findings.
	var summary string
	if req.Diff != nil {
		// Rebuild summary with accurate counts from deduplicated findings
		programmaticSummary := github.BuildProgrammaticSummary(findings, *req.Diff, req.ReviewActions)
		appendix := github.BuildSummaryAppendix(findings, *req.Diff)
		dedupSection := formatDedupSection(dedupStats)
		statusSection := formatStatusSection(statusCounts)
		costSection := formatCostSection(costStats)
		summary = github.AppendSections(programmaticSummary, appendix) + dedupSection + statusSection + costSection
	} else {
		// Fall back to pre-built summary (legacy behavior)
		// Note: This may contain stale counts if deduplication removed findings
		log.Println("[WARN] No Diff provided to PostReview; using pre-built summary (counts may be stale)")
		dedupSection := formatDedupSection(dedupStats)
		statusSection := formatStatusSection(statusCounts)
		costSection := formatCostSection(costStats)
		summary = req.Review.Summary + dedupSection + statusSection + costSection
	}

	// Call the client to create the new review first
	input := github.CreateReviewInput{
		Owner:         req.Owner,
		Repo:          req.Repo,
		PullNumber:    req.PullNumber,
		CommitSHA:     req.CommitSHA,
		Event:         event,
		Summary:       summary,
		Findings:      findings,
		ReviewActions: req.ReviewActions, // Pass through for accurate blocking indicators
	}

	resp, err := p.client.CreateReview(ctx, input)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("CreateReview returned nil response")
	}

	// Dismiss previous bot reviews AFTER successful post
	// This ensures PR always has review signal - if post failed, old reviews remain
	// Pass the new review ID to avoid dismissing the review we just created
	// Use existingReviews if we already fetched them, otherwise let dismissStaleReviews fetch
	var dismissedCount int
	if req.BotUsername != "" {
		dismissedCount = p.dismissStaleReviewsFromList(ctx, req.Owner, req.Repo, req.PullNumber, req.BotUsername, resp.ID, existingReviews)
	}

	// Verify that comments were actually posted (Issue #129)
	// This detects when GitHub silently drops comments due to stale positions
	commentsVerified, commentMismatch := p.verifyPostedComments(
		ctx, req.Owner, req.Repo, req.PullNumber, req.BotUsername, resp.ID, inDiffCount,
	)

	return &PostReviewResult{
		ReviewID:                  resp.ID,
		CommentsPosted:            inDiffCount,
		CommentsSkipped:           skippedCount,
		DuplicatesSkipped:         duplicatesSkipped,
		SemanticDuplicatesSkipped: semanticDuplicatesSkipped,
		Event:                     event,
		HTMLURL:                   resp.HTMLURL,
		DismissedCount:            dismissedCount,
		AcknowledgedCount:         statusCounts.Acknowledged,
		DisputedCount:             statusCounts.Disputed,
		OpenCount:                 statusCounts.Open,
		CommentsVerified:          commentsVerified,
		CommentMismatch:           commentMismatch,
		CurrentCost:               costStats.CurrentCost,
		PriorCost:                 costStats.PriorCost,
		CumulativeCost:            costStats.CumulativeCost,
	}, nil
}

// dismissStaleReviewsFromList dismisses previous bot reviews using a pre-fetched list.
// If reviews is nil, it fetches the list first (fallback behavior).
// The excludeReviewID parameter specifies a review ID to skip (typically the
// newly created review). Returns the number of reviews dismissed. Errors are
// logged but do not block the review posting workflow.
func (p *ReviewPoster) dismissStaleReviewsFromList(ctx context.Context, owner, repo string, pullNumber int, botUsername string, excludeReviewID int64, reviews []github.ReviewSummary) int {
	// Fetch reviews if not provided
	if reviews == nil {
		var err error
		reviews, err = p.client.ListReviews(ctx, owner, repo, pullNumber)
		if err != nil {
			log.Printf("warning: failed to list reviews for dismissal: %v", err)
			return 0
		}
	}

	var dismissedCount int
	for _, review := range reviews {
		// Skip the newly created review to avoid dismissing our own fresh review
		if review.ID == excludeReviewID {
			continue
		}
		if shouldDismissReview(review, botUsername) {
			_, err := p.client.DismissReview(ctx, owner, repo, pullNumber, review.ID, "Superseded by new review")
			if err != nil {
				log.Printf("warning: failed to dismiss review %d: %v", review.ID, err)
				continue
			}
			dismissedCount++
		}
	}

	return dismissedCount
}

// shouldDismissReview returns true if the review should be dismissed.
// A review should be dismissed if it's from the bot and not already dismissed.
func shouldDismissReview(review github.ReviewSummary, botUsername string) bool {
	// Case-insensitive comparison for usernames (GitHub usernames are case-insensitive)
	if !strings.EqualFold(review.User.Login, botUsername) {
		return false
	}

	// Skip already dismissed reviews
	if review.State == string(github.StateDismissed) {
		return false
	}

	// Skip pending reviews (not yet submitted)
	if review.State == string(github.StatePending) {
		return false
	}

	// Dismiss all other states (APPROVED, CHANGES_REQUESTED, COMMENTED)
	return true
}

// verifyPostedComments verifies that the expected number of comments were actually
// posted to GitHub. This detects when GitHub silently drops comments due to stale
// diff positions or other validation failures.
//
// Returns:
//   - commentsVerified: actual count of comments from this review (-1 if verification failed)
//   - commentMismatch: true if expected != actual and verification succeeded
//
// If botUsername is empty, verification is skipped (returns 0, false).
// Errors are logged but do not affect the main review posting workflow.
func (p *ReviewPoster) verifyPostedComments(
	ctx context.Context,
	owner, repo string,
	pullNumber int,
	botUsername string,
	reviewID int64,
	expectedCount int,
) (commentsVerified int, commentMismatch bool) {
	// Skip verification if no bot username (can't identify our comments)
	if botUsername == "" {
		return 0, false
	}

	// Skip verification if we didn't expect any comments
	if expectedCount == 0 {
		return 0, false
	}

	// Fetch all comments on the PR
	comments, err := p.client.ListPullRequestComments(ctx, owner, repo, pullNumber)
	if err != nil {
		log.Printf("[WARN] Failed to verify posted comments (Issue #129): %v", err)
		return -1, false // -1 indicates verification failed
	}

	// Count comments that belong to this specific review
	actualCount := 0
	for _, comment := range comments {
		if comment.PullRequestReviewID == reviewID {
			actualCount++
		}
	}

	// Check for mismatch
	if actualCount != expectedCount {
		log.Printf("[WARN] Comment count mismatch (Issue #129): expected %d, got %d for review %d on %s/%s#%d",
			expectedCount, actualCount, reviewID, owner, repo, pullNumber)
		return actualCount, true
	}

	return actualCount, false
}

// StatusCounts tracks the count of findings by status.
type StatusCounts struct {
	Open         int
	Acknowledged int
	Disputed     int
}

// filterDuplicateFindings filters out findings that have already been posted.
// Returns the filtered findings and the count of duplicates skipped.
func filterDuplicateFindings(
	findings []github.PositionedFinding,
	comments []github.PullRequestComment,
	botUsername string,
) ([]github.PositionedFinding, int) {
	existingFingerprints := extractBotFingerprints(comments, botUsername)
	if len(existingFingerprints) == 0 {
		return findings, 0
	}

	var filtered []github.PositionedFinding
	var duplicatesSkipped int
	for _, pf := range findings {
		fp := domain.FingerprintFromFinding(pf.Finding)
		if existingFingerprints[fp] {
			duplicatesSkipped++
			continue
		}
		filtered = append(filtered, pf)
	}

	return filtered, duplicatesSkipped
}

// extractBotFingerprints extracts fingerprints from comments authored by the bot.
// Returns a map of fingerprints for O(1) lookup.
func extractBotFingerprints(comments []github.PullRequestComment, botUsername string) map[domain.FindingFingerprint]bool {
	fingerprints := make(map[domain.FindingFingerprint]bool)

	for _, comment := range comments {
		// Skip comments not from the bot (case-insensitive)
		if !strings.EqualFold(comment.User.Login, botUsername) {
			continue
		}

		// Try to extract fingerprint from comment body
		if fp, ok := github.ExtractFingerprintFromComment(comment.Body); ok {
			fingerprints[fp] = true
		}
	}

	return fingerprints
}

// FindingStatusInfo contains the status and rationale for a finding.
// The rationale is the actual reply body that determined the status,
// which provides context for why a finding was acknowledged or disputed.
type FindingStatusInfo struct {
	Status    domain.FindingStatus
	Rationale string // The reply body that determined the status
}

// analyzeFindingStatusInfo analyzes bot comments and their replies to determine
// the status and rationale for each existing finding.
// Returns a map of fingerprint → status info (including reply body) and counts.
func analyzeFindingStatusInfo(
	comments []github.PullRequestComment,
	botUsername string,
) (map[domain.FindingFingerprint]FindingStatusInfo, StatusCounts) {
	statusInfo := make(map[domain.FindingFingerprint]FindingStatusInfo)
	var counts StatusCounts

	// Group comments by parent to get reply chains
	grouped := github.GroupCommentsByParent(comments, botUsername)

	for _, group := range grouped {
		// Extract fingerprint from parent (bot) comment
		fp, ok := github.ExtractFingerprintFromComment(group.Parent.Body)
		if !ok {
			continue // Skip comments without fingerprints (legacy)
		}

		// Collect reply texts and find the determining reply
		var replyTexts []string
		var determiningReply string
		for _, reply := range group.Replies {
			replyTexts = append(replyTexts, reply.Body)
		}

		// Detect status from replies
		status := domain.DetectStatusFromReplies(replyTexts)

		// Find the reply that determined the status (first match)
		// This mirrors the logic in DetectStatusFromReplies
		if status != domain.StatusOpen {
			for _, reply := range group.Replies {
				replyStatus := domain.DetectStatusFromText(reply.Body)
				if replyStatus == status {
					determiningReply = reply.Body
					break
				}
			}
		}

		statusInfo[fp] = FindingStatusInfo{
			Status:    status,
			Rationale: determiningReply,
		}

		// Update counts
		switch status {
		case domain.StatusAcknowledged:
			counts.Acknowledged++
		case domain.StatusDisputed:
			counts.Disputed++
		case domain.StatusOpen:
			counts.Open++
		}
	}

	return statusInfo, counts
}

// analyzeFindingStatuses analyzes bot comments and their replies to determine
// the status of each existing finding (Issue #108).
// Returns a map of fingerprint → status and counts for each status.
func analyzeFindingStatuses(
	comments []github.PullRequestComment,
	botUsername string,
) (map[domain.FindingFingerprint]domain.FindingStatus, StatusCounts) {
	// Delegate to the richer function and extract just the status
	statusInfo, counts := analyzeFindingStatusInfo(comments, botUsername)

	statuses := make(map[domain.FindingFingerprint]domain.FindingStatus, len(statusInfo))
	for fp, info := range statusInfo {
		statuses[fp] = info.Status
	}

	return statuses, counts
}

// DedupStats tracks deduplication statistics for summary display.
type DedupStats struct {
	// TotalFindings is the total findings before deduplication.
	TotalFindings int

	// FingerprintDuplicates is the count of exact fingerprint matches skipped.
	FingerprintDuplicates int

	// SemanticDuplicates is the count of LLM-detected semantic duplicates skipped.
	SemanticDuplicates int

	// NewFindings is the count of findings that will be posted (TotalFindings - duplicates).
	NewFindings int
}

// formatDedupSection creates a markdown section showing deduplication statistics.
// Shows stats when there are findings (even if 0 duplicates) so users know dedup was checked.
// Returns empty string only when there are no findings at all.
func formatDedupSection(stats DedupStats) string {
	// Don't show section if there are no findings to report on
	if stats.TotalFindings == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n---\n\n")
	sb.WriteString("### Deduplication\n\n")

	totalDuplicates := stats.FingerprintDuplicates + stats.SemanticDuplicates
	if totalDuplicates == 0 {
		sb.WriteString(fmt.Sprintf("🔄 **%d findings** | 0 duplicates\n", stats.NewFindings))
	} else {
		sb.WriteString(fmt.Sprintf("🔄 **%d new** | %d fingerprint | %d semantic duplicates skipped\n",
			stats.NewFindings, stats.FingerprintDuplicates, stats.SemanticDuplicates))
	}

	return sb.String()
}

// CostStats tracks review cost information for summary display.
type CostStats struct {
	// CurrentCost is the cost of this review in USD.
	CurrentCost float64

	// PriorCost is the sum of costs from previous reviews on this PR.
	PriorCost float64

	// CumulativeCost is CurrentCost + PriorCost.
	CumulativeCost float64
}

// formatCostSection creates a markdown section showing review costs.
// Returns an empty string if cost is zero.
func formatCostSection(stats CostStats) string {
	if stats.CurrentCost == 0 && stats.PriorCost == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n---\n\n")
	sb.WriteString("### Costs\n\n")

	if stats.PriorCost > 0 {
		sb.WriteString(fmt.Sprintf("💰 This review: $%.4f | Cumulative: $%.4f\n",
			stats.CurrentCost, stats.CumulativeCost))
	} else {
		sb.WriteString(fmt.Sprintf("💰 This review: $%.4f\n", stats.CurrentCost))
	}

	// Embed cost marker for future extraction (invisible in rendered markdown)
	sb.WriteString("\n")
	sb.WriteString(github.FormatCostMarker(stats.CurrentCost))

	return sb.String()
}

// extractPriorCosts calculates the total cost from previous bot reviews on this PR.
// It parses the embedded cost marker from each bot review's body.
func extractPriorCosts(reviews []github.ReviewSummary, botUsername string) float64 {
	var totalCost float64

	for _, review := range reviews {
		// Skip reviews not from the bot (case-insensitive)
		if !strings.EqualFold(review.User.Login, botUsername) {
			continue
		}

		// Include ALL bot reviews (even DISMISSED) for true cumulative cost.
		// Dismissed reviews are prior reviews we dismissed after posting new ones,
		// so their costs are still part of the PR's total review spend.
		if cost, ok := github.ExtractCostFromReview(review.Body); ok {
			totalCost += cost
		}
	}

	return totalCost
}

// formatStatusSection creates a markdown section showing finding status breakdown.
// Returns an empty string if all counts are zero.
func formatStatusSection(counts StatusCounts) string {
	// Only include section if there are any existing findings tracked
	total := counts.Open + counts.Acknowledged + counts.Disputed
	if total == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n---\n\n")
	sb.WriteString("### Existing Finding Status\n\n")

	// Always show all statuses for clarity
	sb.WriteString("| Status | Count | Effect |\n")
	sb.WriteString("|--------|-------|--------|\n")
	sb.WriteString("| 🔓 Open | ")
	sb.WriteString(fmt.Sprintf("%d", counts.Open))
	sb.WriteString(" | Counts toward blocking |\n")
	sb.WriteString("| ✅ Acknowledged | ")
	sb.WriteString(fmt.Sprintf("%d", counts.Acknowledged))
	sb.WriteString(" | Won't block (author accepted) |\n")
	sb.WriteString("| ❌ Disputed | ")
	sb.WriteString(fmt.Sprintf("%d", counts.Disputed))
	sb.WriteString(" | Won't block (author disputes) |\n")

	return sb.String()
}

// determineEffectiveEvent determines the review event considering reply statuses.
// Findings that have been acknowledged or disputed don't count toward blocking.
func determineEffectiveEvent(
	findings []github.PositionedFinding,
	existingStatuses map[domain.FindingFingerprint]domain.FindingStatus,
	semanticDupMap SemanticDuplicateMap,
	actions github.ReviewActions,
) github.ReviewEvent {
	// If no status tracking, fall back to standard behavior
	if existingStatuses == nil {
		return github.DetermineReviewEventWithActions(findings, actions)
	}

	// Filter findings to only include those that are "effective" (not acknowledged/disputed)
	var effectiveFindings []github.PositionedFinding
	for _, pf := range findings {
		fp := domain.FingerprintFromFinding(pf.Finding)

		// Check if this finding's fingerprint is directly in existingStatuses
		status, exists := existingStatuses[fp]

		// If not found directly, check if it's a semantic duplicate of an existing finding.
		// Semantic duplicates should inherit the status of the original finding they duplicate.
		// Fix for bug #149: without this, semantic duplicates incorrectly block the PR.
		if !exists && semanticDupMap != nil {
			if originalFP, isDup := semanticDupMap[fp]; isDup {
				status, exists = existingStatuses[originalFP]
			}
		}

		// Include finding if:
		// - It's new (not in existingStatuses, and not a semantic duplicate of an existing finding)
		// - It's in existingStatuses but status is Open
		if !exists || status == domain.StatusOpen {
			effectiveFindings = append(effectiveFindings, pf)
		}
		// Acknowledged and Disputed findings are excluded from blocking calculation
	}

	return github.DetermineReviewEventWithActions(effectiveFindings, actions)
}

// SemanticDuplicateMap maps new finding fingerprints to the existing fingerprints they duplicate.
// This allows determineEffectiveEvent to inherit the status of the original finding.
type SemanticDuplicateMap map[domain.FindingFingerprint]domain.FindingFingerprint

// filterSemanticDuplicates uses LLM-based comparison to identify findings that are
// semantic duplicates of existing comments, even if they have different fingerprints.
// Returns the filtered findings, count of semantic duplicates found, and a mapping
// from new finding fingerprints to the existing fingerprints they duplicate.
func (p *ReviewPoster) filterSemanticDuplicates(
	ctx context.Context,
	findings []github.PositionedFinding,
	comments []github.PullRequestComment,
	botUsername string,
) ([]github.PositionedFinding, int, SemanticDuplicateMap) {
	// Convert bot comments to ExistingFinding for candidate detection
	existingFindings := extractExistingFindings(comments, botUsername)
	if len(existingFindings) == 0 {
		return findings, 0, nil
	}

	// Convert positioned findings to domain.Finding for candidate detection
	var domainFindings []domain.Finding
	for _, pf := range findings {
		domainFindings = append(domainFindings, pf.Finding)
	}

	// Find candidate pairs (spatially overlapping findings)
	candidates, overflow := dedup.FindCandidates(
		domainFindings,
		existingFindings,
		p.semanticConfig.LineThreshold,
		p.semanticConfig.MaxCandidates,
	)

	// If no candidates, nothing to compare
	if len(candidates) == 0 {
		return findings, 0, nil
	}

	// Build set of original indices that were included in candidates (not overflow).
	// This prevents overflow findings from being incorrectly marked as duplicates
	// if they happen to have identical fields to a candidate that was marked duplicate.
	candidateOriginalIndices := make(map[int]bool)
	for _, cp := range candidates {
		for origIdx, pf := range findings {
			if cp.New.File == pf.Finding.File &&
				cp.New.LineStart == pf.Finding.LineStart &&
				cp.New.LineEnd == pf.Finding.LineEnd &&
				cp.New.Category == pf.Finding.Category &&
				cp.New.Severity == pf.Finding.Severity &&
				cp.New.Description == pf.Finding.Description {
				candidateOriginalIndices[origIdx] = true
				break // Each candidate maps to one original
			}
		}
	}

	// Call the semantic comparer
	result, err := p.semanticComparer.Compare(ctx, candidates)
	if err != nil {
		// Fail open: on error, treat all findings as unique
		log.Printf("warning: semantic dedup failed: %v (treating all as unique)", err)
		return findings, 0, nil
	}

	// Mark original finding indices that are semantic duplicates.
	// Only consider indices that were actually sent as candidates (not overflow).
	// Also build a map of new fingerprints -> existing fingerprints for status inheritance.
	duplicateOriginalIndices := make(map[int]bool)
	semanticDupMap := make(SemanticDuplicateMap)
	for _, dup := range result.Duplicates {
		fp := domain.FingerprintFromFinding(dup.NewFinding)
		log.Printf("semantic dedup: %s is duplicate of existing (reason: %s)", fp, dup.Reason)

		// Record the mapping from new fingerprint to existing fingerprint
		// This allows determineEffectiveEvent to inherit the original's status
		semanticDupMap[fp] = dup.ExistingFingerprint

		// Find which original indices correspond to this duplicate's new finding
		for origIdx, pf := range findings {
			if !candidateOriginalIndices[origIdx] {
				continue // Skip overflow findings
			}
			if dup.NewFinding.File == pf.Finding.File &&
				dup.NewFinding.LineStart == pf.Finding.LineStart &&
				dup.NewFinding.LineEnd == pf.Finding.LineEnd &&
				dup.NewFinding.Category == pf.Finding.Category &&
				dup.NewFinding.Severity == pf.Finding.Severity &&
				dup.NewFinding.Description == pf.Finding.Description {
				duplicateOriginalIndices[origIdx] = true
			}
		}
	}

	// Overflow findings remain in the original list (couldn't verify them, fail-open)
	if len(overflow) > 0 {
		log.Printf("semantic dedup: %d candidates exceeded limit, treating as unique", len(overflow))
	}

	// Filter out semantic duplicates from positioned findings by index
	var filtered []github.PositionedFinding
	var duplicatesFound int
	for i, pf := range findings {
		if duplicateOriginalIndices[i] {
			duplicatesFound++
			continue
		}
		filtered = append(filtered, pf)
	}

	return filtered, duplicatesFound, semanticDupMap
}

// extractExistingFindings converts bot comments to ExistingFinding for semantic comparison.
func extractExistingFindings(comments []github.PullRequestComment, botUsername string) []dedup.ExistingFinding {
	var existing []dedup.ExistingFinding

	for _, comment := range comments {
		// Skip comments not from the bot
		if !strings.EqualFold(comment.User.Login, botUsername) {
			continue
		}

		// Skip replies (only process top-level comments)
		if comment.InReplyToID != 0 {
			continue
		}

		// Extract structured details from comment body
		details := github.ExtractCommentDetails(comment.Body)
		if details == nil {
			continue // Not a structured finding comment
		}

		// Use line from API if available, otherwise use parsed line
		lineStart := details.LineStart
		lineEnd := details.LineEnd
		if comment.Line != nil && *comment.Line > 0 {
			lineStart = *comment.Line
			if lineEnd == 0 {
				lineEnd = lineStart
			}
		}

		existing = append(existing, dedup.ExistingFinding{
			Fingerprint: details.Fingerprint,
			File:        comment.Path,
			LineStart:   lineStart,
			LineEnd:     lineEnd,
			Description: details.Description,
			Severity:    details.Severity,
			Category:    details.Category,
		})
	}

	return existing
}
