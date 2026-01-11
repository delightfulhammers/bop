package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/triage"
)

// =============================================================================
// Integration tests: MCP Handler -> PRService -> Mocked Adapters
// These tests verify the full stack without hitting real GitHub APIs.
// =============================================================================

// MockIntegrationAnnotationReader provides mock GitHub annotation data.
type MockIntegrationAnnotationReader struct {
	mock.Mock
}

func (m *MockIntegrationAnnotationReader) ListCheckRuns(ctx context.Context, owner, repo, ref string, checkName *string) ([]domain.CheckRunSummary, error) {
	args := m.Called(ctx, owner, repo, ref, checkName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.CheckRunSummary), args.Error(1)
}

func (m *MockIntegrationAnnotationReader) GetAnnotations(ctx context.Context, owner, repo string, checkRunID int64) ([]domain.Annotation, error) {
	args := m.Called(ctx, owner, repo, checkRunID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Annotation), args.Error(1)
}

func (m *MockIntegrationAnnotationReader) GetAnnotation(ctx context.Context, owner, repo string, checkRunID int64, index int) (*domain.Annotation, error) {
	args := m.Called(ctx, owner, repo, checkRunID, index)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Annotation), args.Error(1)
}

// MockIntegrationPRReader provides mock PR metadata.
type MockIntegrationPRReader struct {
	mock.Mock
}

func (m *MockIntegrationPRReader) GetPRMetadata(ctx context.Context, owner, repo string, prNumber int) (*domain.PRMetadata, error) {
	args := m.Called(ctx, owner, repo, prNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.PRMetadata), args.Error(1)
}

// MockIntegrationCommentReader provides mock PR comment data.
type MockIntegrationCommentReader struct {
	mock.Mock
}

func (m *MockIntegrationCommentReader) ListPRComments(ctx context.Context, owner, repo string, prNumber int, filterByFingerprint bool) ([]domain.PRFinding, error) {
	args := m.Called(ctx, owner, repo, prNumber, filterByFingerprint)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.PRFinding), args.Error(1)
}

func (m *MockIntegrationCommentReader) GetPRComment(ctx context.Context, owner, repo string, prNumber int, commentID int64) (*domain.PRFinding, error) {
	args := m.Called(ctx, owner, repo, prNumber, commentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.PRFinding), args.Error(1)
}

func (m *MockIntegrationCommentReader) GetPRCommentByFingerprint(ctx context.Context, owner, repo string, prNumber int, fingerprint string) (*domain.PRFinding, error) {
	args := m.Called(ctx, owner, repo, prNumber, fingerprint)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.PRFinding), args.Error(1)
}

func (m *MockIntegrationCommentReader) GetThreadHistory(ctx context.Context, owner, repo string, commentID int64) ([]domain.ThreadComment, error) {
	args := m.Called(ctx, owner, repo, commentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.ThreadComment), args.Error(1)
}

func (m *MockIntegrationCommentReader) ListAllFindings(ctx context.Context, owner, repo string, prNumber int, filterByFingerprint bool) ([]domain.PRFinding, error) {
	args := m.Called(ctx, owner, repo, prNumber, filterByFingerprint)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.PRFinding), args.Error(1)
}

// MockIntegrationCommentWriter provides mock comment write operations.
type MockIntegrationCommentWriter struct {
	mock.Mock
}

func (m *MockIntegrationCommentWriter) ReplyToComment(ctx context.Context, owner, repo string, prNumber int, replyTo int64, body string) (int64, error) {
	args := m.Called(ctx, owner, repo, prNumber, replyTo, body)
	if args.Get(0) == nil {
		return 0, args.Error(1)
	}
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockIntegrationCommentWriter) CreateComment(ctx context.Context, owner, repo string, prNumber int, commitSHA, path string, line int, body string) (int64, error) {
	args := m.Called(ctx, owner, repo, prNumber, commitSHA, path, line, body)
	if args.Get(0) == nil {
		return 0, args.Error(1)
	}
	return args.Get(0).(int64), args.Error(1)
}

