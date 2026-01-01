package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	goGit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	formatdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/bkyoung/code-reviewer/internal/domain"
)

// Engine implements the GitEngine port backed by go-git.
type Engine struct {
	repoDir string
}

// NewEngine constructs a Git engine for the provided repository directory.
func NewEngine(repoDir string) *Engine {
	return &Engine{repoDir: repoDir}
}

// GetCumulativeDiff creates a diff between the supplied refs.
func (e *Engine) GetCumulativeDiff(ctx context.Context, baseRef, targetRef string, includeUncommitted bool) (domain.Diff, error) {
	repo, err := goGit.PlainOpenWithOptions(e.repoDir, &goGit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return domain.Diff{}, fmt.Errorf("open repo: %w", err)
	}

	baseCommit, err := resolveCommit(repo, baseRef)
	if err != nil {
		return domain.Diff{}, fmt.Errorf("resolve base ref: %w", err)
	}

	targetCommit, err := resolveCommit(repo, targetRef)
	if err != nil {
		return domain.Diff{}, fmt.Errorf("resolve target ref: %w", err)
	}

	if includeUncommitted {
		fileDiffs, err := diffWithWorkingTree(ctx, e.repoDir, baseRef)
		if err != nil {
			return domain.Diff{}, err
		}
		return domain.Diff{
			FromCommitHash: baseCommit.Hash.String(),
			ToCommitHash:   targetCommit.Hash.String(),
			Files:          fileDiffs,
		}, nil
	}

	patch, err := baseCommit.Patch(targetCommit)
	if err != nil {
		return domain.Diff{}, fmt.Errorf("compute patch: %w", err)
	}

	fileDiffs, err := patchToFileDiffs(patch)
	if err != nil {
		return domain.Diff{}, fmt.Errorf("convert patch to file diffs: %w", err)
	}

	return domain.Diff{
		FromCommitHash: baseCommit.Hash.String(),
		ToCommitHash:   targetCommit.Hash.String(),
		Files:          fileDiffs,
	}, nil
}

// CurrentBranch returns the name of the checked-out branch.
func (e *Engine) CurrentBranch(ctx context.Context) (string, error) {
	repo, err := goGit.PlainOpenWithOptions(e.repoDir, &goGit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return "", fmt.Errorf("open repo: %w", err)
	}
	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("resolve HEAD: %w", err)
	}
	name := head.Name()
	if name.IsBranch() {
		return name.Short(), nil
	}
	return "", fmt.Errorf("detached HEAD")
}

// GetRemoteURL returns the URL of the "origin" remote.
// Returns an empty string and nil error if no origin remote exists.
// Returns an error for other failures (e.g., repository corruption).
func (e *Engine) GetRemoteURL(ctx context.Context) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	repo, err := goGit.PlainOpenWithOptions(e.repoDir, &goGit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return "", fmt.Errorf("open repo: %w", err)
	}

	remote, err := repo.Remote("origin")
	if err != nil {
		// Only treat ErrRemoteNotFound as "no remote configured"
		// Other errors (corruption, etc.) should be surfaced
		if err == goGit.ErrRemoteNotFound {
			return "", nil
		}
		return "", fmt.Errorf("get remote: %w", err)
	}

	config := remote.Config()
	if len(config.URLs) == 0 {
		return "", nil
	}

	return config.URLs[0], nil
}

// BranchExistsLocal checks if a branch exists locally.
func (e *Engine) BranchExistsLocal(ctx context.Context, branch string) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	repo, err := goGit.PlainOpenWithOptions(e.repoDir, &goGit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return false, fmt.Errorf("open repo: %w", err)
	}

	branchRef := plumbing.NewBranchReferenceName(branch)
	_, err = repo.Reference(branchRef, true)
	if err != nil {
		if err == plumbing.ErrReferenceNotFound {
			return false, nil
		}
		return false, fmt.Errorf("check branch %s: %w", branch, err)
	}

	return true, nil
}

// BranchExistsRemote checks if a branch exists on the origin remote.
// This requires network access and may be slow.
func (e *Engine) BranchExistsRemote(ctx context.Context, branch string) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	// Use git ls-remote with full ref path for exact matching.
	// Using just the branch name would do partial matching (e.g., "main" matches "main-v2").
	// The refs/heads/ prefix ensures we match only the exact branch.
	fullRef := "refs/heads/" + branch
	out, err := runGitCommand(ctx, e.repoDir, "ls-remote", "--heads", "origin", fullRef)
	if err != nil {
		// Network error or no remote - return false with error
		return false, err
	}

	// If output is non-empty, the branch exists
	return strings.TrimSpace(out) != "", nil
}

