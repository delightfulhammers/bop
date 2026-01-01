package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/bkyoung/code-reviewer/internal/usecase/triage"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// =============================================================================
// Mock Implementations
// =============================================================================

// MockAnnotationReader implements triage.AnnotationReader for testing.
type MockAnnotationReader struct {
	mock.Mock
}

func (m *MockAnnotationReader) ListCheckRuns(ctx context.Context, owner, repo, ref string, checkName *string) ([]domain.CheckRunSummary, error) {
	args := m.Called(ctx, owner, repo, ref, checkName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.CheckRunSummary), args.Error(1)
}

func (m *MockAnnotationReader) GetAnnotations(ctx context.Context, owner, repo string, checkRunID int64) ([]domain.Annotation, error) {
	args := m.Called(ctx, owner, repo, checkRunID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Annotation), args.Error(1)
}

func (m *MockAnnotationReader) GetAnnotation(ctx context.Context, owner, repo string, checkRunID int64, index int) (*domain.Annotation, error) {
	args := m.Called(ctx, owner, repo, checkRunID, index)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Annotation), args.Error(1)
}

// MockPRReader implements triage.PRReader for testing.
type MockPRReader struct {
	mock.Mock
}

func (m *MockPRReader) GetPRMetadata(ctx context.Context, owner, repo string, prNumber int) (*domain.PRMetadata, error) {
	args := m.Called(ctx, owner, repo, prNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.PRMetadata), args.Error(1)
}

// MockFileReader implements triage.FileReader for testing.
type MockFileReader struct {
	mock.Mock
}

func (m *MockFileReader) ReadFile(ctx context.Context, path, ref string) (string, error) {
	args := m.Called(ctx, path, ref)
	return args.String(0), args.Error(1)
}

func (m *MockFileReader) ReadFileLines(ctx context.Context, path, ref string, startLine, endLine, contextLines int) (*domain.CodeContext, error) {
	args := m.Called(ctx, path, ref, startLine, endLine, contextLines)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.CodeContext), args.Error(1)
}

// MockDiffReader implements triage.DiffReader for testing.
type MockDiffReader struct {
	mock.Mock
}

func (m *MockDiffReader) GetDiffHunk(ctx context.Context, baseBranch, targetRef, file string, startLine, endLine int) (*domain.DiffContext, error) {
	args := m.Called(ctx, baseBranch, targetRef, file, startLine, endLine)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.DiffContext), args.Error(1)
}

// MockCommentReader implements triage.CommentReader for testing.
type MockCommentReader struct {
	mock.Mock
}

func (m *MockCommentReader) ListPRComments(ctx context.Context, owner, repo string, prNumber int, filterByFingerprint bool) ([]domain.PRFinding, error) {
	args := m.Called(ctx, owner, repo, prNumber, filterByFingerprint)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.PRFinding), args.Error(1)
}

func (m *MockCommentReader) GetPRComment(ctx context.Context, owner, repo string, prNumber int, commentID int64) (*domain.PRFinding, error) {
	args := m.Called(ctx, owner, repo, prNumber, commentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.PRFinding), args.Error(1)
}

func (m *MockCommentReader) GetPRCommentByFingerprint(ctx context.Context, owner, repo string, prNumber int, fingerprint string) (*domain.PRFinding, error) {
	args := m.Called(ctx, owner, repo, prNumber, fingerprint)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.PRFinding), args.Error(1)
}

func (m *MockCommentReader) GetThreadHistory(ctx context.Context, owner, repo string, commentID int64) ([]domain.ThreadComment, error) {
	args := m.Called(ctx, owner, repo, commentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.ThreadComment), args.Error(1)
}

// MockSuggestionExtractor implements triage.SuggestionExtractor for testing.
type MockSuggestionExtractor struct {
	mock.Mock
}

func (m *MockSuggestionExtractor) ExtractFromAnnotation(annotation *domain.Annotation) (*domain.Suggestion, error) {
	args := m.Called(annotation)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Suggestion), args.Error(1)
}

func (m *MockSuggestionExtractor) ExtractFromComment(finding *domain.PRFinding) (*domain.Suggestion, error) {
	args := m.Called(finding)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Suggestion), args.Error(1)
}

// MockCommentWriter implements triage.CommentWriter for testing.
type MockCommentWriter struct {
	mock.Mock
}

func (m *MockCommentWriter) ReplyToComment(ctx context.Context, owner, repo string, prNumber int, replyTo int64, body string) (int64, error) {
	args := m.Called(ctx, owner, repo, prNumber, replyTo, body)
	id, _ := args.Get(0).(int64)
	return id, args.Error(1)
}

func (m *MockCommentWriter) CreateComment(ctx context.Context, owner, repo string, prNumber int, commitSHA, path string, line int, body string) (int64, error) {
	args := m.Called(ctx, owner, repo, prNumber, commitSHA, path, line, body)
	id, _ := args.Get(0).(int64)
	return id, args.Error(1)
}

// MockReviewManager implements triage.ReviewManager for testing.
type MockReviewManager struct {
	mock.Mock
}

func (m *MockReviewManager) ResolveThread(ctx context.Context, owner, repo, threadID string) error {
	args := m.Called(ctx, owner, repo, threadID)
	return args.Error(0)
}

func (m *MockReviewManager) UnresolveThread(ctx context.Context, owner, repo, threadID string) error {
	args := m.Called(ctx, owner, repo, threadID)
	return args.Error(0)
}

func (m *MockReviewManager) DismissReview(ctx context.Context, owner, repo string, prNumber int, reviewID int64, message string) error {
	args := m.Called(ctx, owner, repo, prNumber, reviewID, message)
	return args.Error(0)
}

