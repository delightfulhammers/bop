package triage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/bkyoung/code-reviewer/internal/usecase/triage"
)

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

func TestPRService_ListAnnotations(t *testing.T) {
	ctx := context.Background()

	t.Run("returns annotations for PR head commit", func(t *testing.T) {
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
			{ID: 1001, Name: "code-reviewer", Status: "completed", AnnotationCount: 2, HeadSHA: "abc123"},
		}
		mockAnn.On("ListCheckRuns", ctx, "owner", "repo", "abc123", (*string)(nil)).Return(checkRuns, nil)

		annotations := []domain.Annotation{
			{CheckRunID: 1001, Index: 0, Path: "main.go", StartLine: 10, Level: domain.AnnotationLevelWarning},
			{CheckRunID: 1001, Index: 1, Path: "util.go", StartLine: 20, Level: domain.AnnotationLevelFailure},
		}
		mockAnn.On("GetAnnotations", ctx, "owner", "repo", int64(1001)).Return(annotations, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			AnnotationReader: mockAnn,
			PRReader:         mockPR,
		})

		result, err := svc.ListAnnotations(ctx, "owner", "repo", 42, nil, nil)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, "main.go", result[0].Path)
		assert.Equal(t, "util.go", result[1].Path)

		mockPR.AssertExpectations(t)
		mockAnn.AssertExpectations(t)
	})

	t.Run("filters by check name", func(t *testing.T) {
		mockAnn := new(MockAnnotationReader)
		mockPR := new(MockPRReader)

		prMeta := &domain.PRMetadata{Owner: "owner", Repo: "repo", Number: 42, HeadSHA: "abc123"}
		mockPR.On("GetPRMetadata", ctx, "owner", "repo", 42).Return(prMeta, nil)

		checkName := "code-reviewer"
		checkRuns := []domain.CheckRunSummary{
			{ID: 1001, Name: "code-reviewer", Status: "completed", AnnotationCount: 1},
		}
		mockAnn.On("ListCheckRuns", ctx, "owner", "repo", "abc123", &checkName).Return(checkRuns, nil)

		annotations := []domain.Annotation{
			{CheckRunID: 1001, Index: 0, Path: "main.go", Level: domain.AnnotationLevelWarning},
		}
		mockAnn.On("GetAnnotations", ctx, "owner", "repo", int64(1001)).Return(annotations, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			AnnotationReader: mockAnn,
			PRReader:         mockPR,
		})

		result, err := svc.ListAnnotations(ctx, "owner", "repo", 42, &checkName, nil)
		require.NoError(t, err)
		assert.Len(t, result, 1)
	})

	t.Run("filters by level", func(t *testing.T) {
		mockAnn := new(MockAnnotationReader)
		mockPR := new(MockPRReader)

		prMeta := &domain.PRMetadata{Owner: "owner", Repo: "repo", Number: 42, HeadSHA: "abc123"}
		mockPR.On("GetPRMetadata", ctx, "owner", "repo", 42).Return(prMeta, nil)

		checkRuns := []domain.CheckRunSummary{
			{ID: 1001, Name: "code-reviewer", AnnotationCount: 2},
		}
		mockAnn.On("ListCheckRuns", ctx, "owner", "repo", "abc123", (*string)(nil)).Return(checkRuns, nil)

		annotations := []domain.Annotation{
			{CheckRunID: 1001, Index: 0, Path: "main.go", Level: domain.AnnotationLevelWarning},
			{CheckRunID: 1001, Index: 1, Path: "util.go", Level: domain.AnnotationLevelFailure},
		}
		mockAnn.On("GetAnnotations", ctx, "owner", "repo", int64(1001)).Return(annotations, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			AnnotationReader: mockAnn,
			PRReader:         mockPR,
		})

		failureLevel := domain.AnnotationLevelFailure
		result, err := svc.ListAnnotations(ctx, "owner", "repo", 42, nil, &failureLevel)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "util.go", result[0].Path)
	})

	t.Run("returns empty for no check runs", func(t *testing.T) {
		mockAnn := new(MockAnnotationReader)
		mockPR := new(MockPRReader)

		prMeta := &domain.PRMetadata{Owner: "owner", Repo: "repo", Number: 42, HeadSHA: "abc123"}
		mockPR.On("GetPRMetadata", ctx, "owner", "repo", 42).Return(prMeta, nil)

		mockAnn.On("ListCheckRuns", ctx, "owner", "repo", "abc123", (*string)(nil)).Return([]domain.CheckRunSummary{}, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			AnnotationReader: mockAnn,
			PRReader:         mockPR,
		})

		result, err := svc.ListAnnotations(ctx, "owner", "repo", 42, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestPRService_GetAnnotation(t *testing.T) {
	ctx := context.Background()

	t.Run("returns annotation by check run and index", func(t *testing.T) {
		mockAnn := new(MockAnnotationReader)

		annotation := &domain.Annotation{
			CheckRunID: 1001,
			Index:      0,
			Path:       "main.go",
			StartLine:  10,
			Level:      domain.AnnotationLevelWarning,
			Message:    "Unused variable",
		}
		mockAnn.On("GetAnnotation", ctx, "owner", "repo", int64(1001), 0).Return(annotation, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			AnnotationReader: mockAnn,
		})

		result, err := svc.GetAnnotation(ctx, "owner", "repo", 1001, 0)
		require.NoError(t, err)
		assert.Equal(t, "main.go", result.Path)
		assert.Equal(t, "Unused variable", result.Message)
	})

	t.Run("returns error for invalid index", func(t *testing.T) {
		mockAnn := new(MockAnnotationReader)
		mockAnn.On("GetAnnotation", ctx, "owner", "repo", int64(1001), 99).Return(nil, triage.ErrAnnotationNotFound)

		svc := triage.NewPRService(triage.PRServiceDeps{
			AnnotationReader: mockAnn,
		})

		_, err := svc.GetAnnotation(ctx, "owner", "repo", 1001, 99)
		require.Error(t, err)
		assert.ErrorIs(t, err, triage.ErrAnnotationNotFound)
	})
}

func TestPRService_GetCodeContext(t *testing.T) {
	ctx := context.Background()

	t.Run("returns code context for file and lines", func(t *testing.T) {
		mockFile := new(MockFileReader)
		mockPR := new(MockPRReader)

		prMeta := &domain.PRMetadata{Owner: "owner", Repo: "repo", Number: 42, HeadSHA: "abc123"}
		mockPR.On("GetPRMetadata", ctx, "owner", "repo", 42).Return(prMeta, nil)

		codeCtx := &domain.CodeContext{
			File:      "main.go",
			Ref:       "abc123",
			StartLine: 8,
			EndLine:   12,
			Content:   "func main() {\n\tprintln(\"hello\")\n}",
		}
		mockFile.On("ReadFileLines", ctx, "main.go", "abc123", 10, 10, 2).Return(codeCtx, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			PRReader:   mockPR,
			FileReader: mockFile,
		})

		result, err := svc.GetCodeContext(ctx, "owner", "repo", 42, "main.go", 10, 10, 2)
		require.NoError(t, err)
		assert.Equal(t, "main.go", result.File)
		assert.Contains(t, result.Content, "println")
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		mockFile := new(MockFileReader)
		mockPR := new(MockPRReader)

		prMeta := &domain.PRMetadata{Owner: "owner", Repo: "repo", Number: 42, HeadSHA: "abc123"}
		mockPR.On("GetPRMetadata", ctx, "owner", "repo", 42).Return(prMeta, nil)

		mockFile.On("ReadFileLines", ctx, "nonexistent.go", "abc123", 1, 10, 0).Return(nil, triage.ErrFileNotFound)

		svc := triage.NewPRService(triage.PRServiceDeps{
			PRReader:   mockPR,
			FileReader: mockFile,
		})

		_, err := svc.GetCodeContext(ctx, "owner", "repo", 42, "nonexistent.go", 1, 10, 0)
		require.Error(t, err)
		assert.ErrorIs(t, err, triage.ErrFileNotFound)
	})
}