// GetIncrementalDiff creates a diff between two specific commits.
// This is used for incremental reviews where we only want changes since the last reviewed commit.
// Note: go-git operations don't support context cancellation internally, so cancellation
// is best-effort (checked at start only).
func (e *Engine) GetIncrementalDiff(ctx context.Context, fromCommit, toCommit string) (domain.Diff, error) {
	// Check context cancellation first
	if ctx.Err() != nil {
		return domain.Diff{}, ctx.Err()
	}

	repo, err := goGit.PlainOpenWithOptions(e.repoDir, &goGit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return domain.Diff{}, fmt.Errorf("open repo: %w", err)
	}

	fromHash := plumbing.NewHash(fromCommit)
	fromObj, err := repo.CommitObject(fromHash)
	if err != nil {
		return domain.Diff{}, fmt.Errorf("resolve from commit %s: %w", fromCommit, err)
	}

	toHash := plumbing.NewHash(toCommit)
	toObj, err := repo.CommitObject(toHash)
	if err != nil {
		return domain.Diff{}, fmt.Errorf("resolve to commit %s: %w", toCommit, err)
	}

	patch, err := fromObj.Patch(toObj)
	if err != nil {
		return domain.Diff{}, fmt.Errorf("compute patch: %w", err)
	}

	fileDiffs, err := patchToFileDiffs(patch)
	if err != nil {
		return domain.Diff{}, fmt.Errorf("convert patch to file diffs: %w", err)
	}

	return domain.Diff{
		FromCommitHash: fromCommit,
		ToCommitHash:   toCommit,
		Files:          fileDiffs,
	}, nil
}

// CommitExists checks if a commit SHA exists in the repository.
// Used for force-push detection - if the previously reviewed commit no longer exists,
// we should fall back to a full diff.
// Returns (false, nil) if the commit genuinely doesn't exist.
// Returns (false, error) if there was an error checking (e.g., repo access failure, context cancelled).
func (e *Engine) CommitExists(ctx context.Context, commitSHA string) (bool, error) {
	// Check context cancellation first
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	repo, err := goGit.PlainOpenWithOptions(e.repoDir, &goGit.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return false, fmt.Errorf("open repo: %w", err)
	}

	hash := plumbing.NewHash(commitSHA)
	_, err = repo.CommitObject(hash)
	if err != nil {
		if err == plumbing.ErrObjectNotFound {
			return false, nil // Commit genuinely doesn't exist
		}
		return false, fmt.Errorf("check commit %s: %w", commitSHA, err)
	}
	return true, nil
}

