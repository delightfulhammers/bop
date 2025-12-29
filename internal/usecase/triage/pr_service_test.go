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
