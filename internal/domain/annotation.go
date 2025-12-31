package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// AnnotationLevel represents the severity level of a GitHub check run annotation.
type AnnotationLevel string

const (
	// AnnotationLevelNotice indicates an informational annotation.
	AnnotationLevelNotice AnnotationLevel = "notice"

	// AnnotationLevelWarning indicates a warning that should be addressed.
	AnnotationLevelWarning AnnotationLevel = "warning"

	// AnnotationLevelFailure indicates a critical issue that must be fixed.
	AnnotationLevelFailure AnnotationLevel = "failure"
)

// AllAnnotationLevels returns all valid annotation level values.
func AllAnnotationLevels() []AnnotationLevel {
	return []AnnotationLevel{
		AnnotationLevelNotice,
		AnnotationLevelWarning,
		AnnotationLevelFailure,
	}
}

// IsValid returns true if the level is a recognized value.
func (l AnnotationLevel) IsValid() bool {
	for _, valid := range AllAnnotationLevels() {
		if l == valid {
			return true
		}
	}
	return false
}

// Annotation represents a SARIF annotation from a GitHub check run.
// These are created by tools like code-reviewer when posting findings.
type Annotation struct {
	// CheckRunID is the GitHub check run this annotation belongs to.
	CheckRunID int64 `json:"checkRunId"`

	// Index is the position of this annotation within the check run (0-based).
	Index int `json:"index"`

	// Path is the file path relative to the repository root.
	Path string `json:"path"`

	// StartLine is the first line of the annotation (1-based).
	StartLine int `json:"startLine"`

	// EndLine is the last line of the annotation (1-based).
	// If zero, defaults to StartLine (single-line annotation).
	EndLine int `json:"endLine"`

	// Level indicates the severity of the annotation.
	Level AnnotationLevel `json:"level"`

	// Message is the annotation text (may contain markdown).
	Message string `json:"message"`

	// Title is a short summary of the annotation.
	Title string `json:"title,omitempty"`

	// RawDetails contains additional context (e.g., SARIF rule info).
	RawDetails string `json:"rawDetails,omitempty"`
}

// LineRange returns the start and end lines, defaulting EndLine to StartLine if zero.
func (a Annotation) LineRange() (start, end int) {
	start = a.StartLine
	end = a.EndLine
	if end == 0 {
		end = start
	}
	return start, end
}

// MatchesLevelFilter returns true if the annotation matches the given level filter.
// If filter is nil, all annotations match.
func (a Annotation) MatchesLevelFilter(filter *AnnotationLevel) bool {
	if filter == nil {
		return true
	}
	return a.Level == *filter
}

// CheckRunSummary provides summary information about a GitHub check run.
type CheckRunSummary struct {
	// ID is the GitHub check run ID.
	ID int64 `json:"id"`

	// Name is the name of the check (e.g., "code-reviewer").
	Name string `json:"name"`

	// Status is the current status (queued, in_progress, completed).
	Status string `json:"status"`

	// Conclusion is the outcome (success, failure, neutral, etc.).
	// Only set when Status is "completed".
	Conclusion string `json:"conclusion,omitempty"`

	// AnnotationCount is the number of annotations in this check run.
	AnnotationCount int `json:"annotationCount"`

	// StartedAt is when the check run started.
	StartedAt time.Time `json:"startedAt,omitempty"`

	// CompletedAt is when the check run finished.
	CompletedAt time.Time `json:"completedAt,omitempty"`

	// HeadSHA is the commit SHA this check ran against.
	HeadSHA string `json:"headSha"`
}

// HasAnnotations returns true if this check run has any annotations.
func (s CheckRunSummary) HasAnnotations() bool {
	return s.AnnotationCount > 0
}

// IsComplete returns true if the check run has finished.
func (s CheckRunSummary) IsComplete() bool {
	return s.Status == "completed"
}

// MatchesNameFilter returns true if the check run matches the given name filter.
// If filter is nil, all check runs match.
func (s CheckRunSummary) MatchesNameFilter(filter *string) bool {
	if filter == nil {
		return true
	}
	return s.Name == *filter
}

// CodeContext represents a section of source code with context.
type CodeContext struct {
	// File is the path to the file.
	File string `json:"file"`

	// Ref is the git ref (commit SHA, branch, tag) the content is from.
	Ref string `json:"ref"`

	// StartLine is the first line number (1-based).
	StartLine int `json:"startLine"`

	// EndLine is the last line number (1-based).
	EndLine int `json:"endLine"`

	// Content is the actual source code text.
	Content string `json:"content"`

	// ContextBefore is the number of context lines before StartLine.
	ContextBefore int `json:"contextBefore,omitempty"`

	// ContextAfter is the number of context lines after EndLine.
	ContextAfter int `json:"contextAfter,omitempty"`
}

// LineCount returns the number of lines in the range (inclusive).
func (c CodeContext) LineCount() int {
	return c.EndLine - c.StartLine + 1
}