func TestPRService_GetDiffContext(t *testing.T) {
	ctx := context.Background()

	t.Run("returns diff context for file and lines", func(t *testing.T) {
		mockDiff := new(MockDiffReader)
		mockPR := new(MockPRReader)

		prMeta := &domain.PRMetadata{
			Owner:   "owner",
			Repo:    "repo",
			Number:  42,
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

		result, err := svc.GetDiffContext(ctx, "owner", "repo", 42, "main.go", 10, 12)
		require.NoError(t, err)
		assert.Equal(t, "main.go", result.File)
		assert.Contains(t, result.HunkContent, "+new")
		assert.True(t, result.HasChanges())
	})
}

// Test helper to verify we don't break when PR metadata lookup fails
func TestPRService_HandlesErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("propagates PR metadata error", func(t *testing.T) {
		mockPR := new(MockPRReader)
		mockPR.On("GetPRMetadata", ctx, "owner", "repo", 42).Return(nil, errors.New("API error"))
		mockAnno := new(MockAnnotationReader)

		svc := triage.NewPRService(triage.PRServiceDeps{
			PRReader:         mockPR,
			AnnotationReader: mockAnno,
		})

		_, err := svc.ListAnnotations(ctx, "owner", "repo", 42, nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "API error")
	})
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

func TestPRService_GetSuggestion(t *testing.T) {
	ctx := context.Background()

	t.Run("returns suggestion from comment", func(t *testing.T) {
		mockComment := new(MockCommentReader)
		mockExtractor := new(MockSuggestionExtractor)

		finding := &domain.PRFinding{
			CommentID: 123,
			Path:      "main.go",
			Body:      "Fix this:\n```suggestion\nreturn nil\n```",
		}
		mockComment.On("GetPRComment", ctx, "owner", "repo", 42, int64(123)).Return(finding, nil)

		suggestion := &domain.Suggestion{
			File:    "main.go",
			NewCode: "return nil",
			Source:  "comment",
		}
		mockExtractor.On("ExtractFromComment", finding).Return(suggestion, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader:       mockComment,
			SuggestionExtractor: mockExtractor,
		})

		result, err := svc.GetSuggestion(ctx, "owner", "repo", 42, "123")
		require.NoError(t, err)
		assert.Equal(t, "main.go", result.File)
		assert.Equal(t, "return nil", result.NewCode)
		assert.Equal(t, "comment", result.Source)

		mockComment.AssertExpectations(t)
		mockExtractor.AssertExpectations(t)
	})

	t.Run("returns suggestion from fingerprint-based lookup", func(t *testing.T) {
		mockComment := new(MockCommentReader)
		mockExtractor := new(MockSuggestionExtractor)

		finding := &domain.PRFinding{
			CommentID:   456,
			Fingerprint: "abc123",
			Path:        "util.go",
			Body:        "```suggestion\nfunc fixed() {}\n```",
		}
		mockComment.On("GetPRCommentByFingerprint", ctx, "owner", "repo", 42, "abc123").Return(finding, nil)

		suggestion := &domain.Suggestion{
			File:    "util.go",
			NewCode: "func fixed() {}",
			Source:  "comment",
		}
		mockExtractor.On("ExtractFromComment", finding).Return(suggestion, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader:       mockComment,
			SuggestionExtractor: mockExtractor,
		})

		result, err := svc.GetSuggestion(ctx, "owner", "repo", 42, "CR_FP:abc123")
		require.NoError(t, err)
		assert.Equal(t, "util.go", result.File)
		assert.Equal(t, "comment", result.Source)
	})

	t.Run("falls back to annotation when comment has no suggestion", func(t *testing.T) {
		mockComment := new(MockCommentReader)
		mockAnnotation := new(MockAnnotationReader)
		mockExtractor := new(MockSuggestionExtractor)

		// Comment found but no suggestion block
		finding := &domain.PRFinding{
			CommentID: 123,
			Path:      "main.go",
			Body:      "This is just a comment without suggestion",
		}
		mockComment.On("GetPRComment", ctx, "owner", "repo", 42, int64(123)).Return(finding, nil)
		mockExtractor.On("ExtractFromComment", finding).Return(nil, triage.ErrNoSuggestion)

		// Since findingID "123" doesn't parse as annotation ID format "checkRunID:index",
		// the annotation lookup won't be tried
		// The result should be ErrNoSuggestion

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader:       mockComment,
			AnnotationReader:    mockAnnotation,
			SuggestionExtractor: mockExtractor,
		})

		_, err := svc.GetSuggestion(ctx, "owner", "repo", 42, "123")
		require.Error(t, err)
		assert.ErrorIs(t, err, triage.ErrNoSuggestion)
	})

	t.Run("returns suggestion from annotation", func(t *testing.T) {
		mockAnnotation := new(MockAnnotationReader)
		mockExtractor := new(MockSuggestionExtractor)

		annotation := &domain.Annotation{
			CheckRunID: 1001,
			Index:      0,
			Path:       "handler.go",
			Message:    "Fix:\n```suggestion\nif err != nil { return err }\n```",
		}
		mockAnnotation.On("GetAnnotation", ctx, "owner", "repo", int64(1001), 0).Return(annotation, nil)

		suggestion := &domain.Suggestion{
			File:    "handler.go",
			NewCode: "if err != nil { return err }",
			Source:  "annotation",
		}
		mockExtractor.On("ExtractFromAnnotation", annotation).Return(suggestion, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			AnnotationReader:    mockAnnotation,
			SuggestionExtractor: mockExtractor,
		})

		result, err := svc.GetSuggestion(ctx, "owner", "repo", 42, "1001:0")
		require.NoError(t, err)
		assert.Equal(t, "handler.go", result.File)
		assert.Equal(t, "annotation", result.Source)
	})

	t.Run("returns ErrNotImplemented when extractor is nil", func(t *testing.T) {
		svc := triage.NewPRService(triage.PRServiceDeps{})

		_, err := svc.GetSuggestion(ctx, "owner", "repo", 42, "123")
		require.Error(t, err)
		assert.ErrorIs(t, err, triage.ErrNotImplemented)
	})

	t.Run("returns ErrNoSuggestion when no source has suggestion", func(t *testing.T) {
		mockExtractor := new(MockSuggestionExtractor)

		// No CommentReader or AnnotationReader, so falls through to ErrNoSuggestion
		svc := triage.NewPRService(triage.PRServiceDeps{
			SuggestionExtractor: mockExtractor,
		})

		_, err := svc.GetSuggestion(ctx, "owner", "repo", 42, "invalid-id")
		require.Error(t, err)
		assert.ErrorIs(t, err, triage.ErrNoSuggestion)
	})

	t.Run("rejects annotation IDs with trailing content", func(t *testing.T) {
		// This test verifies that "1001:0xyz" is not parsed as a valid annotation ID
		// The parseAnnotationID function should reject IDs with trailing content
		mockAnnotation := new(MockAnnotationReader)
		mockExtractor := new(MockSuggestionExtractor)

		// If parsing worked incorrectly, this would be called with 1001, 0
		// Since parsing should fail, this should NOT be called
		// (we set up no expectations, so if it's called the test will fail)

		svc := triage.NewPRService(triage.PRServiceDeps{
			AnnotationReader:    mockAnnotation,
			SuggestionExtractor: mockExtractor,
		})

		// IDs with trailing content should be rejected
		malformedIDs := []string{
			"1001:0xyz",    // trailing letters
			"1001:0 extra", // trailing with space
			"1001:0:extra", // extra colon
			"1001:0\t",     // trailing tab
		}

		for _, id := range malformedIDs {
			_, err := svc.GetSuggestion(ctx, "owner", "repo", 42, id)
			require.Error(t, err, "expected error for malformed ID: %s", id)
			assert.ErrorIs(t, err, triage.ErrNoSuggestion, "malformed ID %s should not be parsed as annotation", id)
		}

		// Verify GetAnnotation was never called (no expectations were set)
		mockAnnotation.AssertExpectations(t)
	})
}