func (m *MockReviewManager) RequestReviewers(ctx context.Context, owner, repo string, prNumber int, reviewers []string, teamReviewers []string) error {
	args := m.Called(ctx, owner, repo, prNumber, reviewers, teamReviewers)
	return args.Error(0)
}

func (m *MockReviewManager) ListReviews(ctx context.Context, owner, repo string, prNumber int) ([]triage.Review, error) {
	args := m.Called(ctx, owner, repo, prNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]triage.Review), args.Error(1)
}

func (m *MockReviewManager) FindThreadForComment(ctx context.Context, owner, repo string, prNumber int, commentID int64) (*triage.ThreadInfo, error) {
	args := m.Called(ctx, owner, repo, prNumber, commentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*triage.ThreadInfo), args.Error(1)
}

// =============================================================================
// Helper Functions
// =============================================================================

// ptrString returns a pointer to the given string.
func ptrString(s string) *string {
	return &s
}

// createTestServer creates a Server with the given PRService for testing.
func createTestServer(prService *triage.PRService) *Server {
	return NewServer(ServerDeps{
		PRService: prService,
	})
}

// =============================================================================
// M2 Read Handler Tests
// =============================================================================

func TestServer_handleListAnnotations(t *testing.T) {
	ctx := context.Background()

	t.Run("returns annotations for PR", func(t *testing.T) {
		mockAnn := new(MockAnnotationReader)
		mockPR := new(MockPRReader)

		prMeta := &domain.PRMetadata{
			Owner:   "owner",
			Repo:    "repo",
			Number:  42,
			HeadSHA: "abc123",
		}
		mockPR.On("GetPRMetadata", ctx, "owner", "repo", 42).Return(prMeta, nil)

		checkRuns := []domain.CheckRunSummary{
			{ID: 1001, Name: "code-reviewer", Status: "completed", AnnotationCount: 2},
		}
		mockAnn.On("ListCheckRuns", ctx, "owner", "repo", "abc123", (*string)(nil)).Return(checkRuns, nil)

		annotations := []domain.Annotation{
			{CheckRunID: 1001, Index: 0, Path: "main.go", StartLine: 10, Level: domain.AnnotationLevelWarning, Message: "Warning 1"},
			{CheckRunID: 1001, Index: 1, Path: "util.go", StartLine: 20, Level: domain.AnnotationLevelFailure, Message: "Error 1"},
		}
		mockAnn.On("GetAnnotations", ctx, "owner", "repo", int64(1001)).Return(annotations, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			AnnotationReader: mockAnn,
			PRReader:         mockPR,
		})
		server := createTestServer(svc)

		input := ListAnnotationsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 42,
		}

		result, output, err := server.handleListAnnotations(ctx, nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Equal(t, 2, output.Total)
		assert.Len(t, output.Annotations, 2)
		assert.Equal(t, "main.go", output.Annotations[0].Path)
		assert.Equal(t, "util.go", output.Annotations[1].Path)

		mockPR.AssertExpectations(t)
		mockAnn.AssertExpectations(t)
	})

	t.Run("returns not implemented when PRService is nil", func(t *testing.T) {
		server := NewServer(ServerDeps{PRService: nil})

		input := ListAnnotationsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 42,
		}

		result, output, err := server.handleListAnnotations(ctx, nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Equal(t, 0, output.Total)
	})

	t.Run("returns error for invalid level filter", func(t *testing.T) {
		mockAnn := new(MockAnnotationReader)
		mockPR := new(MockPRReader)

		svc := triage.NewPRService(triage.PRServiceDeps{
			AnnotationReader: mockAnn,
			PRReader:         mockPR,
		})
		server := createTestServer(svc)

		input := ListAnnotationsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 42,
			Level:    ptrString("invalid"),
		}

		result, _, err := server.handleListAnnotations(ctx, nil, input)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
	})

	t.Run("filters by check name", func(t *testing.T) {
		mockAnn := new(MockAnnotationReader)
		mockPR := new(MockPRReader)

		prMeta := &domain.PRMetadata{HeadSHA: "abc123"}
		mockPR.On("GetPRMetadata", ctx, "owner", "repo", 42).Return(prMeta, nil)

		checkName := "anthropic"
		checkRuns := []domain.CheckRunSummary{
			{ID: 2001, Name: "anthropic", AnnotationCount: 1},
		}
		mockAnn.On("ListCheckRuns", ctx, "owner", "repo", "abc123", &checkName).Return(checkRuns, nil)

		annotations := []domain.Annotation{
			{CheckRunID: 2001, Index: 0, Path: "test.go", Level: domain.AnnotationLevelWarning},
		}
		mockAnn.On("GetAnnotations", ctx, "owner", "repo", int64(2001)).Return(annotations, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			AnnotationReader: mockAnn,
			PRReader:         mockPR,
		})
		server := createTestServer(svc)

		input := ListAnnotationsInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			CheckName: &checkName,
		}

		result, output, err := server.handleListAnnotations(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Equal(t, 1, output.Total)

		mockPR.AssertExpectations(t)
		mockAnn.AssertExpectations(t)
	})

	t.Run("returns empty list when no annotations", func(t *testing.T) {
		mockAnn := new(MockAnnotationReader)
		mockPR := new(MockPRReader)

		prMeta := &domain.PRMetadata{HeadSHA: "abc123"}
		mockPR.On("GetPRMetadata", ctx, "owner", "repo", 42).Return(prMeta, nil)
		mockAnn.On("ListCheckRuns", ctx, "owner", "repo", "abc123", (*string)(nil)).Return([]domain.CheckRunSummary{}, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			AnnotationReader: mockAnn,
			PRReader:         mockPR,
		})
		server := createTestServer(svc)

		input := ListAnnotationsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 42,
		}

		result, output, err := server.handleListAnnotations(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Equal(t, 0, output.Total)
		assert.Empty(t, output.Annotations)

		mockPR.AssertExpectations(t)
		mockAnn.AssertExpectations(t)
	})

	t.Run("propagates service errors", func(t *testing.T) {
		mockAnn := new(MockAnnotationReader)
		mockPR := new(MockPRReader)

		mockPR.On("GetPRMetadata", ctx, "owner", "repo", 42).Return(nil, errors.New("API error"))

		svc := triage.NewPRService(triage.PRServiceDeps{
			AnnotationReader: mockAnn,
			PRReader:         mockPR,
		})
		server := createTestServer(svc)

		input := ListAnnotationsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 42,
		}

		_, _, err := server.handleListAnnotations(ctx, nil, input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "API error")

		mockPR.AssertExpectations(t)
	})
}

