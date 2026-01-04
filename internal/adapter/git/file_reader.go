package git

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	goGit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/delightfulhammers/bop/internal/diff"
	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/triage"
)

// maxFileSize is the maximum file size that can be read (10 MB).
// Files exceeding this limit return ErrFileTruncated.
const maxFileSize = 10 * 1024 * 1024

// ReadFile retrieves the content of a file at a specific ref.
// The ref can be a commit SHA, branch name, or tag.
// Returns ErrFileNotFound if the file doesn't exist at that ref.
// Returns ErrFileTruncated if the file exceeds maxFileSize (10 MB).
//
// Performance note: Each call reads from disk. For high-concurrency scenarios,
// consider implementing request coalescing or caching at a higher layer.
func (e *Engine) ReadFile(ctx context.Context, path, ref string) (string, error) {
	// Defensive nil check - callers should pass context.Background() instead of nil
	if ctx == nil {
		return "", errors.New("context cannot be nil")
	}

	// Check context cancellation
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	repo, err := goGit.PlainOpenWithOptions(e.repoDir, &goGit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return "", fmt.Errorf("open repo: %w", err)
	}

	commit, err := resolveCommit(repo, ref)
	if err != nil {
		return "", fmt.Errorf("resolve ref %s: %w", ref, err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return "", fmt.Errorf("get tree: %w", err)
	}

	file, err := tree.File(path)
	if err != nil {
		if errors.Is(err, object.ErrFileNotFound) {
			return "", triage.ErrFileNotFound
		}
		return "", fmt.Errorf("get file %s: %w", path, err)
	}

	// Check file size before reading to avoid allocating memory for oversized files.
	// file.Size is from the embedded Blob and is available without reading content.
	if file.Size > maxFileSize {
		return "", triage.ErrFileTruncated
	}

	reader, err := file.Reader()
	if err != nil {
		return "", fmt.Errorf("read file %s: %w", path, err)
	}
	defer func() { _ = reader.Close() }()

	// Read the file content (size already validated above)
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("read content: %w", err)
	}

	return string(content), nil
}

// ReadFileLines retrieves specific lines from a file with optional context.
// startLine and endLine are 1-based and inclusive.
// contextLines specifies how many lines to include before and after.
// Returns a CodeContext with the content and metadata.
//
// Performance note: This reads the entire file into memory to extract lines.
// For files approaching maxFileSize, consider whether the full content is needed.
func (e *Engine) ReadFileLines(ctx context.Context, path, ref string, startLine, endLine, contextLines int) (*domain.CodeContext, error) {
	// Validate line range
	if startLine > endLine {
		return nil, triage.ErrInvalidLineRange
	}
	if startLine < 1 {
		return nil, triage.ErrInvalidLineRange
	}

	// Read full file content
	content, err := e.ReadFile(ctx, path, ref)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	// Remove trailing empty line from split if file ends with newline
	if totalLines > 0 && lines[totalLines-1] == "" {
		totalLines--
		lines = lines[:totalLines]
	}

	// Validate line numbers against actual file length
	// ErrLineOutOfBounds is distinct from ErrInvalidLineRange: the former means
	// the file is shorter than expected (may have changed), the latter means
	// the request itself is malformed (start > end, negative line, etc.)
	if startLine > totalLines || endLine > totalLines {
		return nil, triage.ErrLineOutOfBounds
	}

	// Calculate context bounds
	actualStart := startLine - contextLines
	actualEnd := endLine + contextLines

	// Clamp to file bounds
	if actualStart < 1 {
		actualStart = 1
	}
	if actualEnd > totalLines {
		actualEnd = totalLines
	}

	// Calculate actual context applied
	contextBefore := startLine - actualStart
	contextAfter := actualEnd - endLine

	// Extract lines (convert to 0-indexed)
	selectedLines := lines[actualStart-1 : actualEnd]
	selectedContent := strings.Join(selectedLines, "\n")

	return &domain.CodeContext{
		File:          path,
		Ref:           ref,
		StartLine:     actualStart,
		EndLine:       actualEnd,
		Content:       selectedContent,
		ContextBefore: contextBefore,
		ContextAfter:  contextAfter,
	}, nil
}

// GetDiffHunk retrieves the diff hunk for a specific file and line range.
// baseBranch is the base ref (e.g., "main").
// targetRef is the target ref (e.g., PR head SHA).
// Returns the unified diff hunk(s) covering the specified lines.
//
// Performance note: This computes the full diff for the entire PR, then extracts
// the relevant hunks. For PRs with many files, consider caching the diff result
// or using a more targeted approach if only specific files are needed repeatedly.
func (e *Engine) GetDiffHunk(ctx context.Context, baseBranch, targetRef, file string, startLine, endLine int) (*domain.DiffContext, error) {
	// Get the full diff between base and target
	diff, err := e.GetCumulativeDiff(ctx, baseBranch, targetRef, false)
	if err != nil {
		return nil, fmt.Errorf("get diff: %w", err)
	}

	// Find the file in the diff
	var fileDiff *domain.FileDiff
	for i := range diff.Files {
		if diff.Files[i].Path == file {
			fileDiff = &diff.Files[i]
			break
		}
	}

	if fileDiff == nil {
		return nil, fmt.Errorf("file %s not found in diff", file)
	}

	// Extract relevant hunks that cover the requested line range
	hunkContent := extractRelevantHunks(fileDiff.Patch, startLine, endLine)

	return &domain.DiffContext{
		File:        file,
		BaseBranch:  baseBranch,
		TargetRef:   targetRef,
		HunkContent: hunkContent,
		StartLine:   startLine,
		EndLine:     endLine,
	}, nil
}

// extractRelevantHunks extracts diff hunks that overlap with the given line range.
// It uses the robust diff parser from internal/diff to handle unified diff format correctly.
func extractRelevantHunks(patch string, startLine, endLine int) string {
	if patch == "" {
		return ""
	}

	// Parse the patch using the robust diff parser
	parsed, err := diff.Parse(patch)
	if err != nil || len(parsed.Hunks) == 0 {
		return ""
	}

	// Build a map of hunk start positions to determine which raw lines belong to which hunk
	lines := strings.Split(patch, "\n")
	var result strings.Builder
	inRelevantHunk := false

	for _, line := range lines {
		// Check for hunk header
		if strings.HasPrefix(line, "@@") {
			// Find the matching parsed hunk for this header
			inRelevantHunk = false
			for _, hunk := range parsed.Hunks {
				// Check if this header matches the hunk's new-side range
				if hunkOverlapsRange(hunk, startLine, endLine) {
					// Verify this is the right hunk by checking if line contains the expected range
					expectedStart := fmt.Sprintf("+%d", hunk.NewStart)
					if strings.Contains(line, expectedStart) {
						inRelevantHunk = true
						break
					}
				}
			}
		}

		if inRelevantHunk {
			result.WriteString(line)
			result.WriteString("\n")
		}
	}

	return strings.TrimSuffix(result.String(), "\n")
}

// hunkOverlapsRange checks if a parsed hunk overlaps with the target line range.
// Handles the newCount==0 case correctly (empty range that doesn't overlap anything).
func hunkOverlapsRange(hunk diff.Hunk, targetStart, targetEnd int) bool {
	// If NewLines is 0, the hunk has no new-side lines (pure deletion) - no overlap possible
	if hunk.NewLines == 0 {
		return false
	}

	hunkStart := hunk.NewStart
	hunkEnd := hunk.NewStart + hunk.NewLines - 1

	// Check for overlap: ranges overlap if one starts before the other ends
	return hunkStart <= targetEnd && hunkEnd >= targetStart
}
