package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/review"
	"github.com/delightfulhammers/bop/internal/usecase/triage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mockPRReviewer is a test mock for PRReviewer interface.
type mockPRReviewer struct {
	result review.Result
	err    error
}

func (m *mockPRReviewer) ReviewPR(ctx context.Context, req review.PRRequest) (review.Result, error) {
	return m.result, m.err
}

// mockFindingPoster is a test mock for FindingPoster interface.
type mockFindingPoster struct {
	result *review.GitHubPostResult
	err    error
}

func (m *mockFindingPoster) PostReview(ctx context.Context, req review.GitHubPostRequest) (*review.GitHubPostResult, error) {
	return m.result, m.err
}

// mockPRMetadataFetcher is a test mock for PRMetadataFetcher interface.
type mockPRMetadataFetcher struct {
	metadata *domain.PRMetadata
	diff     domain.Diff
	metaErr  error
	diffErr  error
}

func (m *mockPRMetadataFetcher) GetPRMetadata(ctx context.Context, owner, repo string, prNumber int) (*domain.PRMetadata, error) {
	return m.metadata, m.metaErr
}

func (m *mockPRMetadataFetcher) GetPRDiff(ctx context.Context, owner, repo string, prNumber int) (domain.Diff, error) {
	return m.diff, m.diffErr
}

// mockCommentReaderForDedup is a minimal mock for triage.CommentReader used in skip_duplicates tests.
type mockCommentReaderForDedup struct {
	findings []domain.PRFinding
	err      error
}

func (m *mockCommentReaderForDedup) ListPRComments(ctx context.Context, owner, repo string, prNumber int, filterByFingerprint bool) ([]domain.PRFinding, error) {
	return m.findings, m.err
}

func (m *mockCommentReaderForDedup) GetPRComment(ctx context.Context, owner, repo string, prNumber int, commentID int64) (*domain.PRFinding, error) {
	return nil, nil
}

func (m *mockCommentReaderForDedup) GetPRCommentByFingerprint(ctx context.Context, owner, repo string, prNumber int, fingerprint string) (*domain.PRFinding, error) {
	return nil, nil
}

func (m *mockCommentReaderForDedup) GetThreadHistory(ctx context.Context, owner, repo string, commentID int64) ([]domain.ThreadComment, error) {
	return nil, nil
}

func (m *mockCommentReaderForDedup) ListAllFindings(ctx context.Context, owner, repo string, prNumber int, filterByFingerprint bool) ([]domain.PRFinding, error) {
	return m.findings, m.err
}

// createPRServiceForDedup creates a PRService with a mock CommentReader for testing skip_duplicates.
func createPRServiceForDedup(findings []domain.PRFinding) *triage.PRService {
	return triage.NewPRService(triage.PRServiceDeps{
		CommentReader: &mockCommentReaderForDedup{findings: findings},
	})
}