func TestServer_handleGetAnnotation(t *testing.T) {
	ctx := context.Background()

	t.Run("returns annotation by ID and index", func(t *testing.T) {
		mockAnn := new(MockAnnotationReader)

		annotation := &domain.Annotation{
			CheckRunID: 1001,
			Index:      0,
			Path:       "main.go",
			StartLine:  10,
			Level:      domain.AnnotationLevelWarning,
			Message:    "Unused variable",
			Title:      "Warning",
		}
		mockAnn.On("GetAnnotation", ctx, "owner", "repo", int64(1001), 0).Return(annotation, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			AnnotationReader: mockAnn,
		})
		server := createTestServer(svc)

		input := GetAnnotationInput{
			Owner:      "owner",
			Repo:       "repo",
			CheckRunID: 1001,
			Index:      0,
		}

		result, output, err := server.handleGetAnnotation(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Equal(t, "main.go", output.Annotation.Path)
		assert.Equal(t, "Unused variable", output.Annotation.Message)

		mockAnn.AssertExpectations(t)
	})

	t.Run("returns not implemented when PRService is nil", func(t *testing.T) {
		server := NewServer(ServerDeps{PRService: nil})

		input := GetAnnotationInput{
			Owner:      "owner",
			Repo:       "repo",
			CheckRunID: 1001,
			Index:      0,
		}

		result, _, err := server.handleGetAnnotation(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns error when annotation not found", func(t *testing.T) {
		mockAnn := new(MockAnnotationReader)

		mockAnn.On("GetAnnotation", ctx, "owner", "repo", int64(1001), 99).Return(nil, triage.ErrAnnotationNotFound)

		svc := triage.NewPRService(triage.PRServiceDeps{
			AnnotationReader: mockAnn,
		})
		server := createTestServer(svc)

		input := GetAnnotationInput{
			Owner:      "owner",
			Repo:       "repo",
			CheckRunID: 1001,
			Index:      99,
		}

		result, _, err := server.handleGetAnnotation(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)

		mockAnn.AssertExpectations(t)
	})

	t.Run("propagates unexpected errors", func(t *testing.T) {
		mockAnn := new(MockAnnotationReader)

		mockAnn.On("GetAnnotation", ctx, "owner", "repo", int64(1001), 0).Return(nil, errors.New("network error"))

		svc := triage.NewPRService(triage.PRServiceDeps{
			AnnotationReader: mockAnn,
		})
		server := createTestServer(svc)

		input := GetAnnotationInput{
			Owner:      "owner",
			Repo:       "repo",
			CheckRunID: 1001,
			Index:      0,
		}

		_, _, err := server.handleGetAnnotation(ctx, nil, input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "network error")

		mockAnn.AssertExpectations(t)
	})
}

func TestServer_handleListFindings(t *testing.T) {
	ctx := context.Background()

	t.Run("returns findings for PR", func(t *testing.T) {
		mockComment := new(MockCommentReader)

		findings := []domain.PRFinding{
			{CommentID: 1, Fingerprint: "fp1", Path: "main.go", Line: 10, Severity: "high", Body: "Issue 1"},
			{CommentID: 2, Fingerprint: "fp2", Path: "util.go", Line: 20, Severity: "medium", Body: "Issue 2"},
		}
		mockComment.On("ListPRComments", ctx, "owner", "repo", 42, true).Return(findings, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader: mockComment,
		})
		server := createTestServer(svc)

		input := ListFindingsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 42,
		}

		result, output, err := server.handleListFindings(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Equal(t, 2, output.Total)
		assert.Len(t, output.Findings, 2)

		mockComment.AssertExpectations(t)
	})

	t.Run("returns not implemented when PRService is nil", func(t *testing.T) {
		server := NewServer(ServerDeps{PRService: nil})

		input := ListFindingsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 42,
		}

		result, output, err := server.handleListFindings(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Equal(t, 0, output.Total)
	})

	t.Run("filters by severity", func(t *testing.T) {
		mockComment := new(MockCommentReader)

		findings := []domain.PRFinding{
			{CommentID: 1, Fingerprint: "fp1", Severity: "high"},
			{CommentID: 2, Fingerprint: "fp2", Severity: "medium"},
			{CommentID: 3, Fingerprint: "fp3", Severity: "high"},
		}
		mockComment.On("ListPRComments", ctx, "owner", "repo", 42, true).Return(findings, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader: mockComment,
		})
		server := createTestServer(svc)

		severity := "high"
		input := ListFindingsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 42,
			Severity: &severity,
		}

		result, output, err := server.handleListFindings(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Equal(t, 2, output.Total)

		mockComment.AssertExpectations(t)
	})

	t.Run("returns error for invalid severity filter", func(t *testing.T) {
		mockComment := new(MockCommentReader)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader: mockComment,
		})
		server := createTestServer(svc)

		severity := "invalid"
		input := ListFindingsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 42,
			Severity: &severity,
		}

		result, _, err := server.handleListFindings(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns empty list when no findings", func(t *testing.T) {
		mockComment := new(MockCommentReader)

		mockComment.On("ListPRComments", ctx, "owner", "repo", 42, true).Return([]domain.PRFinding{}, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader: mockComment,
		})
		server := createTestServer(svc)

		input := ListFindingsInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 42,
		}

		result, output, err := server.handleListFindings(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Equal(t, 0, output.Total)
		assert.Empty(t, output.Findings)

		mockComment.AssertExpectations(t)
	})
}