// MockIntegrationReviewManager provides mock review management operations.
type MockIntegrationReviewManager struct {
	mock.Mock
}

func (m *MockIntegrationReviewManager) ListReviews(ctx context.Context, owner, repo string, prNumber int) ([]triage.Review, error) {
	args := m.Called(ctx, owner, repo, prNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]triage.Review), args.Error(1)
}

func (m *MockIntegrationReviewManager) DismissReview(ctx context.Context, owner, repo string, prNumber int, reviewID int64, message string) error {
	args := m.Called(ctx, owner, repo, prNumber, reviewID, message)
	return args.Error(0)
}

func (m *MockIntegrationReviewManager) RequestReviewers(ctx context.Context, owner, repo string, prNumber int, reviewers, teamReviewers []string) error {
	args := m.Called(ctx, owner, repo, prNumber, reviewers, teamReviewers)
	return args.Error(0)
}

func (m *MockIntegrationReviewManager) FindThreadForComment(ctx context.Context, owner, repo string, prNumber int, commentID int64) (*triage.ThreadInfo, error) {
	args := m.Called(ctx, owner, repo, prNumber, commentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*triage.ThreadInfo), args.Error(1)
}

func (m *MockIntegrationReviewManager) ResolveThread(ctx context.Context, owner, repo, threadID string) error {
	args := m.Called(ctx, owner, repo, threadID)
	return args.Error(0)
}

func (m *MockIntegrationReviewManager) UnresolveThread(ctx context.Context, owner, repo, threadID string) error {
	args := m.Called(ctx, owner, repo, threadID)
	return args.Error(0)
}

// =============================================================================
// Integration Test: Full Triage Flow
// =============================================================================

