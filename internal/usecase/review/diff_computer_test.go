package review

import (
	"context"
	"errors"
	"testing"

	"github.com/delightfulhammers/bop/internal/domain"
)

// mockGitEngine is a test double for GitEngine.
type mockGitEngine struct {
	cumulativeDiff    domain.Diff
	cumulativeDiffErr error

	// Call counters for verification
	cumulativeDiffCalls int
}

func (m *mockGitEngine) GetCumulativeDiff(ctx context.Context, baseRef, targetRef string, includeUncommitted bool) (domain.Diff, error) {
	m.cumulativeDiffCalls++
	return m.cumulativeDiff, m.cumulativeDiffErr
}

func (m *mockGitEngine) GetIncrementalDiff(ctx context.Context, fromCommit, toCommit string) (domain.Diff, error) {
	// Not used by simplified DiffComputer
	return domain.Diff{}, nil
}

func (m *mockGitEngine) CommitExists(ctx context.Context, commitSHA string) (bool, error) {
	// Not used by simplified DiffComputer
	return true, nil
}

func (m *mockGitEngine) CurrentBranch(ctx context.Context) (string, error) {
	return "main", nil
}

func TestDiffComputer_ComputeDiffForReview_ReturnsFullDiff(t *testing.T) {
	ctx := context.Background()
	expectedDiff := domain.Diff{
		FromCommitHash: "base123",
		ToCommitHash:   "head456",
		Files: []domain.FileDiff{
			{Path: "main.go", Status: domain.FileStatusModified},
		},
	}

	git := &mockGitEngine{cumulativeDiff: expectedDiff}
	computer := NewDiffComputer(git)

	req := BranchRequest{
		BaseRef:   "main",
		TargetRef: "feature",
		CommitSHA: "head456",
	}

	diff, err := computer.ComputeDiffForReview(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diff.ToCommitHash != expectedDiff.ToCommitHash {
		t.Errorf("ToCommitHash = %s, want %s", diff.ToCommitHash, expectedDiff.ToCommitHash)
	}
	if len(diff.Files) != 1 {
		t.Errorf("Files count = %d, want 1", len(diff.Files))
	}
	if git.cumulativeDiffCalls != 1 {
		t.Errorf("GetCumulativeDiff called %d times, want 1", git.cumulativeDiffCalls)
	}
}

func TestDiffComputer_ComputeDiffForReview_PropagatesError(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("git error")

	git := &mockGitEngine{cumulativeDiffErr: expectedErr}
	computer := NewDiffComputer(git)

	req := BranchRequest{
		BaseRef:   "main",
		TargetRef: "feature",
		CommitSHA: "head456",
	}

	_, err := computer.ComputeDiffForReview(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("error = %v, want %v", err, expectedErr)
	}
}

func TestDiffComputer_ComputeDiffForReview_WithUncommittedChanges(t *testing.T) {
	ctx := context.Background()
	expectedDiff := domain.Diff{
		FromCommitHash: "base123",
		ToCommitHash:   "head456",
		Files: []domain.FileDiff{
			{Path: "main.go", Status: domain.FileStatusModified},
			{Path: "uncommitted.go", Status: domain.FileStatusAdded},
		},
	}

	git := &mockGitEngine{cumulativeDiff: expectedDiff}
	computer := NewDiffComputer(git)

	req := BranchRequest{
		BaseRef:            "main",
		TargetRef:          "feature",
		CommitSHA:          "head456",
		IncludeUncommitted: true,
	}

	diff, err := computer.ComputeDiffForReview(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify diff is returned correctly - the mock doesn't differentiate
	// based on IncludeUncommitted, but the real implementation does
	if len(diff.Files) != 2 {
		t.Errorf("Files count = %d, want 2", len(diff.Files))
	}
}