func TestServer_handleGetFinding(t *testing.T) {
	ctx := context.Background()

	t.Run("returns finding by comment ID", func(t *testing.T) {
		mockComment := new(MockCommentReader)

		finding := &domain.PRFinding{
			CommentID:   12345,
			Fingerprint: "fp-abc",
			Path:        "main.go",
			Line:        50,
			Body:        "Fix this issue",
			Severity:    "high",
		}
		mockComment.On("GetPRComment", ctx, "owner", "repo", 42, int64(12345)).Return(finding, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader: mockComment,
		})
		server := createTestServer(svc)

		input := GetFindingInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			FindingID: "12345",
		}

		result, output, err := server.handleGetFinding(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Equal(t, int64(12345), output.Finding.CommentID)
		assert.Equal(t, "fp-abc", output.Finding.Fingerprint)

		mockComment.AssertExpectations(t)
	})

	t.Run("returns finding by fingerprint", func(t *testing.T) {
		mockComment := new(MockCommentReader)

		finding := &domain.PRFinding{
			CommentID:   99999,
			Fingerprint: "fp-xyz-123",
			Path:        "util.go",
			Line:        20,
		}
		mockComment.On("GetPRCommentByFingerprint", ctx, "owner", "repo", 42, "fp-xyz-123").Return(finding, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader: mockComment,
		})
		server := createTestServer(svc)

		input := GetFindingInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			FindingID: "CR_FP:fp-xyz-123",
		}

		result, output, err := server.handleGetFinding(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Equal(t, "fp-xyz-123", output.Finding.Fingerprint)

		mockComment.AssertExpectations(t)
	})

	t.Run("returns not implemented when PRService is nil", func(t *testing.T) {
		server := NewServer(ServerDeps{PRService: nil})

		input := GetFindingInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			FindingID: "12345",
		}

		result, _, err := server.handleGetFinding(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns error when finding not found", func(t *testing.T) {
		mockComment := new(MockCommentReader)

		mockComment.On("GetPRComment", ctx, "owner", "repo", 42, int64(999)).Return(nil, triage.ErrCommentNotFound)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader: mockComment,
		})
		server := createTestServer(svc)

		input := GetFindingInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			FindingID: "999",
		}

		result, _, err := server.handleGetFinding(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)

		mockComment.AssertExpectations(t)
	})
}

func TestServer_handleGetSuggestion(t *testing.T) {
	ctx := context.Background()

	t.Run("returns suggestion from comment", func(t *testing.T) {
		mockComment := new(MockCommentReader)
		mockExtractor := new(MockSuggestionExtractor)

		finding := &domain.PRFinding{
			CommentID: 123,
			Path:      "main.go",
			Body:      "```suggestion\nreturn nil\n```",
		}
		mockComment.On("GetPRComment", ctx, "owner", "repo", 42, int64(123)).Return(finding, nil)

		suggestion := &domain.Suggestion{
			File:    "main.go",
			OldCode: "return err",
			NewCode: "return nil",
			Source:  "comment",
		}
		mockExtractor.On("ExtractFromComment", finding).Return(suggestion, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader:       mockComment,
			SuggestionExtractor: mockExtractor,
		})
		server := createTestServer(svc)

		input := GetSuggestionInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			FindingID: "123",
		}

		result, output, err := server.handleGetSuggestion(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Equal(t, "main.go", output.File)
		assert.Equal(t, "return nil", output.NewCode)

		mockComment.AssertExpectations(t)
		mockExtractor.AssertExpectations(t)
	})

	t.Run("returns not implemented when PRService is nil", func(t *testing.T) {
		server := NewServer(ServerDeps{PRService: nil})

		input := GetSuggestionInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			FindingID: "123",
		}

		result, _, err := server.handleGetSuggestion(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns error when no suggestion found", func(t *testing.T) {
		mockComment := new(MockCommentReader)
		mockExtractor := new(MockSuggestionExtractor)

		finding := &domain.PRFinding{
			CommentID: 123,
			Path:      "main.go",
			Body:      "Just a comment, no suggestion",
		}
		mockComment.On("GetPRComment", ctx, "owner", "repo", 42, int64(123)).Return(finding, nil)
		mockExtractor.On("ExtractFromComment", finding).Return(nil, triage.ErrNoSuggestion)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader:       mockComment,
			SuggestionExtractor: mockExtractor,
		})
		server := createTestServer(svc)

		input := GetSuggestionInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			FindingID: "123",
		}

		result, _, err := server.handleGetSuggestion(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)

		mockComment.AssertExpectations(t)
		mockExtractor.AssertExpectations(t)
	})
}

