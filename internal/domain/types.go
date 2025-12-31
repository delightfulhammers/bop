package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const (
	FileStatusAdded    = "added"
	FileStatusModified = "modified"
	FileStatusDeleted  = "deleted"
	FileStatusRenamed  = "renamed"
)

// Diff represents a cumulative diff between two refs.
type Diff struct {
	FromCommitHash string
	ToCommitHash   string
	Files          []FileDiff
}

// FileDiff captures the change for a single file.
type FileDiff struct {
	Path     string
	OldPath  string // Set when Status == FileStatusRenamed
	Status   string
	Patch    string
	IsBinary bool // True for binary files (patch contains "Binary files differ")
}

// Review is the output from an LLM provider.
type Review struct {
	ProviderName string    `json:"providerName"`
	ModelName    string    `json:"modelName"`
	Summary      string    `json:"summary"`
	Findings     []Finding `json:"findings"`

	// Usage metadata from LLM API calls
	TokensIn  int     `json:"tokensIn"`  // Input tokens consumed
	TokensOut int     `json:"tokensOut"` // Output tokens generated
	Cost      float64 `json:"cost"`      // Cost in USD

	// Agent verification fields (Epic #92 - agent-based verification)
	// When agent verification is enabled:
	//   - DiscoveryFindings: raw candidates from LLM discovery (may include duplicates)
	//   - VerifiedFindings: findings confirmed by agent verification
	//   - ReportableFindings: verified findings meeting confidence thresholds
	// When disabled, only Findings is populated (legacy behavior).
	DiscoveryFindings  []CandidateFinding `json:"discoveryFindings,omitempty"`
	VerifiedFindings   []VerifiedFinding  `json:"verifiedFindings,omitempty"`
	ReportableFindings []VerifiedFinding  `json:"reportableFindings,omitempty"`

	// Size guard fields (Epic #7 - PR size guards)
	// When the PR exceeds token limits, files may be truncated to fit.
	// These fields capture what was truncated and warn users about incomplete reviews.
	SizeLimitExceeded bool     `json:"sizeLimitExceeded,omitempty"` // True if prompt exceeded warn threshold
	WasTruncated      bool     `json:"wasTruncated,omitempty"`      // True if files were removed to fit
	TruncatedFiles    []string `json:"truncatedFiles,omitempty"`    // List of files removed for size
	TruncationWarning string   `json:"truncationWarning,omitempty"` // User-friendly warning message
}

// Finding represents a single issue detected by an LLM.
type Finding struct {
	ID          string `json:"id"`
	File        string `json:"file"`
	LineStart   int    `json:"lineStart"`
	LineEnd     int    `json:"lineEnd"`
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion"`
	Evidence    bool   `json:"evidence"`

	// Reviewer attribution (Phase 3.2 - Reviewer Personas)
	// These fields are NOT included in the finding hash/ID to allow
	// the same finding from different reviewers to merge together.
	ReviewerName   string  `json:"reviewerName,omitempty"`
	ReviewerWeight float64 `json:"reviewerWeight,omitempty"`
}

// FindingInput captures the information required to create a Finding.
type FindingInput struct {
	File        string
	LineStart   int
	LineEnd     int
	Severity    string
	Category    string
	Description string
	Suggestion  string
	Evidence    bool
}

// NewFinding constructs a Finding with a deterministic ID.
func NewFinding(input FindingInput) Finding {
	id := hashFinding(input)
	return Finding{
		ID:          id,
		File:        input.File,
		LineStart:   input.LineStart,
		LineEnd:     input.LineEnd,
		Severity:    input.Severity,
		Category:    input.Category,
		Description: input.Description,
		Suggestion:  input.Suggestion,
		Evidence:    input.Evidence,
	}
}

func hashFinding(input FindingInput) string {
	payload := fmt.Sprintf("%s|%d|%d|%s|%s|%s|%t",
		input.File,
		input.LineStart,
		input.LineEnd,
		input.Severity,
		input.Category,
		input.Description,
		input.Evidence,
	)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// FindingFingerprint uniquely identifies a finding across reviews.
// It's stable across line number changes if the code intent remains the same.
type FindingFingerprint string

// NewFindingFingerprint creates a stable identifier for a finding.
// Uses file path + category + severity + normalized description prefix.
// Line numbers are intentionally excluded so the fingerprint remains stable
// when code shifts due to unrelated changes.
func NewFindingFingerprint(file, category, severity, description string) FindingFingerprint {
	// Use first 100 characters of description to allow minor wording changes.
	// Use rune slicing to avoid splitting multi-byte UTF-8 characters.
	descRunes := []rune(description)
	descPrefix := description
	if len(descRunes) > 100 {
		descPrefix = string(descRunes[:100])
	}

	payload := fmt.Sprintf("%s|%s|%s|%s", file, category, severity, descPrefix)
	sum := sha256.Sum256([]byte(payload))
	return FindingFingerprint(hex.EncodeToString(sum[:16])) // 32 hex chars
}

// FingerprintFromFinding creates a fingerprint from an existing Finding.
func FingerprintFromFinding(f Finding) FindingFingerprint {
	return NewFindingFingerprint(f.File, f.Category, f.Severity, f.Description)
}

// Fingerprint returns a stable identifier for this finding.
// The fingerprint is based on file, category, severity, and description prefix.
// Line numbers are intentionally excluded so the fingerprint remains stable
// when code shifts due to unrelated changes.
func (f Finding) Fingerprint() FindingFingerprint {
	return FingerprintFromFinding(f)
}

// MarkdownArtifact encapsulates the Markdown generation inputs.
type MarkdownArtifact struct {
	OutputDir    string
	Repository   string
	BaseRef      string
	TargetRef    string
	Diff         Diff
	Review       Review
	ProviderName string
}

// JSONArtifact encapsulates the JSON generation inputs.
type JSONArtifact struct {
	OutputDir    string
	Repository   string
	BaseRef      string
	TargetRef    string
	Review       Review
	ProviderName string
}
