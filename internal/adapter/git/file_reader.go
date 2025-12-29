package git

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	goGit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/bkyoung/code-reviewer/internal/usecase/triage"
)

// maxFileSize is the maximum file size that can be read (10 MB).
const maxFileSize = 10 * 1024 * 1024

// ReadFile retrieves the content of a file at a specific ref.
// The ref can be a commit SHA, branch name, or tag.
// Returns ErrFileNotFound if the file doesn't exist at that ref.
func (e *Engine) ReadFile(ctx context.Context, path, ref string) (string, error) {
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

	reader, err := file.Reader()
	if err != nil {
		return "", fmt.Errorf("read file %s: %w", path, err)
	}
	defer func() { _ = reader.Close() }()

	// Limit file size to prevent OOM on extremely large files
	content, err := io.ReadAll(io.LimitReader(reader, maxFileSize))
	if err != nil {
		return "", fmt.Errorf("read content: %w", err)
	}

	return string(content), nil
}

// ReadFileLines retrieves specific lines from a file with optional context.
// startLine and endLine are 1-based and inclusive.
// contextLines specifies how many lines to include before and after.
// Returns a CodeContext with the content and metadata.
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

	// Validate line numbers against actual file
	if startLine > totalLines {
		return nil, triage.ErrInvalidLineRange
	}
	if endLine > totalLines {
		return nil, triage.ErrInvalidLineRange
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
// It parses unified diff format and returns hunks that include the specified lines.
func extractRelevantHunks(patch string, startLine, endLine int) string {
	if patch == "" {
		return ""
	}

	lines := strings.Split(patch, "\n")
	var result strings.Builder
	inRelevantHunk := false
	currentHunkStart := 0
	currentHunkEnd := 0

	for _, line := range lines {
		// Parse hunk header: @@ -old_start,old_count +new_start,new_count @@
		if strings.HasPrefix(line, "@@") {
			// Extract new file line range from hunk header
			newStart, newCount := parseHunkHeader(line)
			currentHunkStart = newStart
			currentHunkEnd = newStart + newCount - 1
			if newCount == 0 {
				currentHunkEnd = newStart
			}

			// Check if this hunk overlaps with our target range
			inRelevantHunk = hunksOverlap(currentHunkStart, currentHunkEnd, startLine, endLine)
		}

		if inRelevantHunk {
			result.WriteString(line)
			result.WriteString("\n")
		}
	}

	return strings.TrimSuffix(result.String(), "\n")
}

// parseHunkHeader extracts new file start line and count from a unified diff hunk header.
// Format: @@ -old_start,old_count +new_start,new_count @@ optional context
func parseHunkHeader(header string) (start, count int) {
	// Default values
	start, count = 1, 1

	// Find the +new_start,new_count part
	plusIdx := strings.Index(header, "+")
	if plusIdx == -1 {
		return
	}

	// Find the end of the range (space or @)
	endIdx := strings.Index(header[plusIdx:], " ")
	if endIdx == -1 {
		endIdx = strings.Index(header[plusIdx:], "@")
	}
	if endIdx == -1 {
		return
	}

	rangeStr := header[plusIdx+1 : plusIdx+endIdx]

	// Parse "start,count" or just "start"
	if commaIdx := strings.Index(rangeStr, ","); commaIdx != -1 {
		_, _ = fmt.Sscanf(rangeStr, "%d,%d", &start, &count)
	} else {
		_, _ = fmt.Sscanf(rangeStr, "%d", &start)
		count = 1
	}

	return
}

// hunksOverlap returns true if two line ranges overlap.
func hunksOverlap(hunkStart, hunkEnd, targetStart, targetEnd int) bool {
	return hunkStart <= targetEnd && hunkEnd >= targetStart
}