func TestServer_handleGetCodeContext(t *testing.T) {
	ctx := context.Background()

	t.Run("returns code context for file and lines", func(t *testing.T) {
		mockFile := new(MockFileReader)
		mockPR := new(MockPRReader)

		prMeta := &domain.PRMetadata{HeadSHA: "abc123"}
		mockPR.On("GetPRMetadata", ctx, "owner", "repo", 42).Return(prMeta, nil)

		codeCtx := &domain.CodeContext{
			File:      "main.go",
			Ref:       "abc123",
			StartLine: 8,
			EndLine:   12,
			Content:   "func main() {\n\tprintln(\"hello\")\n}",
		}
		mockFile.On("ReadFileLines", ctx, "main.go", "abc123", 10, 10, 3).Return(codeCtx, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			PRReader:   mockPR,
			FileReader: mockFile,
		})
		server := createTestServer(svc)

		input := GetCodeContextInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			File:      "main.go",
			StartLine: 10,
			EndLine:   10,
		}

		result, output, err := server.handleGetCodeContext(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Equal(t, "main.go", output.File)
		assert.Contains(t, output.Content, "println")

		mockPR.AssertExpectations(t)
		mockFile.AssertExpectations(t)
	})

	t.Run("returns not implemented when PRService is nil", func(t *testing.T) {
		server := NewServer(ServerDeps{PRService: nil})

		input := GetCodeContextInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			File:      "main.go",
			StartLine: 10,
			EndLine:   10,
		}

		result, _, err := server.handleGetCodeContext(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns error for invalid PR number", func(t *testing.T) {
		mockFile := new(MockFileReader)
		mockPR := new(MockPRReader)

		svc := triage.NewPRService(triage.PRServiceDeps{
			PRReader:   mockPR,
			FileReader: mockFile,
		})
		server := createTestServer(svc)

		input := GetCodeContextInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  -1,
			File:      "main.go",
			StartLine: 10,
			EndLine:   10,
		}

		result, _, err := server.handleGetCodeContext(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns error for invalid line range", func(t *testing.T) {
		mockFile := new(MockFileReader)
		mockPR := new(MockPRReader)

		svc := triage.NewPRService(triage.PRServiceDeps{
			PRReader:   mockPR,
			FileReader: mockFile,
		})
		server := createTestServer(svc)

		input := GetCodeContextInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			File:      "main.go",
			StartLine: 20,
			EndLine:   10, // end < start
		}

		result, _, err := server.handleGetCodeContext(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns error for file not found", func(t *testing.T) {
		mockFile := new(MockFileReader)
		mockPR := new(MockPRReader)

		prMeta := &domain.PRMetadata{HeadSHA: "abc123"}
		mockPR.On("GetPRMetadata", ctx, "owner", "repo", 42).Return(prMeta, nil)
		mockFile.On("ReadFileLines", ctx, "nonexistent.go", "abc123", 1, 10, 3).Return(nil, triage.ErrFileNotFound)

		svc := triage.NewPRService(triage.PRServiceDeps{
			PRReader:   mockPR,
			FileReader: mockFile,
		})
		server := createTestServer(svc)

		input := GetCodeContextInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			File:      "nonexistent.go",
			StartLine: 1,
			EndLine:   10,
		}

		result, _, err := server.handleGetCodeContext(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)

		mockPR.AssertExpectations(t)
		mockFile.AssertExpectations(t)
	})
}

