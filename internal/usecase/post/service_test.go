package post_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/post"
	"github.com/delightfulhammers/bop/internal/usecase/review"
)

// =============================================================================
// Mock Implementations
// =============================================================================

type mockPRClient struct {
	metadata    *domain.PRMetadata
	metadataErr error
	diff        domain.Diff
	diffErr     error
}

func (m *mockPRClient) GetPRMetadata(ctx context.Context, owner, repo string, prNumber int) (*domain.PRMetadata, error) {
	if m.metadataErr != nil {
		return nil, m.metadataErr
	}
	return m.metadata, nil
}

func (m *mockPRClient) GetPRDiff(ctx context.Context, owner, repo string, prNumber int) (domain.Diff, error) {
	if m.diffErr != nil {
		return domain.Diff{}, m.diffErr
	}
	return m.diff, nil
}

type mockPoster struct {
	result    *review.GitHubPostResult
	postErr   error
	lastReq   *review.GitHubPostRequest
	callCount int
}

func (m *mockPoster) PostReview(ctx context.Context, req review.GitHubPostRequest) (*review.GitHubPostResult, error) {
	m.callCount++
	m.lastReq = &req
	if m.postErr != nil {
		return nil, m.postErr
	}
	return m.result, nil
}

// =============================================================================
// Service Tests
// =============================================================================