// DiffContext represents a section of diff output centered on specific lines.
type DiffContext struct {
	// File is the path to the file.
	File string `json:"file"`

	// BaseBranch is the base ref for the diff (e.g., "main").
	BaseBranch string `json:"baseBranch"`

	// TargetRef is the target ref for the diff (e.g., PR head SHA).
	TargetRef string `json:"targetRef"`

	// HunkContent is the unified diff hunk(s) covering the requested lines.
	HunkContent string `json:"hunkContent"`

	// StartLine is the first line of interest (1-based, in new file).
	StartLine int `json:"startLine"`

	// EndLine is the last line of interest (1-based, in new file).
	EndLine int `json:"endLine"`
}

// HasChanges returns true if there is diff content.
func (d DiffContext) HasChanges() bool {
	return d.HunkContent != ""
}

// PRFinding represents a finding posted as a PR comment.
// Unlike Annotation (from SARIF/check runs), these are accumulated
// review comments that may have thread history.
type PRFinding struct {
	// CommentID is the GitHub comment ID.
	CommentID int64 `json:"commentId"`

	// Fingerprint is the CR_FP marker extracted from the comment body.
	Fingerprint string `json:"fingerprint,omitempty"`

	// Path is the file path the comment is on.
	Path string `json:"path"`

	// Line is the line number the comment is on.
	Line int `json:"line"`

	// Body is the comment text.
	Body string `json:"body"`

	// Author is the GitHub username who posted the comment.
	Author string `json:"author"`

	// CreatedAt is when the comment was posted.
	CreatedAt time.Time `json:"createdAt"`

	// IsResolved indicates if the thread is marked resolved.
	IsResolved bool `json:"isResolved"`

	// ReplyCount is the number of replies in the thread.
	ReplyCount int `json:"replyCount"`

	// Severity is extracted from the comment body (if present).
	Severity string `json:"severity,omitempty"`

	// Category is extracted from the comment body (if present).
	Category string `json:"category,omitempty"`

	// Reviewer is the persona name extracted from the comment body (if present).
	// This is set from the CR_REVIEWER marker in Phase 3.2 findings.
	Reviewer string `json:"reviewer,omitempty"`
}

// ThreadStatus returns the discussion status of this finding.
func (f PRFinding) ThreadStatus() string {
	if f.IsResolved {
		return "resolved"
	}
	if f.ReplyCount > 0 {
		return "active"
	}
	return "pending"
}

// FindingIDPrefix is the prefix for fingerprint-based finding IDs.
const FindingIDPrefix = "CR_FP:"

// ResolveFindingID parses a finding ID that may be either a fingerprint or comment ID.
// Returns (fingerprint, commentID, isFingerprint).
func ResolveFindingID(id string) (fingerprint string, commentID int64, isFingerprint bool) {
	// Check for fingerprint prefix
	if strings.HasPrefix(id, FindingIDPrefix) {
		return strings.TrimPrefix(id, FindingIDPrefix), 0, true
	}

	// Try parsing as comment ID
	if cid, err := strconv.ParseInt(id, 10, 64); err == nil {
		return "", cid, false
	}

	// Default to treating as fingerprint
	return id, 0, true
}

// PRMetadata contains information about a pull request.
type PRMetadata struct {
	// Owner is the repository owner.
	Owner string `json:"owner"`

	// Repo is the repository name.
	Repo string `json:"repo"`

	// Number is the PR number.
	Number int `json:"number"`

	// HeadRef is the head branch name.
	HeadRef string `json:"headRef"`

	// HeadSHA is the head commit SHA.
	HeadSHA string `json:"headSha"`

	// BaseRef is the base branch name.
	BaseRef string `json:"baseRef"`

	// BaseSHA is the base commit SHA.
	BaseSHA string `json:"baseSha"`

	// Title is the PR title.
	Title string `json:"title"`

	// Description is the PR body.
	Description string `json:"description"`

	// Author is the PR author's username.
	Author string `json:"author"`

	// State is the PR state (open, closed, merged).
	State string `json:"state"`

	// CreatedAt is when the PR was created.
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is when the PR was last updated.
	UpdatedAt time.Time `json:"updatedAt"`
}

// FullName returns the owner/repo format.
func (p PRMetadata) FullName() string {
	return fmt.Sprintf("%s/%s", p.Owner, p.Repo)
}

// IsOpen returns true if the PR is open.
func (p PRMetadata) IsOpen() bool {
	return p.State == "open"
}

// Suggestion represents a structured code suggestion for str_replace.
type Suggestion struct {
	// File is the path to the file to modify.
	File string `json:"file"`

	// OldCode is the original code to be replaced.
	OldCode string `json:"oldCode"`

	// NewCode is the suggested replacement code.
	NewCode string `json:"newCode"`

	// Explanation describes why this change is suggested.
	Explanation string `json:"explanation,omitempty"`

	// Source indicates where this suggestion came from.
	Source string `json:"source"` // "annotation" or "comment"
}

// IsApplicable returns true if the suggestion has valid old and new code.
func (s Suggestion) IsApplicable() bool {
	return s.OldCode != "" && s.NewCode != "" && s.OldCode != s.NewCode
}