func TestHandleEditFinding(t *testing.T) {
	// Create a server with no dependencies (edit_finding is a pure function)
	s := &Server{deps: ServerDeps{}}

	baseFinding := FindingInput{
		ID:          "original-id",
		File:        "main.go",
		LineStart:   10,
		LineEnd:     15,
		Severity:    "medium",
		Category:    "security",
		Description: "Original description",
		Suggestion:  "Original suggestion",
	}

	t.Run("returns finding unchanged when no overrides provided", func(t *testing.T) {
		input := EditFindingInput{
			Finding: baseFinding,
		}

		result, output, err := s.handleEditFinding(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.IsError)

		assert.Equal(t, baseFinding.File, output.Finding.File)
		assert.Equal(t, baseFinding.LineStart, output.Finding.LineStart)
		assert.Equal(t, baseFinding.LineEnd, output.Finding.LineEnd)
		assert.Equal(t, baseFinding.Severity, output.Finding.Severity)
		assert.Equal(t, baseFinding.Category, output.Finding.Category)
		assert.Equal(t, baseFinding.Description, output.Finding.Description)
		assert.Equal(t, baseFinding.Suggestion, output.Finding.Suggestion)
	})

	t.Run("overrides severity when provided", func(t *testing.T) {
		newSeverity := "critical"
		input := EditFindingInput{
			Finding:  baseFinding,
			Severity: &newSeverity,
		}

		result, output, err := s.handleEditFinding(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.IsError)

		assert.Equal(t, "critical", output.Finding.Severity)
		// Other fields unchanged
		assert.Equal(t, baseFinding.File, output.Finding.File)
		assert.Equal(t, baseFinding.Category, output.Finding.Category)
		assert.Equal(t, baseFinding.Description, output.Finding.Description)
	})

	t.Run("overrides category when provided", func(t *testing.T) {
		newCategory := "performance"
		input := EditFindingInput{
			Finding:  baseFinding,
			Category: &newCategory,
		}

		result, output, err := s.handleEditFinding(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.IsError)

		assert.Equal(t, "performance", output.Finding.Category)
	})

	t.Run("overrides description when provided", func(t *testing.T) {
		newDescription := "Updated description with more detail"
		input := EditFindingInput{
			Finding:     baseFinding,
			Description: &newDescription,
		}

		result, output, err := s.handleEditFinding(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.IsError)

		assert.Equal(t, "Updated description with more detail", output.Finding.Description)
	})

	t.Run("overrides suggestion when provided", func(t *testing.T) {
		newSuggestion := "Updated suggestion"
		input := EditFindingInput{
			Finding:    baseFinding,
			Suggestion: &newSuggestion,
		}

		result, output, err := s.handleEditFinding(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.IsError)

		assert.Equal(t, "Updated suggestion", output.Finding.Suggestion)
	})

	t.Run("overrides multiple fields at once", func(t *testing.T) {
		newSeverity := "high"
		newCategory := "bug"
		newDescription := "This is a bug"
		input := EditFindingInput{
			Finding:     baseFinding,
			Severity:    &newSeverity,
			Category:    &newCategory,
			Description: &newDescription,
		}

		result, output, err := s.handleEditFinding(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.IsError)

		assert.Equal(t, "high", output.Finding.Severity)
		assert.Equal(t, "bug", output.Finding.Category)
		assert.Equal(t, "This is a bug", output.Finding.Description)
		// Unchanged fields
		assert.Equal(t, baseFinding.File, output.Finding.File)
		assert.Equal(t, baseFinding.LineStart, output.Finding.LineStart)
		assert.Equal(t, baseFinding.Suggestion, output.Finding.Suggestion)
	})

	t.Run("recalculates fingerprint when fingerprint-relevant field changes", func(t *testing.T) {
		// Get original fingerprint
		originalInput := EditFindingInput{Finding: baseFinding}
		_, originalOutput, _ := s.handleEditFinding(context.Background(), nil, originalInput)
		originalFingerprint := originalOutput.Finding.Fingerprint

		// Change severity (fingerprint-relevant)
		newSeverity := "critical"
		changedInput := EditFindingInput{
			Finding:  baseFinding,
			Severity: &newSeverity,
		}
		_, changedOutput, _ := s.handleEditFinding(context.Background(), nil, changedInput)

		// Fingerprint should be different
		assert.NotEqual(t, originalFingerprint, changedOutput.Finding.Fingerprint)
	})

	t.Run("fingerprint unchanged when only suggestion changes", func(t *testing.T) {
		// Get original fingerprint
		originalInput := EditFindingInput{Finding: baseFinding}
		_, originalOutput, _ := s.handleEditFinding(context.Background(), nil, originalInput)
		originalFingerprint := originalOutput.Finding.Fingerprint

		// Change suggestion (NOT fingerprint-relevant)
		newSuggestion := "Totally different suggestion"
		changedInput := EditFindingInput{
			Finding:    baseFinding,
			Suggestion: &newSuggestion,
		}
		_, changedOutput, _ := s.handleEditFinding(context.Background(), nil, changedInput)

		// Fingerprint should be same (suggestion is not part of fingerprint)
		assert.Equal(t, originalFingerprint, changedOutput.Finding.Fingerprint)
	})

	t.Run("result message indicates what was changed", func(t *testing.T) {
		newSeverity := "low"
		input := EditFindingInput{
			Finding:  baseFinding,
			Severity: &newSeverity,
		}

		result, _, err := s.handleEditFinding(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Check that result has text content
		require.Len(t, result.Content, 1)
		textContent, ok := result.Content[0].(*mcp.TextContent)
		require.True(t, ok)
		assert.Contains(t, textContent.Text, "severity")
	})

	t.Run("validates severity value", func(t *testing.T) {
		invalidSeverity := "extreme" // not a valid severity
		input := EditFindingInput{
			Finding:  baseFinding,
			Severity: &invalidSeverity,
		}

		result, _, err := s.handleEditFinding(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
	})
}

func TestFindingInputToDomain(t *testing.T) {
	input := FindingInput{
		ID:           "test-id",
		File:         "pkg/handler.go",
		LineStart:    100,
		LineEnd:      105,
		Severity:     "high",
		Category:     "security",
		Description:  "SQL injection vulnerability",
		Suggestion:   "Use parameterized queries",
		ReviewerName: "security-bot",
	}

	finding := findingInputToDomain(input)

	assert.Equal(t, input.ID, finding.ID)
	assert.Equal(t, input.File, finding.File)
	assert.Equal(t, input.LineStart, finding.LineStart)
	assert.Equal(t, input.LineEnd, finding.LineEnd)
	assert.Equal(t, input.Severity, finding.Severity)
	assert.Equal(t, input.Category, finding.Category)
	assert.Equal(t, input.Description, finding.Description)
	assert.Equal(t, input.Suggestion, finding.Suggestion)
	assert.Equal(t, input.ReviewerName, finding.ReviewerName)
}

func TestDomainFindingToOutput(t *testing.T) {
	finding := domain.Finding{
		ID:             "abc123",
		File:           "main.go",
		LineStart:      10,
		LineEnd:        20,
		Severity:       "medium",
		Category:       "bug",
		Description:    "Potential nil pointer",
		Suggestion:     "Add nil check",
		ReviewerName:   "arch-reviewer",
		ReviewerWeight: 1.5,
	}

	output := domainFindingToOutput(finding)

	assert.Equal(t, finding.ID, output.ID)
	assert.Equal(t, finding.File, output.File)
	assert.Equal(t, finding.LineStart, output.LineStart)
	assert.Equal(t, finding.LineEnd, output.LineEnd)
	assert.Equal(t, finding.Severity, output.Severity)
	assert.Equal(t, finding.Category, output.Category)
	assert.Equal(t, finding.Description, output.Description)
	assert.Equal(t, finding.Suggestion, output.Suggestion)
	assert.Equal(t, finding.ReviewerName, output.ReviewerName)
	assert.Equal(t, finding.ReviewerWeight, output.ReviewerWeight)
	// Fingerprint should be calculated
	assert.NotEmpty(t, output.Fingerprint)
	assert.Equal(t, string(finding.Fingerprint()), output.Fingerprint)
}

func TestHandleReviewPR(t *testing.T) {
	t.Run("returns not implemented when PRReviewer is nil", func(t *testing.T) {
		s := &Server{deps: ServerDeps{}}

		input := ReviewPRInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
		}

		result, output, err := s.handleReviewPR(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Empty(t, output.Findings)

		// Verify output has properly initialized maps (not nil) for schema validation
		assert.NotNil(t, output.BySeverity, "BySeverity should be initialized, not nil")
		assert.NotNil(t, output.ByCategory, "ByCategory should be initialized, not nil")
		assert.NotNil(t, output.Findings, "Findings should be initialized, not nil")
		assert.NotNil(t, output.ReviewerStats, "ReviewerStats should be initialized, not nil")
	})

	t.Run("validates required inputs", func(t *testing.T) {
		mock := &mockPRReviewer{}
		s := &Server{deps: ServerDeps{PRReviewer: mock}}

		testCases := []struct {
			name  string
			input ReviewPRInput
			want  string
		}{
			{
				name:  "missing owner",
				input: ReviewPRInput{Repo: "repo", PRNumber: 123},
				want:  "owner is required",
			},
			{
				name:  "missing repo",
				input: ReviewPRInput{Owner: "owner", PRNumber: 123},
				want:  "repo is required",
			},
			{
				name:  "invalid PR number",
				input: ReviewPRInput{Owner: "owner", Repo: "repo", PRNumber: 0},
				want:  "invalid PR number",
			},
			{
				name:  "negative PR number",
				input: ReviewPRInput{Owner: "owner", Repo: "repo", PRNumber: -1},
				want:  "invalid PR number",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result, _, err := s.handleReviewPR(context.Background(), nil, tc.input)
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.True(t, result.IsError)
				textContent, ok := result.Content[0].(*mcp.TextContent)
				require.True(t, ok)
				assert.Contains(t, textContent.Text, tc.want)
			})
		}
	})

	t.Run("returns findings from review", func(t *testing.T) {
		mock := &mockPRReviewer{
			result: review.Result{
				Reviews: []domain.Review{
					{
						ProviderName: "anthropic",
						ModelName:    "claude-sonnet-4-5",
						TokensIn:     1000,
						TokensOut:    500,
						Cost:         0.01,
						Findings: []domain.Finding{
							{
								ID:          "f1",
								File:        "main.go",
								LineStart:   10,
								LineEnd:     15,
								Severity:    "high",
								Category:    "security",
								Description: "SQL injection vulnerability",
							},
							{
								ID:          "f2",
								File:        "utils.go",
								LineStart:   20,
								LineEnd:     25,
								Severity:    "medium",
								Category:    "bug",
								Description: "Nil pointer dereference",
							},
						},
					},
				},
			},
		}
		s := &Server{deps: ServerDeps{PRReviewer: mock}}

		input := ReviewPRInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
		}

		result, output, err := s.handleReviewPR(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.IsError)

		assert.Equal(t, 2, output.TotalFindings)
		assert.Len(t, output.Findings, 2)
		assert.Equal(t, 1, output.BySeverity["high"])
		assert.Equal(t, 1, output.BySeverity["medium"])
		assert.Equal(t, 1, output.ByCategory["security"])
		assert.Equal(t, 1, output.ByCategory["bug"])
		assert.Equal(t, 1000, output.TokensIn)
		assert.Equal(t, 500, output.TokensOut)
		assert.Equal(t, 0.01, output.Cost)
	})

	t.Run("aggregates findings from multiple reviews", func(t *testing.T) {
		mock := &mockPRReviewer{
			result: review.Result{
				Reviews: []domain.Review{
					{
						ProviderName: "security",
						TokensIn:     500,
						TokensOut:    200,
						Cost:         0.005,
						Findings: []domain.Finding{
							{ID: "f1", Severity: "high", Category: "security"},
						},
					},
					{
						ProviderName: "architecture",
						TokensIn:     600,
						TokensOut:    300,
						Cost:         0.006,
						Findings: []domain.Finding{
							{ID: "f2", Severity: "medium", Category: "architecture"},
							{ID: "f3", Severity: "low", Category: "maintainability"},
						},
					},
				},
			},
		}
		s := &Server{deps: ServerDeps{PRReviewer: mock}}

		input := ReviewPRInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
		}

		result, output, err := s.handleReviewPR(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 3, output.TotalFindings)
		assert.Equal(t, 1100, output.TokensIn)
		assert.Equal(t, 500, output.TokensOut)
		assert.InDelta(t, 0.011, output.Cost, 0.0001)
	})

	t.Run("handles no findings", func(t *testing.T) {
		mock := &mockPRReviewer{
			result: review.Result{
				Reviews: []domain.Review{
					{ProviderName: "security", Findings: []domain.Finding{}},
				},
			},
		}
		s := &Server{deps: ServerDeps{PRReviewer: mock}}

		input := ReviewPRInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
		}

		result, output, err := s.handleReviewPR(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 0, output.TotalFindings)
		assert.Contains(t, output.Message, "no issues found")
	})

	t.Run("propagates review errors", func(t *testing.T) {
		mock := &mockPRReviewer{
			err: errors.New("GitHub API rate limit exceeded"),
		}
		s := &Server{deps: ServerDeps{PRReviewer: mock}}

		input := ReviewPRInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
		}

		_, _, err := s.handleReviewPR(context.Background(), nil, input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "GitHub API rate limit exceeded")
	})
}

