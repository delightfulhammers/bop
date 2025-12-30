package github

import (
	"context"
	"log"
	"strings"

	"github.com/bkyoung/code-reviewer/internal/adapter/github"
	"github.com/bkyoung/code-reviewer/internal/domain"
)

// TriageContextFetcher fetches prior triage context from GitHub PR comments.
// It implements the review.TriageContextFetcher interface.
//
// This is used for INPUT-SIDE deduplication: injecting previously-addressed findings
// into the LLM prompt so it doesn't re-raise them. This complements the OUTPUT-SIDE
// deduplication in PostReview which catches any duplicates that slip through.
type TriageContextFetcher struct {
	client      ReviewClient
	botUsername string
}

// NewTriageContextFetcher creates a new fetcher with the given client and bot username.
// The botUsername is used to identify which comments are from the code reviewer bot.
func NewTriageContextFetcher(client ReviewClient, botUsername string) *TriageContextFetcher {
	return &TriageContextFetcher{
		client:      client,
		botUsername: botUsername,
	}
}

// FetchTriagedFindings retrieves findings that have been acknowledged or disputed.
// Returns nil if there are no triaged findings, if the bot username is not set,
// or if fetching fails. Errors are logged but not returned to avoid blocking the review.
//
// This method satisfies the review.TriageContextFetcher interface.
func (f *TriageContextFetcher) FetchTriagedFindings(
	ctx context.Context,
	owner, repo string,
	prNumber int,
) *domain.TriagedFindingContext {
	// Skip if no bot username configured
	if f.botUsername == "" {
		return nil
	}

	// Fetch all PR comments
	comments, err := f.client.ListPullRequestComments(ctx, owner, repo, prNumber)
	if err != nil {
		log.Printf("[WARN] Failed to fetch comments for prior triage context: %v", err)
		return nil
	}

	// Analyze triage statuses using existing logic from poster.go
	statuses, _ := analyzeFindingStatuses(comments, f.botUsername)
	if len(statuses) == 0 {
		return nil
	}

	// Extract triaged findings (acknowledged or disputed only)
	findings := f.extractTriagedFindings(comments, statuses)
	if len(findings) == 0 {
		return nil
	}

	return &domain.TriagedFindingContext{
		PRNumber: prNumber,
		Findings: findings,
	}
}

// extractTriagedFindings converts bot comments with acknowledged/disputed status
// into TriagedFinding structs for prompt injection.
func (f *TriageContextFetcher) extractTriagedFindings(
	comments []github.PullRequestComment,
	statuses map[domain.FindingFingerprint]domain.FindingStatus,
) []domain.TriagedFinding {
	var findings []domain.TriagedFinding

	for _, comment := range comments {
		// Skip comments not from the bot
		if !strings.EqualFold(comment.User.Login, f.botUsername) {
			continue
		}

		// Skip replies (only process top-level finding comments)
		if comment.InReplyToID != 0 {
			continue
		}

		// Extract fingerprint and details from comment
		fp, ok := github.ExtractFingerprintFromComment(comment.Body)
		if !ok {
			continue // Not a structured finding comment
		}

		// Check if this finding has a triaged status
		status, exists := statuses[fp]
		if !exists || status == domain.StatusOpen {
			continue // Only include acknowledged or disputed findings
		}

		// Extract structured details
		details := github.ExtractCommentDetails(comment.Body)
		if details == nil {
			continue
		}

		// Determine line numbers
		lineStart := details.LineStart
		lineEnd := details.LineEnd
		if comment.Line != nil && *comment.Line > 0 {
			lineStart = *comment.Line
			if lineEnd == 0 {
				lineEnd = lineStart
			}
		}

		// Create triaged finding
		tf := domain.TriagedFinding{
			File:        comment.Path,
			LineStart:   lineStart,
			LineEnd:     lineEnd,
			Category:    details.Category,
			Severity:    details.Severity,
			Description: details.Description,
			Status:      status,
			Fingerprint: fp,
		}

		// Set status reason based on status
		switch status {
		case domain.StatusAcknowledged:
			tf.StatusReason = domain.StatusReasonForAcknowledged()
		case domain.StatusDisputed:
			tf.StatusReason = domain.StatusReasonForDisputed()
		}

		findings = append(findings, tf)
	}

	return findings
}