func TestServer_handleGetDiffContext(t *testing.T) {
	ctx := context.Background()

	t.Run("returns diff context for file and lines", func(t *testing.T) {
		mockDiff := new(MockDiffReader)
		mockPR := new(MockPRReader)

		prMeta := &domain.PRMetadata{
			HeadSHA: "abc123",
			BaseRef: "main",
		}
		mockPR.On("GetPRMetadata", ctx, "owner", "repo", 42).Return(prMeta, nil)

		diffCtx := &domain.DiffContext{
			File:        "main.go",
			BaseBranch:  "main",
			TargetRef:   "abc123",
			HunkContent: "@@ -10,3 +10,4 @@\n-old\n+new",
			StartLine:   10,
			EndLine:     12,
		}
		mockDiff.On("GetDiffHunk", ctx, "main", "abc123", "main.go", 10, 12).Return(diffCtx, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			PRReader:   mockPR,
			DiffReader: mockDiff,
		})
		server := createTestServer(svc)

		input := GetDiffContextInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			File:      "main.go",
			StartLine: 10,
			EndLine:   12,
		}

		result, output, err := server.handleGetDiffContext(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Equal(t, "main.go", output.File)
		assert.Contains(t, output.HunkContent, "+new")

		mockPR.AssertExpectations(t)
		mockDiff.AssertExpectations(t)
	})

	t.Run("returns not implemented when PRService is nil", func(t *testing.T) {
		server := NewServer(ServerDeps{PRService: nil})

		input := GetDiffContextInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			File:      "main.go",
			StartLine: 10,
			EndLine:   12,
		}

		result, _, err := server.handleGetDiffContext(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("returns error for invalid line range", func(t *testing.T) {
		mockDiff := new(MockDiffReader)
		mockPR := new(MockPRReader)

		svc := triage.NewPRService(triage.PRServiceDeps{
			PRReader:   mockPR,
			DiffReader: mockDiff,
		})
		server := createTestServer(svc)

		input := GetDiffContextInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			File:      "main.go",
			StartLine: 0, // invalid
			EndLine:   10,
		}

		result, _, err := server.handleGetDiffContext(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

// =============================================================================
// M3 Write Handler Tests
// =============================================================================

func TestServer_handleGetThread(t *testing.T) {
	ctx := context.Background()

	t.Run("returns thread history for comment", func(t *testing.T) {
		mockComment := new(MockCommentReader)

		history := []domain.ThreadComment{
			{
				Author:    "bot",
				Body:      "Initial finding",
				CreatedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
				IsReply:   false,
			},
			{
				Author:    "developer",
				Body:      "I'll fix this",
				CreatedAt: time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
				IsReply:   true,
			},
		}
		mockComment.On("GetThreadHistory", ctx, "owner", "repo", int64(12345)).Return(history, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader: mockComment,
		})
		server := createTestServer(svc)

		input := GetThreadInput{
			Owner:     "owner",
			Repo:      "repo",
			CommentID: 12345,
		}

		result, output, err := server.handleGetThread(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Equal(t, 2, output.Total)
		assert.Len(t, output.Comments, 2)
		assert.Equal(t, "bot", output.Comments[0].Author)

		mockComment.AssertExpectations(t)
	})

	t.Run("returns not implemented when PRService is nil", func(t *testing.T) {
		server := NewServer(ServerDeps{PRService: nil})

		input := GetThreadInput{
			Owner:     "owner",
			Repo:      "repo",
			CommentID: 12345,
		}

		result, _, err := server.handleGetThread(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("includes thread ID when PR number provided", func(t *testing.T) {
		mockComment := new(MockCommentReader)
		mockManager := new(MockReviewManager)

		history := []domain.ThreadComment{
			{Author: "bot", Body: "Finding", IsReply: false},
		}
		mockComment.On("GetThreadHistory", ctx, "owner", "repo", int64(12345)).Return(history, nil)

		threadInfo := &triage.ThreadInfo{
			ID:         "PRRT_kwDOAbc123",
			IsResolved: false,
		}
		mockManager.On("FindThreadForComment", ctx, "owner", "repo", 42, int64(12345)).Return(threadInfo, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader: mockComment,
			ReviewManager: mockManager,
		})
		server := createTestServer(svc)

		input := GetThreadInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			CommentID: 12345,
		}

		result, output, err := server.handleGetThread(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Equal(t, "PRRT_kwDOAbc123", output.ThreadID)
		assert.False(t, output.IsResolved)

		mockComment.AssertExpectations(t)
		mockManager.AssertExpectations(t)
	})
}

func TestServer_handleReplyToFinding(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully replies to finding", func(t *testing.T) {
		mockComment := new(MockCommentReader)
		mockWriter := new(MockCommentWriter)

		finding := &domain.PRFinding{
			CommentID: 12345,
			Path:      "main.go",
		}
		mockComment.On("GetPRComment", ctx, "owner", "repo", 42, int64(12345)).Return(finding, nil)
		mockWriter.On("ReplyToComment", ctx, "owner", "repo", 42, int64(12345), "My reply").Return(int64(99999), nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader: mockComment,
			CommentWriter: mockWriter,
		})
		server := createTestServer(svc)

		input := ReplyToFindingInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			FindingID: "12345",
			Body:      "My reply",
		}

		result, output, err := server.handleReplyToFinding(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.True(t, output.Success)
		assert.Equal(t, int64(99999), output.CommentID)

		mockComment.AssertExpectations(t)
		mockWriter.AssertExpectations(t)
	})

	t.Run("includes status tag in reply", func(t *testing.T) {
		mockComment := new(MockCommentReader)
		mockWriter := new(MockCommentWriter)

		finding := &domain.PRFinding{
			CommentID: 12345,
			Path:      "main.go",
		}
		mockComment.On("GetPRComment", ctx, "owner", "repo", 42, int64(12345)).Return(finding, nil)
		mockWriter.On("ReplyToComment", ctx, "owner", "repo", 42, int64(12345), "**Status:** fixed\n\nFixed in latest commit").Return(int64(99999), nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader: mockComment,
			CommentWriter: mockWriter,
		})
		server := createTestServer(svc)

		status := "fixed"
		input := ReplyToFindingInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			FindingID: "12345",
			Body:      "Fixed in latest commit",
			Status:    &status,
		}

		result, output, err := server.handleReplyToFinding(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.True(t, output.Success)

		mockComment.AssertExpectations(t)
		mockWriter.AssertExpectations(t)
	})

	t.Run("returns not implemented when PRService is nil", func(t *testing.T) {
		server := NewServer(ServerDeps{PRService: nil})

		input := ReplyToFindingInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			FindingID: "12345",
			Body:      "reply",
		}

		result, output, err := server.handleReplyToFinding(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.False(t, output.Success)
	})

	t.Run("returns error when finding not found", func(t *testing.T) {
		mockComment := new(MockCommentReader)
		mockWriter := new(MockCommentWriter)

		mockComment.On("GetPRComment", ctx, "owner", "repo", 42, int64(999)).Return(nil, triage.ErrCommentNotFound)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader: mockComment,
			CommentWriter: mockWriter,
		})
		server := createTestServer(svc)

		input := ReplyToFindingInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			FindingID: "999",
			Body:      "reply",
		}

		result, output, err := server.handleReplyToFinding(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.False(t, output.Success)

		mockComment.AssertExpectations(t)
	})

	t.Run("returns error for invalid status tag", func(t *testing.T) {
		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader: new(MockCommentReader),
			CommentWriter: new(MockCommentWriter),
		})
		server := createTestServer(svc)

		invalidStatus := "invalid_status"
		input := ReplyToFindingInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			FindingID: "12345",
			Body:      "reply",
			Status:    &invalidStatus,
		}

		result, output, err := server.handleReplyToFinding(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.False(t, output.Success)
		assert.Equal(t, "Invalid status tag", output.Message)
		// Verify error message includes the invalid value and valid options
		assert.Contains(t, result.Content[0].(*mcpsdk.TextContent).Text, "invalid_status")
		assert.Contains(t, result.Content[0].(*mcpsdk.TextContent).Text, "acknowledged")
	})

	t.Run("normalizes status tag case and whitespace", func(t *testing.T) {
		mockComment := new(MockCommentReader)
		mockWriter := new(MockCommentWriter)

		finding := &domain.PRFinding{
			CommentID: 12345,
			Path:      "main.go",
		}
		mockComment.On("GetPRComment", ctx, "owner", "repo", 42, int64(12345)).Return(finding, nil)
		// Verify the normalized status is used (lowercase, trimmed)
		mockWriter.On("ReplyToComment", ctx, "owner", "repo", 42, int64(12345), "**Status:** acknowledged\n\nAcknowledged").Return(int64(99999), nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader: mockComment,
			CommentWriter: mockWriter,
		})
		server := createTestServer(svc)

		// Use mixed case with whitespace
		status := "  ACKNOWLEDGED  "
		input := ReplyToFindingInput{
			Owner:     "owner",
			Repo:      "repo",
			PRNumber:  42,
			FindingID: "12345",
			Body:      "Acknowledged",
			Status:    &status,
		}

		result, output, err := server.handleReplyToFinding(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.True(t, output.Success)

		mockComment.AssertExpectations(t)
		mockWriter.AssertExpectations(t)
	})
}