func TestHandlePostFindings(t *testing.T) {
	t.Run("returns not implemented when FindingPoster is nil", func(t *testing.T) {
		s := &Server{deps: ServerDeps{}}

		input := PostFindingsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
			Findings: []FindingInput{{File: "main.go", Severity: "high"}},
		}

		result, output, err := s.handlePostFindings(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.False(t, output.Success)
	})

	t.Run("returns not implemented when RemoteGitHubClient is nil", func(t *testing.T) {
		s := &Server{deps: ServerDeps{
			FindingPoster: &mockFindingPoster{},
		}}

		input := PostFindingsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
			Findings: []FindingInput{{File: "main.go", Severity: "high"}},
		}

		result, output, err := s.handlePostFindings(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.False(t, output.Success)
	})

	t.Run("validates required inputs", func(t *testing.T) {
		s := &Server{deps: ServerDeps{FindingPoster: nil}} // Even with nil, validation runs first

		testCases := []struct {
			name  string
			input PostFindingsInput
			want  string
		}{
			{
				name: "missing owner",
				input: PostFindingsInput{
					Repo:     "repo",
					PRNumber: 123,
					Findings: []FindingInput{{File: "main.go"}},
				},
				want: "owner is required",
			},
			{
				name: "missing repo",
				input: PostFindingsInput{
					Owner:    "owner",
					PRNumber: 123,
					Findings: []FindingInput{{File: "main.go"}},
				},
				want: "repo is required",
			},
			{
				name: "invalid PR number",
				input: PostFindingsInput{
					Owner:    "owner",
					Repo:     "repo",
					PRNumber: 0,
					Findings: []FindingInput{{File: "main.go"}},
				},
				want: "invalid PR number",
			},
			{
				name: "no findings",
				input: PostFindingsInput{
					Owner:    "owner",
					Repo:     "repo",
					PRNumber: 123,
					Findings: []FindingInput{},
				},
				want: "at least one finding is required",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result, _, err := s.handlePostFindings(context.Background(), nil, tc.input)
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.True(t, result.IsError)
				textContent, ok := result.Content[0].(*mcp.TextContent)
				require.True(t, ok)
				assert.Contains(t, textContent.Text, tc.want)
			})
		}
	})

	t.Run("dry run returns what would be posted", func(t *testing.T) {
		s := &Server{deps: ServerDeps{
			FindingPoster:      &mockFindingPoster{},
			RemoteGitHubClient: &mockPRMetadataFetcher{},
		}}

		input := PostFindingsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
			Findings: []FindingInput{
				{File: "main.go", LineStart: 10, Severity: "high", Category: "security", Description: "SQL injection"},
				{File: "utils.go", LineStart: 20, Severity: "medium", Category: "bug", Description: "Nil pointer"},
			},
			DryRun: true,
		}

		result, output, err := s.handlePostFindings(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.IsError)

		assert.True(t, output.Success)
		assert.Equal(t, 2, output.Posted)
		assert.Len(t, output.PostedFingerprints, 2)
		assert.Contains(t, output.Message, "Dry run")
	})

	t.Run("posts findings successfully", func(t *testing.T) {
		mockPoster := &mockFindingPoster{
			result: &review.GitHubPostResult{
				ReviewID:        12345,
				CommentsPosted:  2,
				CommentsSkipped: 0,
			},
		}
		mockFetcher := &mockPRMetadataFetcher{
			metadata: &domain.PRMetadata{
				HeadSHA: "abc123",
				BaseRef: "main",
				HeadRef: "feature",
			},
			diff: domain.Diff{
				Files: []domain.FileDiff{{Path: "main.go"}},
			},
		}
		s := &Server{deps: ServerDeps{
			FindingPoster:      mockPoster,
			RemoteGitHubClient: mockFetcher,
		}}

		input := PostFindingsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
			Findings: []FindingInput{
				{File: "main.go", LineStart: 10, Severity: "high", Category: "security", Description: "Issue 1"},
				{File: "utils.go", LineStart: 20, Severity: "medium", Category: "bug", Description: "Issue 2"},
			},
		}

		result, output, err := s.handlePostFindings(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.IsError)

		assert.True(t, output.Success)
		assert.Equal(t, 2, output.Posted)
		assert.Equal(t, int64(12345), output.ReviewID)
		assert.Equal(t, "REQUEST_CHANGES", output.ReviewAction)
		assert.Len(t, output.PostedFingerprints, 2)
	})

	t.Run("propagates metadata fetch errors", func(t *testing.T) {
		mockFetcher := &mockPRMetadataFetcher{
			metaErr: errors.New("rate limit exceeded"),
		}
		s := &Server{deps: ServerDeps{
			FindingPoster:      &mockFindingPoster{},
			RemoteGitHubClient: mockFetcher,
		}}

		input := PostFindingsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
			Findings: []FindingInput{{File: "main.go", Severity: "high"}},
		}

		_, _, err := s.handlePostFindings(context.Background(), nil, input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "rate limit exceeded")
	})

	t.Run("propagates diff fetch errors", func(t *testing.T) {
		mockFetcher := &mockPRMetadataFetcher{
			metadata: &domain.PRMetadata{HeadSHA: "abc123"},
			diffErr:  errors.New("diff too large"),
		}
		s := &Server{deps: ServerDeps{
			FindingPoster:      &mockFindingPoster{},
			RemoteGitHubClient: mockFetcher,
		}}

		input := PostFindingsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
			Findings: []FindingInput{{File: "main.go", Severity: "high"}},
		}

		_, _, err := s.handlePostFindings(context.Background(), nil, input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "diff too large")
	})

	t.Run("propagates post errors", func(t *testing.T) {
		mockPoster := &mockFindingPoster{
			err: errors.New("permission denied"),
		}
		mockFetcher := &mockPRMetadataFetcher{
			metadata: &domain.PRMetadata{HeadSHA: "abc123"},
			diff:     domain.Diff{},
		}
		s := &Server{deps: ServerDeps{
			FindingPoster:      mockPoster,
			RemoteGitHubClient: mockFetcher,
		}}

		input := PostFindingsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
			Findings: []FindingInput{{File: "main.go", Severity: "high"}},
		}

		_, _, err := s.handlePostFindings(context.Background(), nil, input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
	})

	t.Run("validates severity for each finding", func(t *testing.T) {
		s := &Server{deps: ServerDeps{
			FindingPoster:      &mockFindingPoster{},
			RemoteGitHubClient: &mockPRMetadataFetcher{},
		}}

		input := PostFindingsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
			Findings: []FindingInput{
				{File: "main.go", Severity: "high", Category: "security", Description: "Valid"},
				{File: "utils.go", Severity: "invalid-severity", Category: "bug", Description: "Invalid"},
			},
		}

		result, _, err := s.handlePostFindings(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
		textContent, ok := result.Content[0].(*mcp.TextContent)
		require.True(t, ok)
		assert.Contains(t, textContent.Text, "finding 1 has invalid severity")
		assert.Contains(t, textContent.Text, "invalid-severity")
	})

	t.Run("preserves fingerprint from input", func(t *testing.T) {
		mockPoster := &mockFindingPoster{
			result: &review.GitHubPostResult{
				ReviewID:       12345,
				CommentsPosted: 1,
			},
		}
		mockFetcher := &mockPRMetadataFetcher{
			metadata: &domain.PRMetadata{HeadSHA: "abc123"},
			diff:     domain.Diff{},
		}
		s := &Server{deps: ServerDeps{
			FindingPoster:      mockPoster,
			RemoteGitHubClient: mockFetcher,
		}}

		// Use a preserved fingerprint from review_pr output
		preservedFingerprint := "abc123def456preserved"
		input := PostFindingsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
			Findings: []FindingInput{
				{
					Fingerprint: preservedFingerprint,
					File:        "main.go",
					LineStart:   10,
					Severity:    "high",
					Category:    "security",
					Description: "Test issue",
				},
			},
		}

		result, output, err := s.handlePostFindings(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.IsError)

		assert.True(t, output.Success)
		require.Len(t, output.PostedFingerprints, 1)
		assert.Equal(t, preservedFingerprint, output.PostedFingerprints[0], "fingerprint should be preserved from input")
	})

	t.Run("computes fingerprint when not provided", func(t *testing.T) {
		s := &Server{deps: ServerDeps{
			FindingPoster:      &mockFindingPoster{},
			RemoteGitHubClient: &mockPRMetadataFetcher{},
		}}

		input := PostFindingsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
			Findings: []FindingInput{
				{
					// No fingerprint provided
					File:        "main.go",
					LineStart:   10,
					Severity:    "high",
					Category:    "security",
					Description: "Test issue",
				},
			},
			DryRun: true,
		}

		result, output, err := s.handlePostFindings(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.IsError)

		require.Len(t, output.PostedFingerprints, 1)
		assert.NotEmpty(t, output.PostedFingerprints[0], "fingerprint should be computed when not provided")
		// Computed fingerprints are MD5 hex, should be 32 chars
		assert.Len(t, output.PostedFingerprints[0], 32)
	})

	t.Run("skip_duplicates filters existing fingerprints", func(t *testing.T) {
		prService := createPRServiceForDedup([]domain.PRFinding{
			{CommentID: 1, Fingerprint: "existing-fp-1"},
			{CommentID: 2, Fingerprint: "existing-fp-2"},
		})

		server := &Server{
			deps: ServerDeps{
				FindingPoster: &mockFindingPoster{
					result: &review.GitHubPostResult{CommentsPosted: 1},
				},
				RemoteGitHubClient: &mockPRMetadataFetcher{
					metadata: &domain.PRMetadata{HeadSHA: "abc123"},
					diff:     domain.Diff{Files: []domain.FileDiff{{Path: "test.go"}}},
				},
				PRService: prService,
			},
		}

		input := PostFindingsInput{
			Owner:          "owner",
			Repo:           "repo",
			PRNumber:       42,
			SkipDuplicates: true,
			Findings: []FindingInput{
				{File: "test.go", LineStart: 1, LineEnd: 1, Severity: "high", Category: "bug", Description: "New finding", Fingerprint: "new-fp"},
				{File: "test.go", LineStart: 2, LineEnd: 2, Severity: "high", Category: "bug", Description: "Duplicate", Fingerprint: "existing-fp-1"},
			},
		}

		result, output, err := server.handlePostFindings(context.Background(), nil, input)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Equal(t, 1, output.Posted)
		assert.Equal(t, 1, output.SkippedDuplicates)
		require.Len(t, output.PostedFingerprints, 1)
		assert.Equal(t, "new-fp", output.PostedFingerprints[0])
	})

	t.Run("skip_duplicates returns early when all duplicates", func(t *testing.T) {
		prService := createPRServiceForDedup([]domain.PRFinding{
			{CommentID: 1, Fingerprint: "existing-fp-1"},
		})

		server := &Server{
			deps: ServerDeps{
				FindingPoster: &mockFindingPoster{
					result: &review.GitHubPostResult{},
				},
				RemoteGitHubClient: &mockPRMetadataFetcher{
					metadata: &domain.PRMetadata{HeadSHA: "abc123"},
					diff:     domain.Diff{Files: []domain.FileDiff{{Path: "test.go"}}},
				},
				PRService: prService,
			},
		}

		input := PostFindingsInput{
			Owner:          "owner",
			Repo:           "repo",
			PRNumber:       42,
			SkipDuplicates: true,
			Findings: []FindingInput{
				{File: "test.go", LineStart: 1, LineEnd: 1, Severity: "high", Category: "bug", Description: "Duplicate", Fingerprint: "existing-fp-1"},
			},
		}

		result, output, err := server.handlePostFindings(context.Background(), nil, input)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Equal(t, 0, output.Posted)
		assert.Equal(t, 1, output.SkippedDuplicates)
		assert.Contains(t, output.Message, "already exist")
	})

	t.Run("skip_duplicates fails open when ListFindings errors", func(t *testing.T) {
		// Create PRService with a mock that returns an error
		prService := triage.NewPRService(triage.PRServiceDeps{
			CommentReader: &mockCommentReaderForDedup{
				err: errors.New("API rate limit exceeded"),
			},
		})

		server := &Server{
			deps: ServerDeps{
				FindingPoster: &mockFindingPoster{
					result: &review.GitHubPostResult{CommentsPosted: 2},
				},
				RemoteGitHubClient: &mockPRMetadataFetcher{
					metadata: &domain.PRMetadata{HeadSHA: "abc123"},
					diff:     domain.Diff{Files: []domain.FileDiff{{Path: "test.go"}}},
				},
				PRService: prService,
			},
		}

		input := PostFindingsInput{
			Owner:          "owner",
			Repo:           "repo",
			PRNumber:       42,
			SkipDuplicates: true,
			Findings: []FindingInput{
				{File: "test.go", LineStart: 1, LineEnd: 1, Severity: "high", Category: "bug", Description: "Finding 1"},
				{File: "test.go", LineStart: 2, LineEnd: 2, Severity: "medium", Category: "bug", Description: "Finding 2"},
			},
		}

		result, output, err := server.handlePostFindings(context.Background(), nil, input)

		// Should succeed despite ListFindings error (fail open)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Equal(t, 2, output.Posted) // All findings posted since dedup couldn't run
	})

	t.Run("skip_duplicates detects duplicates within same batch", func(t *testing.T) {
		// PRService with no existing findings
		prService := createPRServiceForDedup([]domain.PRFinding{})

		server := &Server{
			deps: ServerDeps{
				FindingPoster: &mockFindingPoster{
					result: &review.GitHubPostResult{CommentsPosted: 1},
				},
				RemoteGitHubClient: &mockPRMetadataFetcher{
					metadata: &domain.PRMetadata{HeadSHA: "abc123"},
					diff:     domain.Diff{Files: []domain.FileDiff{{Path: "test.go"}}},
				},
				PRService: prService,
			},
		}

		// Two findings with the same fingerprint in the same batch
		input := PostFindingsInput{
			Owner:          "owner",
			Repo:           "repo",
			PRNumber:       42,
			SkipDuplicates: true,
			Findings: []FindingInput{
				{File: "test.go", LineStart: 1, LineEnd: 1, Severity: "high", Category: "bug", Description: "Same issue", Fingerprint: "duplicate-fp"},
				{File: "test.go", LineStart: 1, LineEnd: 1, Severity: "high", Category: "bug", Description: "Same issue", Fingerprint: "duplicate-fp"},
			},
		}

		result, output, err := server.handlePostFindings(context.Background(), nil, input)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Equal(t, 1, output.Posted)            // Only 1 should be posted
		assert.Equal(t, 1, output.SkippedDuplicates) // 1 skipped as batch duplicate
	})
}