// =============================================================================
// Write Operation Mocks
// =============================================================================

// MockCommentWriter implements triage.CommentWriter for testing.
type MockCommentWriter struct {
	mock.Mock
}

func (m *MockCommentWriter) ReplyToComment(ctx context.Context, owner, repo string, prNumber int, replyTo int64, body string) (int64, error) {
	args := m.Called(ctx, owner, repo, prNumber, replyTo, body)
	// Safe type assertion to avoid panic if Return not set
	id, _ := args.Get(0).(int64)
	return id, args.Error(1)
}

func (m *MockCommentWriter) CreateComment(ctx context.Context, owner, repo string, prNumber int, commitSHA, path string, line int, body string) (int64, error) {
	args := m.Called(ctx, owner, repo, prNumber, commitSHA, path, line, body)
	// Safe type assertion to avoid panic if Return not set
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
// Write Operation Tests
// =============================================================================

func TestPRService_ReplyToFinding(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully replies to finding by comment ID", func(t *testing.T) {
		mockComment := new(MockCommentReader)
		mockWriter := new(MockCommentWriter)

		finding := &domain.PRFinding{
			CommentID: 12345,
			Path:      "main.go",
			Body:      "Test finding",
		}

		mockComment.On("GetPRComment", ctx, "owner", "repo", 42, int64(12345)).
			Return(finding, nil)
		mockWriter.On("ReplyToComment", ctx, "owner", "repo", 42, int64(12345), "My reply").
			Return(int64(99999), nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader: mockComment,
			CommentWriter: mockWriter,
		})

		replyID, err := svc.ReplyToFinding(ctx, "owner", "repo", 42, "12345", "My reply")
		require.NoError(t, err)
		assert.Equal(t, int64(99999), replyID)

		mockComment.AssertExpectations(t)
		mockWriter.AssertExpectations(t)
	})

	t.Run("returns ErrNotImplemented when writer is nil", func(t *testing.T) {
		svc := triage.NewPRService(triage.PRServiceDeps{})

		_, err := svc.ReplyToFinding(ctx, "owner", "repo", 42, "123", "reply")
		require.Error(t, err)
		assert.ErrorIs(t, err, triage.ErrNotImplemented)
	})

	t.Run("returns ErrNotImplemented when reader is nil", func(t *testing.T) {
		mockWriter := new(MockCommentWriter)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentWriter: mockWriter,
		})

		_, err := svc.ReplyToFinding(ctx, "owner", "repo", 42, "123", "reply")
		require.Error(t, err)
		assert.ErrorIs(t, err, triage.ErrNotImplemented)
	})

	t.Run("returns error when finding not found", func(t *testing.T) {
		mockComment := new(MockCommentReader)
		mockWriter := new(MockCommentWriter)

		mockComment.On("GetPRComment", ctx, "owner", "repo", 42, int64(999)).
			Return(nil, triage.ErrCommentNotFound)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentReader: mockComment,
			CommentWriter: mockWriter,
		})

		_, err := svc.ReplyToFinding(ctx, "owner", "repo", 42, "999", "reply")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get finding")
	})
}

