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

// FetchTriagedFindings retrieves ALL previously posted bot findings for a PR.
// This includes findings with any status: open, acknowledged, or disputed.
// Returns nil if there are no findings, if the bot username is not set,
// or if fetching fails. Errors are logged but not returned to avoid blocking the review.
//
// Including all findings (not just acknowledged/disputed) prevents the LLM from
// re-raising issues that were already posted in earlier review rounds (Issue #165).
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

	// Analyze triage statuses with rationale using existing logic from poster.go
	statusInfo, _ := analyzeFindingStatusInfo(comments, f.botUsername)
	if len(statusInfo) == 0 {
		return nil
	}

	// Extract triaged findings with actual reply rationale
	findings := f.extractTriagedFindings(comments, statusInfo)
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
// Uses the actual reply rationale when available for better LLM context.
func (f *TriageContextFetcher) extractTriagedFindings(
	comments []github.PullRequestComment,
	statusInfo map[domain.FindingFingerprint]FindingStatusInfo,
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

		// Get status info if available (may be empty for findings with no replies)
		// Include ALL bot findings regardless of status to prevent LLM from re-raising them
		info := statusInfo[fp]

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
			Status:      info.Status,
			Fingerprint: fp,
		}

		// Set status reason - prefer actual rationale over generic text
		switch info.Status {
		case domain.StatusAcknowledged:
			if info.Rationale != "" {
				tf.StatusReason = info.Rationale
			} else {
				tf.StatusReason = domain.StatusReasonForAcknowledged()
			}
		case domain.StatusDisputed:
			if info.Rationale != "" {
				tf.StatusReason = info.Rationale
			} else {
				tf.StatusReason = domain.StatusReasonForDisputed()
			}
		default:
			// StatusOpen or empty - finding was posted but not yet replied to
			tf.Status = domain.StatusOpen
			tf.StatusReason = domain.StatusReasonForOpen()
		}

		findings = append(findings, tf)
	}

	return findings
}