func TestCountBySeverity(t *testing.T) {
	findings := []domain.Finding{
		{Severity: "high"},
		{Severity: "high"},
		{Severity: "medium"},
		{Severity: "low"},
		{Severity: ""},
	}

	counts := countBySeverity(findings)

	assert.Equal(t, 2, counts["high"])
	assert.Equal(t, 1, counts["medium"])
	assert.Equal(t, 1, counts["low"])
	assert.Equal(t, 1, counts["unknown"])
}

func TestBuildFindingsSummary(t *testing.T) {
	t.Run("empty findings", func(t *testing.T) {
		summary := buildFindingsSummary([]domain.Finding{})
		assert.Equal(t, "No findings detected", summary)
	})

	t.Run("single severity", func(t *testing.T) {
		findings := []domain.Finding{
			{Severity: "high"},
			{Severity: "high"},
		}
		summary := buildFindingsSummary(findings)
		assert.Equal(t, "2 high", summary)
	})

	t.Run("multiple severities", func(t *testing.T) {
		findings := []domain.Finding{
			{Severity: "critical"},
			{Severity: "high"},
			{Severity: "high"},
			{Severity: "medium"},
		}
		summary := buildFindingsSummary(findings)
		assert.Equal(t, "1 critical, 2 high, 1 medium", summary)
	})
}