func TestPRService_PostComment(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully posts comment at file and line", func(t *testing.T) {
		mockPR := new(MockPRReader)
		mockWriter := new(MockCommentWriter)

		mockPR.On("GetPRMetadata", ctx, "owner", "repo", 42).
			Return(&domain.PRMetadata{HeadSHA: "abc123"}, nil)
		mockWriter.On("CreateComment", ctx, "owner", "repo", 42, "abc123", "main.go", 50, "New comment").
			Return(int64(77777), nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			PRReader:      mockPR,
			CommentWriter: mockWriter,
		})

		commentID, err := svc.PostComment(ctx, "owner", "repo", 42, "main.go", 50, "New comment")
		require.NoError(t, err)
		assert.Equal(t, int64(77777), commentID)

		mockPR.AssertExpectations(t)
		mockWriter.AssertExpectations(t)
	})

	t.Run("returns ErrNotImplemented when writer is nil", func(t *testing.T) {
		svc := triage.NewPRService(triage.PRServiceDeps{})

		_, err := svc.PostComment(ctx, "owner", "repo", 42, "main.go", 50, "comment")
		require.Error(t, err)
		assert.ErrorIs(t, err, triage.ErrNotImplemented)
	})

	t.Run("returns ErrNotImplemented when PRReader is nil", func(t *testing.T) {
		mockWriter := new(MockCommentWriter)

		svc := triage.NewPRService(triage.PRServiceDeps{
			CommentWriter: mockWriter,
		})

		_, err := svc.PostComment(ctx, "owner", "repo", 42, "main.go", 50, "comment")
		require.Error(t, err)
		assert.ErrorIs(t, err, triage.ErrNotImplemented)
	})
}