func TestIntegration_TriageWorkflow(t *testing.T) {
	ctx := context.Background()

	// Setup mocks for the complete stack
	mockAnnotations := new(MockIntegrationAnnotationReader)
	mockPR := new(MockIntegrationPRReader)
	mockComments := new(MockIntegrationCommentReader)
	mockCommentWriter := new(MockIntegrationCommentWriter)
	mockReviewManager := new(MockIntegrationReviewManager)

	// Setup test data
	prMeta := &domain.PRMetadata{
		Owner:   "testorg",
		Repo:    "testrepo",
		Number:  123,
		HeadSHA: "abc123def",
		BaseRef: "main",
	}

	checkRuns := []domain.CheckRunSummary{
		{ID: 1001, Name: "bop", Status: "completed", AnnotationCount: 1},
	}

	annotations := []domain.Annotation{
		{
			CheckRunID: 1001,
			Index:      0,
			Path:       "src/main.go",
			StartLine:  42,
			EndLine:    42,
			Level:      domain.AnnotationLevelWarning,
			Message:    "Potential null pointer dereference",
			Title:      "null-check",
		},
	}

	findings := []domain.PRFinding{
		{
			CommentID:   2001,
			Fingerprint: "CR_FP:abc123",
			Path:        "src/handler.go",
			Line:        100,
			Severity:    "high",
			Category:    "security",
			Body:        "SQL injection vulnerability detected",
			Author:      "github-actions[bot]",
			IsResolved:  false,
			ReplyCount:  0,
		},
	}

	// Configure mock expectations
	mockPR.On("GetPRMetadata", ctx, "testorg", "testrepo", 123).Return(prMeta, nil)
	mockAnnotations.On("ListCheckRuns", ctx, "testorg", "testrepo", "abc123def", (*string)(nil)).Return(checkRuns, nil)
	mockAnnotations.On("GetAnnotations", ctx, "testorg", "testrepo", int64(1001)).Return(annotations, nil)
	mockComments.On("ListAllFindings", ctx, "testorg", "testrepo", 123, true).Return(findings, nil)
	mockComments.On("GetPRCommentByFingerprint", ctx, "testorg", "testrepo", 123, "abc123").Return(&findings[0], nil)
	mockCommentWriter.On("ReplyToComment", ctx, "testorg", "testrepo", 123, int64(2001), mock.AnythingOfType("string")).Return(int64(3001), nil)
	// Note: FindThreadForComment is not called by reply - it's used by get_thread handler
	mockReviewManager.On("ResolveThread", ctx, "testorg", "testrepo", "PRRT_thread123").Return(nil)

	// Create PRService with all mocks
	svc := triage.NewPRService(triage.PRServiceDeps{
		AnnotationReader: mockAnnotations,
		PRReader:         mockPR,
		CommentReader:    mockComments,
		CommentWriter:    mockCommentWriter,
		ReviewManager:    mockReviewManager,
	})

	// Create MCP server with the service
	server := NewServer(ServerDeps{PRService: svc})

	t.Run("step 1: list annotations from SARIF", func(t *testing.T) {
		input := ListAnnotationsInput{
			Owner:    "testorg",
			Repo:     "testrepo",
			PRNumber: 123,
		}

		result, output, err := server.handleListAnnotations(ctx, nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Equal(t, 1, output.Total)
		assert.Equal(t, "src/main.go", output.Annotations[0].Path)
		assert.Equal(t, "warning", output.Annotations[0].Level)
	})

	t.Run("step 2: list findings from PR comments", func(t *testing.T) {
		input := ListFindingsInput{
			Owner:    "testorg",
			Repo:     "testrepo",
			PRNumber: 123,
		}

		result, output, err := server.handleListFindings(ctx, nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Equal(t, 1, output.Total)
		assert.Equal(t, "high", output.Findings[0].Severity)
		assert.Equal(t, "security", output.Findings[0].Category)
	})

	t.Run("step 3: get specific finding by fingerprint", func(t *testing.T) {
		input := GetFindingInput{
			Owner:     "testorg",
			Repo:      "testrepo",
			PRNumber:  123,
			FindingID: "CR_FP:abc123",
		}

		result, output, err := server.handleGetFinding(ctx, nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Equal(t, "SQL injection vulnerability detected", output.Finding.Body)
	})

	t.Run("step 4: reply to finding with status", func(t *testing.T) {
		status := "fixed"
		input := ReplyToFindingInput{
			Owner:     "testorg",
			Repo:      "testrepo",
			PRNumber:  123,
			FindingID: "CR_FP:abc123",
			Body:      "Fixed in commit abc123. Added parameterized query.",
			Status:    &status,
		}

		result, output, err := server.handleReplyToFinding(ctx, nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.True(t, output.Success)
		assert.Equal(t, int64(3001), output.CommentID)
	})

	t.Run("step 5: mark thread as resolved", func(t *testing.T) {
		input := MarkResolvedInput{
			Owner:    "testorg",
			Repo:     "testrepo",
			ThreadID: "PRRT_thread123",
			Resolved: true,
		}

		result, output, err := server.handleMarkResolved(ctx, nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.True(t, output.Success)
		assert.True(t, output.Resolved)
	})

	// Verify all mock expectations
	mockAnnotations.AssertExpectations(t)
	mockPR.AssertExpectations(t)
	mockComments.AssertExpectations(t)
	mockCommentWriter.AssertExpectations(t)
	mockReviewManager.AssertExpectations(t)
}

// =============================================================================
// Integration Test: Error Propagation Through Stack
// =============================================================================

func TestIntegration_ErrorPropagation(t *testing.T) {
	ctx := context.Background()

	t.Run("PR metadata error propagates to handler", func(t *testing.T) {
		mockPR := new(MockIntegrationPRReader)
		mockAnnotations := new(MockIntegrationAnnotationReader)

		mockPR.On("GetPRMetadata", ctx, "testorg", "testrepo", 123).Return(nil, assert.AnError)

		svc := triage.NewPRService(triage.PRServiceDeps{
			AnnotationReader: mockAnnotations,
			PRReader:         mockPR,
		})
		server := NewServer(ServerDeps{PRService: svc})

		input := ListAnnotationsInput{
			Owner:    "testorg",
			Repo:     "testrepo",
			PRNumber: 123,
		}

		_, _, err := server.handleListAnnotations(ctx, nil, input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get PR metadata")
	})

	t.Run("comment reader error propagates to handler", func(t *testing.T) {
		mockComments := new(MockIntegrationCommentReader)
		mockComments.On("ListAllFindings", ctx, "testorg", "testrepo", 123, true).Return(nil, assert.AnError)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader: mockComments,
		})
		server := NewServer(ServerDeps{PRService: svc})

		input := ListFindingsInput{
			Owner:    "testorg",
			Repo:     "testrepo",
			PRNumber: 123,
		}

		_, _, err := server.handleListFindings(ctx, nil, input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list findings")
	})
}

// =============================================================================
// Integration Test: Review Dismissal and Re-request
// =============================================================================

func TestIntegration_ReviewManagement(t *testing.T) {
	ctx := context.Background()

	mockReviewManager := new(MockIntegrationReviewManager)

	// Setup bot review that should be dismissed
	botReviews := []triage.Review{
		{
			ID:          5001,
			User:        "github-actions[bot]",
			UserType:    "Bot",
			State:       "CHANGES_REQUESTED",
			SubmittedAt: time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
		},
	}

	mockReviewManager.On("ListReviews", ctx, "testorg", "testrepo", 123).Return(botReviews, nil)
	mockReviewManager.On("DismissReview", ctx, "testorg", "testrepo", 123, int64(5001), "Addressed in latest push").Return(nil)
	// Note: RequestReviewers is only called if reviewers/teams are specified, which they aren't in this test

	svc := triage.NewPRService(triage.PRServiceDeps{
		ReviewManager: mockReviewManager,
	})
	server := NewServer(ServerDeps{PRService: svc})

	t.Run("dismiss stale bot reviews and request re-review", func(t *testing.T) {
		input := RequestRereviewInput{
			Owner:        "testorg",
			Repo:         "testrepo",
			PRNumber:     123,
			DismissStale: true,
			Message:      "Addressed in latest push",
		}

		result, output, err := server.handleRequestRereview(ctx, nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.True(t, output.Success)
		assert.Equal(t, 1, output.ReviewsDismissed)

		mockReviewManager.AssertExpectations(t)
	})
}

// =============================================================================
// Integration Test: Thread Operations
// =============================================================================

func TestIntegration_ThreadOperations(t *testing.T) {
	ctx := context.Background()

	mockComments := new(MockIntegrationCommentReader)
	mockReviewManager := new(MockIntegrationReviewManager)

	threadHistory := []domain.ThreadComment{
		{
			Author:    "github-actions[bot]",
			Body:      "Found potential memory leak",
			CreatedAt: time.Now().Add(-2 * time.Hour),
			IsReply:   false,
		},
		{
			Author:    "developer",
			Body:      "Fixed by adding defer statement",
			CreatedAt: time.Now().Add(-1 * time.Hour),
			IsReply:   true,
		},
	}

	mockComments.On("GetThreadHistory", ctx, "testorg", "testrepo", int64(2001)).Return(threadHistory, nil)
	mockReviewManager.On("FindThreadForComment", ctx, "testorg", "testrepo", 123, int64(2001)).Return(&triage.ThreadInfo{
		ID:         "PRRT_thread456",
		IsResolved: false,
	}, nil)

	svc := triage.NewPRService(triage.PRServiceDeps{
		CommentReader: mockComments,
		ReviewManager: mockReviewManager,
	})
	server := NewServer(ServerDeps{PRService: svc})

	t.Run("get thread history with thread ID lookup", func(t *testing.T) {
		input := GetThreadInput{
			Owner:     "testorg",
			Repo:      "testrepo",
			PRNumber:  123,
			CommentID: 2001,
		}

		result, output, err := server.handleGetThread(ctx, nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Equal(t, 2, output.Total)
		assert.Equal(t, "PRRT_thread456", output.ThreadID)
		assert.False(t, output.IsResolved)
		assert.Equal(t, "github-actions[bot]", output.Comments[0].Author)
		assert.True(t, output.Comments[1].IsReply)

		mockComments.AssertExpectations(t)
		mockReviewManager.AssertExpectations(t)
	})
}