func resolveCommit(repo *goGit.Repository, ref string) (*object.Commit, error) {
	candidates := []string{
		ref,
		fmt.Sprintf("refs/heads/%s", ref),
		fmt.Sprintf("refs/remotes/origin/%s", ref),
	}

	var lastErr error
	for _, candidate := range candidates {
		name := plumbing.Revision(candidate)
		hash, err := repo.ResolveRevision(name)
		if err != nil {
			lastErr = err
			continue
		}
		return repo.CommitObject(*hash)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("unable to resolve ref %s", ref)
}

// patchToFileDiffs converts a go-git patch into a slice of domain FileDiffs.
// This is shared between GetCumulativeDiff and GetIncrementalDiff.
func patchToFileDiffs(patch *object.Patch) ([]domain.FileDiff, error) {
	fileDiffs := make([]domain.FileDiff, 0, len(patch.FilePatches()))
	for _, fp := range patch.FilePatches() {
		path, oldPath, status := diffPathAndStatus(fp)
		patchText, err := encodeFilePatch(fp)
		if err != nil {
			return nil, fmt.Errorf("encode patch: %w", err)
		}
		fileDiffs = append(fileDiffs, domain.FileDiff{
			Path:     path,
			OldPath:  oldPath,
			Status:   status,
			Patch:    patchText,
			IsBinary: IsBinaryPatch(patchText),
		})
	}
	return fileDiffs, nil
}

// diffPathAndStatus returns the path, old path (for renames), and status for a file patch.
// For renamed files, path is the new path and oldPath is the previous path.
// For non-renames, oldPath is empty.
func diffPathAndStatus(fp formatdiff.FilePatch) (path, oldPath, status string) {
	from, to := fp.Files()

	switch {
	case from == nil && to != nil:
		return to.Path(), "", domain.FileStatusAdded
	case from != nil && to == nil:
		return from.Path(), "", domain.FileStatusDeleted
	case from != nil && to != nil:
		// Check if this is a rename (different paths)
		if from.Path() != to.Path() {
			return to.Path(), from.Path(), domain.FileStatusRenamed
		}
		return to.Path(), "", domain.FileStatusModified
	default:
		return "", "", domain.FileStatusModified
	}
}

// IsBinaryPatch checks if a patch represents a binary file.
// Git outputs binary markers at the start of a line:
//   - "Binary files a/... and b/... differ"
//   - "GIT binary patch"
//
// We check line-by-line to avoid false positives from code containing these strings.
func IsBinaryPatch(patchText string) bool {
	for _, line := range strings.Split(patchText, "\n") {
		if strings.HasPrefix(line, "Binary files ") ||
			strings.HasPrefix(line, "GIT binary patch") {
			return true
		}
	}
	return false
}

func diffWithWorkingTree(ctx context.Context, repoDir, baseRef string) ([]domain.FileDiff, error) {
	statusOut, err := runGitCommand(ctx, repoDir, "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}

	trimmed := strings.TrimRight(statusOut, "\r\n")
	if trimmed == "" {
		return []domain.FileDiff{}, nil
	}
	lines := strings.Split(trimmed, "\n")
	diffs := make([]domain.FileDiff, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		if len(line) < 3 {
			continue
		}
		statusChar := selectStatusChar(line)
		path, oldPath := ExtractPathAndOldPath(line)
		patchOut, err := runGitCommand(ctx, repoDir, "diff", baseRef, "--", path)
		if err != nil {
			return nil, fmt.Errorf("git diff %s: %w", path, err)
		}
		diffs = append(diffs, domain.FileDiff{
			Path:     path,
			OldPath:  oldPath,
			Status:   MapGitStatus(statusChar),
			Patch:    patchOut,
			IsBinary: IsBinaryPatch(patchOut),
		})
	}
	return diffs, nil
}

func runGitCommand(ctx context.Context, repoDir string, args ...string) (string, error) {
	fullArgs := append([]string{"-C", repoDir}, args...)
	cmd := exec.CommandContext(ctx, "git", fullArgs...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("git %v: %w", args, ctx.Err())
		}
		if stderr.Len() > 0 {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("git %v: %w", args, err)
	}
	return stdout.String(), nil
}

func selectStatusChar(line string) rune {
	if len(line) < 2 {
		return 'M'
	}
	first := rune(line[0])
	second := rune(line[1])
	switch {
	case second != ' ':
		return second
	case first != ' ':
		return first
	default:
		return 'M'
	}
}

// ExtractPathAndOldPath extracts both the current path and old path (for renames) from a git status line.
// For renames, git status shows "R  old_path -> new_path".
// Returns (newPath, oldPath) where oldPath is empty for non-renames.
func ExtractPathAndOldPath(line string) (path, oldPath string) {
	if len(line) <= 3 {
		return strings.TrimSpace(line), ""
	}
	pathPart := strings.TrimSpace(line[3:])
	if strings.Contains(pathPart, " -> ") {
		parts := strings.Split(pathPart, " -> ")
		if len(parts) == 2 {
			return strings.TrimSpace(parts[1]), strings.TrimSpace(parts[0])
		}
	}
	return pathPart, ""
}

// MapGitStatus converts a git status character to a domain file status.
func MapGitStatus(status rune) string {
	switch status {
	case 'A', '?':
		return domain.FileStatusAdded
	case 'D':
		return domain.FileStatusDeleted
	case 'R':
		return domain.FileStatusRenamed
	default:
		return domain.FileStatusModified
	}
}

func encodeFilePatch(fp formatdiff.FilePatch) (string, error) {
	var buf bytes.Buffer
	encoder := formatdiff.NewUnifiedEncoder(&buf, formatdiff.DefaultContextLines)
	if err := encoder.Encode(singlePatch{fp: fp}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type singlePatch struct {
	fp formatdiff.FilePatch
}

func (s singlePatch) FilePatches() []formatdiff.FilePatch {
	return []formatdiff.FilePatch{s.fp}
}

func (s singlePatch) Message() string {
	return ""
}