func TestPRService_ResolveThread(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully resolves thread", func(t *testing.T) {
		mockManager := new(MockReviewManager)
		mockManager.On("ResolveThread", ctx, "owner", "repo", "PRRT_test123").
			Return(nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			ReviewManager: mockManager,
		})

		err := svc.ResolveThread(ctx, "owner", "repo", "PRRT_test123")
		require.NoError(t, err)
		mockManager.AssertExpectations(t)
	})

	t.Run("returns ErrNotImplemented when manager is nil", func(t *testing.T) {
		svc := triage.NewPRService(triage.PRServiceDeps{})

		err := svc.ResolveThread(ctx, "owner", "repo", "PRRT_test")
		require.Error(t, err)
		assert.ErrorIs(t, err, triage.ErrNotImplemented)
	})
}

func TestPRService_UnresolveThread(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully unresolves thread", func(t *testing.T) {
		mockManager := new(MockReviewManager)
		mockManager.On("UnresolveThread", ctx, "owner", "repo", "PRRT_test123").
			Return(nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			ReviewManager: mockManager,
		})

		err := svc.UnresolveThread(ctx, "owner", "repo", "PRRT_test123")
		require.NoError(t, err)
		mockManager.AssertExpectations(t)
	})
}

func TestPRService_DismissReview(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully dismisses review", func(t *testing.T) {
		mockManager := new(MockReviewManager)
		mockManager.On("DismissReview", ctx, "owner", "repo", 42, int64(999), "Stale review").
			Return(nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			ReviewManager: mockManager,
		})

		err := svc.DismissReview(ctx, "owner", "repo", 42, 999, "Stale review")
		require.NoError(t, err)
		mockManager.AssertExpectations(t)
	})

	t.Run("returns ErrNotImplemented when manager is nil", func(t *testing.T) {
		svc := triage.NewPRService(triage.PRServiceDeps{})

		err := svc.DismissReview(ctx, "owner", "repo", 42, 999, "message")
		require.Error(t, err)
		assert.ErrorIs(t, err, triage.ErrNotImplemented)
	})
}