func TestServer_handlePostComment(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully posts comment at file and line", func(t *testing.T) {
		mockPR := new(MockPRReader)
		mockWriter := new(MockCommentWriter)

		prMeta := &domain.PRMetadata{HeadSHA: "abc123"}
		mockPR.On("GetPRMetadata", ctx, "owner", "repo", 42).Return(prMeta, nil)
		mockWriter.On("CreateComment", ctx, "owner", "repo", 42, "abc123", "main.go", 50, "New comment").Return(int64(77777), nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			PRReader:      mockPR,
			CommentWriter: mockWriter,
		})
		server := createTestServer(svc)

		input := PostCommentInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 42,
			File:     "main.go",
			Line:     50,
			Body:     "New comment",
		}

		result, output, err := server.handlePostComment(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.True(t, output.Success)
		assert.Equal(t, int64(77777), output.CommentID)

		mockPR.AssertExpectations(t)
		mockWriter.AssertExpectations(t)
	})

	t.Run("returns not implemented when PRService is nil", func(t *testing.T) {
		server := NewServer(ServerDeps{PRService: nil})

		input := PostCommentInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 42,
			File:     "main.go",
			Line:     50,
			Body:     "comment",
		}

		result, output, err := server.handlePostComment(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.False(t, output.Success)
	})

	t.Run("returns error for empty file path", func(t *testing.T) {
		mockPR := new(MockPRReader)
		mockWriter := new(MockCommentWriter)

		svc := triage.NewPRService(triage.PRServiceDeps{
			PRReader:      mockPR,
			CommentWriter: mockWriter,
		})
		server := createTestServer(svc)

		input := PostCommentInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 42,
			File:     "",
			Line:     50,
			Body:     "comment",
		}

		result, output, err := server.handlePostComment(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.False(t, output.Success)
	})

	t.Run("returns error for invalid line number", func(t *testing.T) {
		mockPR := new(MockPRReader)
		mockWriter := new(MockCommentWriter)

		svc := triage.NewPRService(triage.PRServiceDeps{
			PRReader:      mockPR,
			CommentWriter: mockWriter,
		})
		server := createTestServer(svc)

		input := PostCommentInput{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 42,
			File:     "main.go",
			Line:     0,
			Body:     "comment",
		}

		result, output, err := server.handlePostComment(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.False(t, output.Success)
	})
}