func TestDetermineReviewAction(t *testing.T) {
	t.Run("uses override when provided", func(t *testing.T) {
		override := "comment"
		action := determineReviewAction(
			[]domain.Finding{{Severity: "critical"}},
			&override,
			nil,
			nil,
		)
		assert.Equal(t, "COMMENT", action)
	})

	t.Run("returns REQUEST_CHANGES for critical findings", func(t *testing.T) {
		action := determineReviewAction(
			[]domain.Finding{{Severity: "critical"}},
			nil,
			nil,
			nil,
		)
		assert.Equal(t, "REQUEST_CHANGES", action)
	})

	t.Run("returns REQUEST_CHANGES for high findings", func(t *testing.T) {
		action := determineReviewAction(
			[]domain.Finding{{Severity: "high"}},
			nil,
			nil,
			nil,
		)
		assert.Equal(t, "REQUEST_CHANGES", action)
	})

	t.Run("returns COMMENT for medium/low findings", func(t *testing.T) {
		action := determineReviewAction(
			[]domain.Finding{{Severity: "medium"}},
			nil,
			nil,
			nil,
		)
		assert.Equal(t, "COMMENT", action)
	})

	t.Run("returns APPROVE when no findings", func(t *testing.T) {
		action := determineReviewAction([]domain.Finding{}, nil, nil, nil)
		assert.Equal(t, "APPROVE", action)
	})

	t.Run("uses preserved fingerprints for blocking check", func(t *testing.T) {
		preservedFP := "preserved-fingerprint-123"
		action := determineReviewAction(
			[]domain.Finding{{Severity: "low", File: "test.go"}}, // low severity normally returns COMMENT
			nil,
			[]string{preservedFP}, // blocking fingerprint
			[]string{preservedFP}, // preserved fingerprint matches blocking
		)
		assert.Equal(t, "REQUEST_CHANGES", action, "should REQUEST_CHANGES when preserved fingerprint matches blocking")
	})
}

