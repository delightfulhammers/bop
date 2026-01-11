package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAnnotationLevel_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		level AnnotationLevel
		want  bool
	}{
		{"notice is valid", AnnotationLevelNotice, true},
		{"warning is valid", AnnotationLevelWarning, true},
		{"failure is valid", AnnotationLevelFailure, true},
		{"empty is invalid", AnnotationLevel(""), false},
		{"unknown is invalid", AnnotationLevel("info"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.level.IsValid()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAnnotation_LineRange(t *testing.T) {
	tests := []struct {
		name       string
		annotation Annotation
		wantStart  int
		wantEnd    int
	}{
		{
			name: "single line",
			annotation: Annotation{
				StartLine: 10,
				EndLine:   10,
			},
			wantStart: 10,
			wantEnd:   10,
		},
		{
			name: "multi-line",
			annotation: Annotation{
				StartLine: 10,
				EndLine:   20,
			},
			wantStart: 10,
			wantEnd:   20,
		},
		{
			name: "zero end line defaults to start",
			annotation: Annotation{
				StartLine: 10,
				EndLine:   0,
			},
			wantStart: 10,
			wantEnd:   10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := tt.annotation.LineRange()
			assert.Equal(t, tt.wantStart, start)
			assert.Equal(t, tt.wantEnd, end)
		})
	}
}

func TestCheckRunSummary_HasAnnotations(t *testing.T) {
	tests := []struct {
		name     string
		summary  CheckRunSummary
		expected bool
	}{
		{
			name: "has annotations",
			summary: CheckRunSummary{
				AnnotationCount: 5,
			},
			expected: true,
		},
		{
			name: "no annotations",
			summary: CheckRunSummary{
				AnnotationCount: 0,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.summary.HasAnnotations()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestCheckRunSummary_IsComplete(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected bool
	}{
		{"completed", "completed", true},
		{"in_progress", "in_progress", false},
		{"queued", "queued", false},
		{"pending", "pending", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := CheckRunSummary{Status: tt.status}
			assert.Equal(t, tt.expected, summary.IsComplete())
		})
	}
}

func TestCodeContext_LineCount(t *testing.T) {
	tests := []struct {
		name    string
		context CodeContext
		want    int
	}{
		{
			name: "single line",
			context: CodeContext{
				StartLine: 10,
				EndLine:   10,
			},
			want: 1,
		},
		{
			name: "multi-line",
			context: CodeContext{
				StartLine: 10,
				EndLine:   20,
			},
			want: 11,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.context.LineCount()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDiffContext_HasChanges(t *testing.T) {
	tests := []struct {
		name    string
		context DiffContext
		want    bool
	}{
		{
			name: "has hunk content",
			context: DiffContext{
				HunkContent: "@@ -1,5 +1,6 @@\n line1",
			},
			want: true,
		},
		{
			name: "empty hunk",
			context: DiffContext{
				HunkContent: "",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.context.HasChanges()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPRFinding_ResolveFindingID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantFP   string
		wantCID  int64
		wantIsFP bool
	}{
		{
			name:     "fingerprint format",
			input:    "CR_FP:abc123def456",
			wantFP:   "abc123def456",
			wantCID:  0,
			wantIsFP: true,
		},
		{
			name:     "comment ID format",
			input:    "12345678",
			wantFP:   "",
			wantCID:  12345678,
			wantIsFP: false,
		},
		{
			name:     "invalid number defaults to fingerprint",
			input:    "not-a-number",
			wantFP:   "not-a-number",
			wantCID:  0,
			wantIsFP: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp, commentID, isFP := ResolveFindingID(tt.input)
			assert.Equal(t, tt.wantFP, fp)
			assert.Equal(t, tt.wantCID, commentID)
			assert.Equal(t, tt.wantIsFP, isFP)
		})
	}
}

func TestPRFinding_ThreadStatus(t *testing.T) {
	tests := []struct {
		name     string
		finding  PRFinding
		expected string
	}{
		{
			name:     "resolved thread",
			finding:  PRFinding{IsResolved: true},
			expected: "resolved",
		},
		{
			name:     "unresolved with replies",
			finding:  PRFinding{IsResolved: false, ReplyCount: 3},
			expected: "active",
		},
		{
			name:     "unresolved no replies",
			finding:  PRFinding{IsResolved: false, ReplyCount: 0},
			expected: "pending",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.finding.ThreadStatus()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestAnnotation_MatchesFilters(t *testing.T) {
	annotation := Annotation{
		Path:    "main.go",
		Level:   AnnotationLevelWarning,
		Message: "unused variable",
	}

	tests := []struct {
		name        string
		levelFilter *AnnotationLevel
		want        bool
	}{
		{
			name:        "no filter matches",
			levelFilter: nil,
			want:        true,
		},
		{
			name:        "matching level filter",
			levelFilter: ptr(AnnotationLevelWarning),
			want:        true,
		},
		{
			name:        "non-matching level filter",
			levelFilter: ptr(AnnotationLevelFailure),
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := annotation.MatchesLevelFilter(tt.levelFilter)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCheckRunSummary_MatchesNameFilter(t *testing.T) {
	summary := CheckRunSummary{
		Name: "bop",
	}

	tests := []struct {
		name       string
		nameFilter *string
		want       bool
	}{
		{
			name:       "no filter matches",
			nameFilter: nil,
			want:       true,
		},
		{
			name:       "matching name filter",
			nameFilter: strPtr("bop"),
			want:       true,
		},
		{
			name:       "non-matching name filter",
			nameFilter: strPtr("other-check"),
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summary.MatchesNameFilter(tt.nameFilter)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Helper to create pointer to AnnotationLevel
func ptr(l AnnotationLevel) *AnnotationLevel {
	return &l
}

// Helper to create pointer to string
func strPtr(s string) *string {
	return &s
}

func TestPRMetadata(t *testing.T) {
	meta := PRMetadata{
		Owner:       "delightfulhammers",
		Repo:        "bop",
		Number:      42,
		HeadRef:     "feature-branch",
		HeadSHA:     "abc123",
		BaseRef:     "main",
		BaseSHA:     "def456",
		Title:       "Add new feature",
		Description: "This adds a cool feature",
		Author:      "delightfulhammers",
		State:       "open",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	assert.Equal(t, "delightfulhammers/bop", meta.FullName())
	assert.True(t, meta.IsOpen())
}

func TestPRFinding_IsOutOfDiff(t *testing.T) {
	tests := []struct {
		name        string
		finding     PRFinding
		wantOutDiff bool
	}{
		{
			name:        "default is false (in-diff)",
			finding:     PRFinding{CommentID: 123},
			wantOutDiff: false,
		},
		{
			name:        "explicitly set to true",
			finding:     PRFinding{CommentID: 456, IsOutOfDiff: true},
			wantOutDiff: true,
		},
		{
			name:        "explicitly set to false",
			finding:     PRFinding{CommentID: 789, IsOutOfDiff: false},
			wantOutDiff: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantOutDiff, tt.finding.IsOutOfDiff)
		})
	}
}