func TestServer_handleMarkResolved(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully resolves thread", func(t *testing.T) {
		mockManager := new(MockReviewManager)

		mockManager.On("ResolveThread", ctx, "owner", "repo", "PRRT_kwDOAbc123").Return(nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			ReviewManager: mockManager,
		})
		server := createTestServer(svc)

		input := MarkResolvedInput{
			Owner:    "owner",
			Repo:     "repo",
			ThreadID: "PRRT_kwDOAbc123",
			Resolved: true,
		}

		result, output, err := server.handleMarkResolved(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.True(t, output.Success)
		assert.True(t, output.Resolved)

		mockManager.AssertExpectations(t)
	})

	t.Run("successfully unresolves thread", func(t *testing.T) {
		mockManager := new(MockReviewManager)

		mockManager.On("UnresolveThread", ctx, "owner", "repo", "PRRT_kwDOAbc123").Return(nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			ReviewManager: mockManager,
		})
		server := createTestServer(svc)

		input := MarkResolvedInput{
			Owner:    "owner",
			Repo:     "repo",
			ThreadID: "PRRT_kwDOAbc123",
			Resolved: false,
		}

		result, output, err := server.handleMarkResolved(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.True(t, output.Success)
		assert.False(t, output.Resolved)

		mockManager.AssertExpectations(t)
	})

	t.Run("returns not implemented when PRService is nil", func(t *testing.T) {
		server := NewServer(ServerDeps{PRService: nil})

		input := MarkResolvedInput{
			Owner:    "owner",
			Repo:     "repo",
			ThreadID: "PRRT_kwDOAbc123",
			Resolved: true,
		}

		result, output, err := server.handleMarkResolved(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.False(t, output.Success)
	})

	t.Run("returns error for missing owner", func(t *testing.T) {
		mockManager := new(MockReviewManager)

		svc := triage.NewPRService(triage.PRServiceDeps{
			ReviewManager: mockManager,
		})
		server := createTestServer(svc)

		input := MarkResolvedInput{
			Owner:    "",
			Repo:     "repo",
			ThreadID: "PRRT_kwDOAbc123",
			Resolved: true,
		}

		result, output, err := server.handleMarkResolved(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.False(t, output.Success)
	})

	t.Run("returns error for invalid thread ID format", func(t *testing.T) {
		mockManager := new(MockReviewManager)

		svc := triage.NewPRService(triage.PRServiceDeps{
			ReviewManager: mockManager,
		})
		server := createTestServer(svc)

		input := MarkResolvedInput{
			Owner:    "owner",
			Repo:     "repo",
			ThreadID: "invalid-id",
			Resolved: true,
		}

		result, output, err := server.handleMarkResolved(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.False(t, output.Success)
	})

	t.Run("returns error when thread not found", func(t *testing.T) {
		mockManager := new(MockReviewManager)

		mockManager.On("ResolveThread", ctx, "owner", "repo", "PRRT_notfound").Return(triage.ErrThreadNotFound)

		svc := triage.NewPRService(triage.PRServiceDeps{
			ReviewManager: mockManager,
		})
		server := createTestServer(svc)

		input := MarkResolvedInput{
			Owner:    "owner",
			Repo:     "repo",
			ThreadID: "PRRT_notfound",
			Resolved: true,
		}

		result, output, err := server.handleMarkResolved(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.False(t, output.Success)

		mockManager.AssertExpectations(t)
	})
}

func TestServer_handleRequestRereview(t *testing.T) {
	ctx := context.Background()

	t.Run("dismisses stale reviews and requests new review", func(t *testing.T) {
		mockManager := new(MockReviewManager)

		reviews := []triage.Review{
			{ID: 1, User: "code-reviewer-bot", UserType: "Bot", State: "APPROVED"},
			{ID: 2, User: "human", UserType: "User", State: "CHANGES_REQUESTED"},
		}
		mockManager.On("ListReviews", ctx, "owner", "repo", 42).Return(reviews, nil)
		mockManager.On("DismissReview", ctx, "owner", "repo", 42, int64(1), "Dismissed stale bot review to allow fresh re-review").Return(nil)
		mockManager.On("RequestReviewers", ctx, "owner", "repo", 42, []string{"reviewer1"}, []string(nil)).Return(nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			ReviewManager: mockManager,
		})
		server := createTestServer(svc)

		input := RequestRereviewInput{
			Owner:        "owner",
			Repo:         "repo",
			PRNumber:     42,
			DismissStale: true,
			Reviewers:    []string{"reviewer1"},
		}

		result, output, err := server.handleRequestRereview(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.True(t, output.Success)
		assert.Equal(t, 1, output.ReviewsDismissed)
		assert.Equal(t, 1, output.ReviewsRequested)

		mockManager.AssertExpectations(t)
	})

	t.Run("returns not implemented when PRService is nil", func(t *testing.T) {
		server := NewServer(ServerDeps{PRService: nil})

		input := RequestRereviewInput{
			Owner:        "owner",
			Repo:         "repo",
			PRNumber:     42,
			DismissStale: true,
		}

		result, output, err := server.handleRequestRereview(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.False(t, output.Success)
	})

	t.Run("only requests review without dismissing", func(t *testing.T) {
		mockManager := new(MockReviewManager)

		mockManager.On("RequestReviewers", ctx, "owner", "repo", 42, []string{"reviewer1"}, []string{"team-a"}).Return(nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			ReviewManager: mockManager,
		})
		server := createTestServer(svc)

		input := RequestRereviewInput{
			Owner:         "owner",
			Repo:          "repo",
			PRNumber:      42,
			DismissStale:  false,
			Reviewers:     []string{"reviewer1"},
			TeamReviewers: []string{"team-a"},
		}

		result, output, err := server.handleRequestRereview(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.True(t, output.Success)
		assert.Equal(t, 0, output.ReviewsDismissed)
		assert.Equal(t, 2, output.ReviewsRequested)

		mockManager.AssertExpectations(t)
	})

	t.Run("handles no reviews to dismiss", func(t *testing.T) {
		mockManager := new(MockReviewManager)

		reviews := []triage.Review{
			{ID: 1, User: "human", UserType: "User", State: "APPROVED"}, // Not a bot
		}
		mockManager.On("ListReviews", ctx, "owner", "repo", 42).Return(reviews, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			ReviewManager: mockManager,
		})
		server := createTestServer(svc)

		input := RequestRereviewInput{
			Owner:        "owner",
			Repo:         "repo",
			PRNumber:     42,
			DismissStale: true,
		}

		result, output, err := server.handleRequestRereview(ctx, nil, input)
		require.NoError(t, err)
		assert.False(t, result.IsError)
		assert.True(t, output.Success)
		assert.Equal(t, 0, output.ReviewsDismissed)
		assert.Equal(t, 0, output.ReviewsRequested)

		mockManager.AssertExpectations(t)
	})

	t.Run("returns error when reviewer not found", func(t *testing.T) {
		mockManager := new(MockReviewManager)

		mockManager.On("RequestReviewers", ctx, "owner", "repo", 42, []string{"nonexistent"}, []string(nil)).Return(triage.ErrUserNotFound)

		svc := triage.NewPRService(triage.PRServiceDeps{
			ReviewManager: mockManager,
		})
		server := createTestServer(svc)

		input := RequestRereviewInput{
			Owner:        "owner",
			Repo:         "repo",
			PRNumber:     42,
			DismissStale: false,
			Reviewers:    []string{"nonexistent"},
		}

		result, output, err := server.handleRequestRereview(ctx, nil, input)
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.False(t, output.Success)

		mockManager.AssertExpectations(t)
	})
}