// =============================================================================
// review_branch Tests
// =============================================================================

// mockBranchReviewer is a test mock for BranchReviewer interface.
type mockBranchReviewer struct {
	result review.Result
	err    error
}

func (m *mockBranchReviewer) ReviewBranch(ctx context.Context, req review.BranchRequest) (review.Result, error) {
	return m.result, m.err
}

func TestHandleReviewBranch(t *testing.T) {
	t.Run("returns not implemented when BranchReviewer is nil and no sampling", func(t *testing.T) {
		s := &Server{deps: ServerDeps{}}

		input := ReviewBranchInput{
			BaseRef:   "main",
			TargetRef: "feature/no-provider",
		}

		result, output, err := s.handleReviewBranch(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Empty(t, output.Findings)
		// Should provide helpful message about API keys or sampling
		textContent, ok := result.Content[0].(*mcp.TextContent)
		require.True(t, ok)
		assert.Contains(t, textContent.Text, "LLM API keys")
		assert.Contains(t, textContent.Text, "sampling")

		// Verify output has properly initialized maps (not nil) for schema validation
		assert.NotNil(t, output.BySeverity, "BySeverity should be initialized, not nil")
		assert.NotNil(t, output.ByCategory, "ByCategory should be initialized, not nil")
		assert.NotNil(t, output.Findings, "Findings should be initialized, not nil")
		assert.NotNil(t, output.ReviewerStats, "ReviewerStats should be initialized, not nil")
	})

	t.Run("validates required base_ref", func(t *testing.T) {
		mock := &mockBranchReviewer{}
		s := &Server{deps: ServerDeps{BranchReviewer: mock}}

		input := ReviewBranchInput{} // Missing BaseRef

		result, _, err := s.handleReviewBranch(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
		textContent, ok := result.Content[0].(*mcp.TextContent)
		require.True(t, ok)
		assert.Contains(t, textContent.Text, "base_ref is required")
	})

	t.Run("returns findings from branch review", func(t *testing.T) {
		mock := &mockBranchReviewer{
			result: review.Result{
				Reviews: []domain.Review{
					{
						ProviderName: "anthropic",
						ModelName:    "claude-sonnet-4-5",
						TokensIn:     1000,
						TokensOut:    500,
						Cost:         0.01,
						Findings: []domain.Finding{
							{
								ID:          "f1",
								File:        "main.go",
								LineStart:   10,
								LineEnd:     15,
								Severity:    "high",
								Category:    "security",
								Description: "SQL injection vulnerability",
							},
							{
								ID:          "f2",
								File:        "utils.go",
								LineStart:   20,
								LineEnd:     25,
								Severity:    "medium",
								Category:    "bug",
								Description: "Nil pointer dereference",
							},
						},
					},
				},
			},
		}
		s := &Server{deps: ServerDeps{BranchReviewer: mock}}

		input := ReviewBranchInput{
			BaseRef:   "main",
			TargetRef: "feature/test",
		}

		result, output, err := s.handleReviewBranch(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.IsError)

		assert.Equal(t, 2, output.TotalFindings)
		assert.Len(t, output.Findings, 2)
		assert.Equal(t, 1, output.BySeverity["high"])
		assert.Equal(t, 1, output.BySeverity["medium"])
		assert.Equal(t, 1, output.ByCategory["security"])
		assert.Equal(t, 1, output.ByCategory["bug"])
		assert.Equal(t, 1000, output.TokensIn)
		assert.Equal(t, 500, output.TokensOut)
		assert.Equal(t, 0.01, output.Cost)
		assert.Equal(t, "main", output.BaseRef)
		assert.Equal(t, "feature/test", output.TargetRef)
	})

	t.Run("aggregates findings from multiple reviewers", func(t *testing.T) {
		mock := &mockBranchReviewer{
			result: review.Result{
				Reviews: []domain.Review{
					{
						ProviderName: "security",
						TokensIn:     500,
						TokensOut:    200,
						Cost:         0.005,
						Findings: []domain.Finding{
							{ID: "f1", Severity: "high", Category: "security"},
						},
					},
					{
						ProviderName: "architecture",
						TokensIn:     600,
						TokensOut:    300,
						Cost:         0.006,
						Findings: []domain.Finding{
							{ID: "f2", Severity: "medium", Category: "architecture"},
							{ID: "f3", Severity: "low", Category: "maintainability"},
						},
					},
				},
			},
		}
		s := &Server{deps: ServerDeps{BranchReviewer: mock}}

		input := ReviewBranchInput{
			BaseRef:   "main",
			TargetRef: "feature/multi-reviewer",
		}

		result, output, err := s.handleReviewBranch(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 3, output.TotalFindings)
		assert.Equal(t, 1100, output.TokensIn)
		assert.Equal(t, 500, output.TokensOut)
		assert.InDelta(t, 0.011, output.Cost, 0.0001)
	})

	t.Run("handles no findings", func(t *testing.T) {
		mock := &mockBranchReviewer{
			result: review.Result{
				Reviews: []domain.Review{
					{ProviderName: "security", Findings: []domain.Finding{}},
				},
			},
		}
		s := &Server{deps: ServerDeps{BranchReviewer: mock}}

		input := ReviewBranchInput{
			BaseRef:   "main",
			TargetRef: "feature/no-issues",
		}

		result, output, err := s.handleReviewBranch(context.Background(), nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 0, output.TotalFindings)
		assert.Contains(t, output.Message, "no issues found")
		assert.Equal(t, "No findings detected", output.Summary)
	})

	t.Run("propagates review errors", func(t *testing.T) {
		mock := &mockBranchReviewer{
			err: errors.New("git ref not found"),
		}
		s := &Server{deps: ServerDeps{BranchReviewer: mock}}

		input := ReviewBranchInput{
			BaseRef:   "nonexistent-branch",
			TargetRef: "feature/error-test",
		}

		_, _, err := s.handleReviewBranch(context.Background(), nil, input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "git ref not found")
	})

	t.Run("passes include_uncommitted flag correctly", func(t *testing.T) {
		var capturedReq review.BranchRequest
		mock := &mockBranchReviewer{
			result: review.Result{},
		}
		// Create a custom mock to capture the request
		s := &Server{deps: ServerDeps{BranchReviewer: &capturingBranchReviewer{
			result:     review.Result{},
			capturedFn: func(req review.BranchRequest) { capturedReq = req },
		}}}

		input := ReviewBranchInput{
			BaseRef:            "main",
			TargetRef:          "feature/uncommitted-test",
			IncludeUncommitted: true,
		}

		_, _, err := s.handleReviewBranch(context.Background(), nil, input)
		require.NoError(t, err)

		assert.True(t, capturedReq.IncludeUncommitted)
		_ = mock // silence unused warning
	})

	t.Run("passes reviewers correctly", func(t *testing.T) {
		var capturedReq review.BranchRequest
		s := &Server{deps: ServerDeps{BranchReviewer: &capturingBranchReviewer{
			result:     review.Result{},
			capturedFn: func(req review.BranchRequest) { capturedReq = req },
		}}}

		input := ReviewBranchInput{
			BaseRef:   "main",
			TargetRef: "feature/reviewers-test",
			Reviewers: []string{"security", "architecture"},
		}

		_, _, err := s.handleReviewBranch(context.Background(), nil, input)
		require.NoError(t, err)

		assert.Equal(t, []string{"security", "architecture"}, capturedReq.Reviewers)
	})

	t.Run("never sets PostToGitHub", func(t *testing.T) {
		var capturedReq review.BranchRequest
		s := &Server{deps: ServerDeps{BranchReviewer: &capturingBranchReviewer{
			result:     review.Result{},
			capturedFn: func(req review.BranchRequest) { capturedReq = req },
		}}}

		input := ReviewBranchInput{
			BaseRef:   "main",
			TargetRef: "feature/no-github-post",
		}

		_, _, err := s.handleReviewBranch(context.Background(), nil, input)
		require.NoError(t, err)

		assert.False(t, capturedReq.PostToGitHub, "PostToGitHub should always be false for review_branch")
	})
}

// capturingBranchReviewer captures the request for inspection in tests.
type capturingBranchReviewer struct {
	result     review.Result
	err        error
	capturedFn func(review.BranchRequest)
}

func (c *capturingBranchReviewer) ReviewBranch(ctx context.Context, req review.BranchRequest) (review.Result, error) {
	if c.capturedFn != nil {
		c.capturedFn(req)
	}
	return c.result, c.err
}