func TestService_PostFindings_Success(t *testing.T) {
	prClient := &mockPRClient{
		metadata: &domain.PRMetadata{
			HeadSHA: "abc123",
			Title:   "Test PR",
		},
		diff: domain.Diff{
			Files: []domain.FileDiff{
				{Path: "main.go", Status: "modified", Patch: "@@ -10,5 +10,5 @@"},
			},
		},
	}

	poster := &mockPoster{
		result: &review.GitHubPostResult{
			ReviewID:       12345,
			CommentsPosted: 2,
			HTMLURL:        "https://github.com/owner/repo/pull/1#pullrequestreview-12345",
		},
	}

	svc := post.NewService(prClient, poster)

	result, err := svc.PostFindings(context.Background(), post.Request{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 1,
		Findings: []domain.Finding{
			{File: "main.go", LineStart: 10, LineEnd: 12, Severity: "high", Category: "bug", Description: "Test finding 1"},
			{File: "main.go", LineStart: 15, LineEnd: 15, Severity: "medium", Category: "style", Description: "Test finding 2"},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, int64(12345), result.ReviewID)
	assert.Equal(t, 2, result.Posted)
	assert.Equal(t, "https://github.com/owner/repo/pull/1#pullrequestreview-12345", result.ReviewURL)

	// Verify poster was called with correct request
	require.NotNil(t, poster.lastReq)
	assert.Equal(t, "owner", poster.lastReq.Owner)
	assert.Equal(t, "repo", poster.lastReq.Repo)
	assert.Equal(t, 1, poster.lastReq.PRNumber)
	assert.Equal(t, "abc123", poster.lastReq.CommitSHA)
	assert.Len(t, poster.lastReq.Review.Findings, 2)
}

func TestService_PostFindings_DryRun(t *testing.T) {
	prClient := &mockPRClient{
		metadata: &domain.PRMetadata{HeadSHA: "abc123"},
		diff:     domain.Diff{},
	}

	poster := &mockPoster{}

	svc := post.NewService(prClient, poster)

	result, err := svc.PostFindings(context.Background(), post.Request{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 1,
		Findings: []domain.Finding{
			{File: "main.go", LineStart: 10, Severity: "high", Category: "bug", Description: "Test"},
		},
		DryRun: true,
	})

	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.Equal(t, 1, result.WouldPost)
	assert.Equal(t, 0, poster.callCount, "poster should not be called in dry-run mode")
}

func TestService_PostFindings_EmptyFindings(t *testing.T) {
	prClient := &mockPRClient{
		metadata: &domain.PRMetadata{HeadSHA: "abc123"},
		diff:     domain.Diff{},
	}

	poster := &mockPoster{
		result: &review.GitHubPostResult{
			ReviewID:       12345,
			CommentsPosted: 0,
		},
	}

	svc := post.NewService(prClient, poster)

	result, err := svc.PostFindings(context.Background(), post.Request{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 1,
		Findings: []domain.Finding{},
	})

	require.NoError(t, err)
	assert.Equal(t, 0, result.Posted)
}

func TestService_PostFindings_MetadataError(t *testing.T) {
	prClient := &mockPRClient{
		metadataErr: errors.New("PR not found"),
	}

	poster := &mockPoster{}

	svc := post.NewService(prClient, poster)

	_, err := svc.PostFindings(context.Background(), post.Request{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 999,
		Findings: []domain.Finding{
			{File: "main.go", LineStart: 10, Severity: "high", Category: "bug", Description: "Test"},
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "PR not found")
}

func TestService_PostFindings_DiffError(t *testing.T) {
	prClient := &mockPRClient{
		metadata: &domain.PRMetadata{HeadSHA: "abc123"},
		diffErr:  errors.New("diff too large"),
	}

	poster := &mockPoster{}

	svc := post.NewService(prClient, poster)

	_, err := svc.PostFindings(context.Background(), post.Request{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 1,
		Findings: []domain.Finding{
			{File: "main.go", LineStart: 10, Severity: "high", Category: "bug", Description: "Test"},
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "diff")
}

func TestService_PostFindings_PosterError(t *testing.T) {
	prClient := &mockPRClient{
		metadata: &domain.PRMetadata{HeadSHA: "abc123"},
		diff:     domain.Diff{},
	}

	poster := &mockPoster{
		postErr: errors.New("rate limit exceeded"),
	}

	svc := post.NewService(prClient, poster)

	_, err := svc.PostFindings(context.Background(), post.Request{
		Owner:    "owner",
		Repo:     "repo",
		PRNumber: 1,
		Findings: []domain.Finding{
			{File: "main.go", LineStart: 10, Severity: "high", Category: "bug", Description: "Test"},
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit")
}

func TestService_PostFindings_WithReviewAction(t *testing.T) {
	prClient := &mockPRClient{
		metadata: &domain.PRMetadata{HeadSHA: "abc123"},
		diff:     domain.Diff{},
	}

	poster := &mockPoster{
		result: &review.GitHubPostResult{ReviewID: 12345, CommentsPosted: 1},
	}

	svc := post.NewService(prClient, poster)

	action := "COMMENT"
	_, err := svc.PostFindings(context.Background(), post.Request{
		Owner:        "owner",
		Repo:         "repo",
		PRNumber:     1,
		Findings:     []domain.Finding{{File: "main.go", LineStart: 10, Severity: "high", Category: "bug", Description: "Test"}},
		ReviewAction: &action,
	})

	require.NoError(t, err)

	// Verify the review action was passed through
	require.NotNil(t, poster.lastReq)
	// The action should affect ActionOnHigh/Critical/etc in the request
	// For COMMENT override, all actions should be COMMENT
	assert.Equal(t, "COMMENT", poster.lastReq.ActionOnCritical)
	assert.Equal(t, "COMMENT", poster.lastReq.ActionOnHigh)
}

func TestService_PostFindings_ValidationErrors(t *testing.T) {
	prClient := &mockPRClient{}
	poster := &mockPoster{}
	svc := post.NewService(prClient, poster)

	tests := []struct {
		name    string
		req     post.Request
		wantErr string
	}{
		{
			name:    "empty owner",
			req:     post.Request{Owner: "", Repo: "repo", PRNumber: 1},
			wantErr: "owner is required",
		},
		{
			name:    "empty repo",
			req:     post.Request{Owner: "owner", Repo: "", PRNumber: 1},
			wantErr: "repo is required",
		},
		{
			name:    "zero PR number",
			req:     post.Request{Owner: "owner", Repo: "repo", PRNumber: 0},
			wantErr: "PR number is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.PostFindings(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