func TestPRService_RequestReview(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully requests review from users", func(t *testing.T) {
		mockManager := new(MockReviewManager)
		mockManager.On("RequestReviewers", ctx, "owner", "repo", 42, []string{"user1", "user2"}, []string(nil)).
			Return(nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			ReviewManager: mockManager,
		})

		err := svc.RequestReview(ctx, "owner", "repo", 42, []string{"user1", "user2"}, nil)
		require.NoError(t, err)
		mockManager.AssertExpectations(t)
	})

	t.Run("successfully requests review from teams", func(t *testing.T) {
		mockManager := new(MockReviewManager)
		mockManager.On("RequestReviewers", ctx, "owner", "repo", 42, []string(nil), []string{"team-a"}).
			Return(nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			ReviewManager: mockManager,
		})

		err := svc.RequestReview(ctx, "owner", "repo", 42, nil, []string{"team-a"})
		require.NoError(t, err)
		mockManager.AssertExpectations(t)
	})

	t.Run("returns ErrNotImplemented when manager is nil", func(t *testing.T) {
		svc := triage.NewPRService(triage.PRServiceDeps{})

		err := svc.RequestReview(ctx, "owner", "repo", 42, []string{"user1"}, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, triage.ErrNotImplemented)
	})
}

func TestPRService_ListReviews(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully lists reviews", func(t *testing.T) {
		mockManager := new(MockReviewManager)
		expectedReviews := []triage.Review{
			{ID: 1, User: "bot", UserType: "Bot", State: "APPROVED", SubmittedAt: "2024-01-01T00:00:00Z"},
			{ID: 2, User: "human", UserType: "User", State: "CHANGES_REQUESTED", SubmittedAt: "2024-01-02T00:00:00Z"},
		}
		mockManager.On("ListReviews", ctx, "owner", "repo", 42).
			Return(expectedReviews, nil)

		svc := triage.NewPRService(triage.PRServiceDeps{
			ReviewManager: mockManager,
		})

		reviews, err := svc.ListReviews(ctx, "owner", "repo", 42)
		require.NoError(t, err)
		assert.Len(t, reviews, 2)
		assert.Equal(t, "bot", reviews[0].User)
		assert.Equal(t, "Bot", reviews[0].UserType)
		mockManager.AssertExpectations(t)
	})

	t.Run("returns ErrNotImplemented when manager is nil", func(t *testing.T) {
		svc := triage.NewPRService(triage.PRServiceDeps{})

		_, err := svc.ListReviews(ctx, "owner", "repo", 42)
		require.Error(t, err)
		assert.ErrorIs(t, err, triage.ErrNotImplemented)
	})
}
