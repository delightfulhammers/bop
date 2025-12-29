package triage

import (
	"testing"

	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultSuggestionExtractor_ExtractFromAnnotation(t *testing.T) {
	extractor := NewSuggestionExtractor()

	tests := []struct {
		name           string
		annotation     *domain.Annotation
		wantNewCode    string
		wantSource     string
		wantFile       string
		wantErr        error
		wantErrContain string
	}{
		{
			name:    "nil annotation returns ErrNoSuggestion",
			wantErr: ErrNoSuggestion,
		},
		{
			name: "annotation with github suggestion block in RawDetails",
			annotation: &domain.Annotation{
				Path:       "src/main.go",
				Message:    "Variable should be renamed",
				RawDetails: "Consider this change:\n```suggestion\nfunc renamed() {}\n```",
			},
			wantNewCode: "func renamed() {}",
			wantSource:  "annotation",
			wantFile:    "src/main.go",
		},
		{
			name: "annotation with generic code block in RawDetails",
			annotation: &domain.Annotation{
				Path:       "src/util.go",
				Message:    "Use constants",
				RawDetails: "Suggested fix:\n```go\nconst MaxRetries = 3\n```",
			},
			wantNewCode: "const MaxRetries = 3",
			wantSource:  "annotation",
			wantFile:    "src/util.go",
		},
		{
			name: "annotation with suggestion in Message (fallback)",
			annotation: &domain.Annotation{
				Path:       "src/handler.go",
				Message:    "Handle error properly:\n```suggestion\nif err != nil { return err }\n```",
				RawDetails: "",
			},
			wantNewCode: "if err != nil { return err }",
			wantSource:  "annotation",
			wantFile:    "src/handler.go",
		},
		{
			name: "annotation without any code block",
			annotation: &domain.Annotation{
				Path:       "src/main.go",
				Message:    "This variable name is unclear",
				RawDetails: "Consider using a more descriptive name",
			},
			wantErr: ErrNoSuggestion,
		},
		{
			name: "annotation with empty Message and RawDetails",
			annotation: &domain.Annotation{
				Path: "src/main.go",
			},
			wantErr: ErrNoSuggestion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestion, err := extractor.ExtractFromAnnotation(tt.annotation)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, suggestion)
			assert.Equal(t, tt.wantNewCode, suggestion.NewCode)
			assert.Equal(t, tt.wantSource, suggestion.Source)
			assert.Equal(t, tt.wantFile, suggestion.File)
		})
	}
}

func TestDefaultSuggestionExtractor_ExtractFromComment(t *testing.T) {
	extractor := NewSuggestionExtractor()

	tests := []struct {
		name        string
		finding     *domain.PRFinding
		wantNewCode string
		wantSource  string
		wantFile    string
		wantErr     error
	}{
		{
			name:    "nil finding returns ErrNoSuggestion",
			wantErr: ErrNoSuggestion,
		},
		{
			name: "comment with github suggestion block",
			finding: &domain.PRFinding{
				Path: "pkg/service.go",
				Body: "**Severity: medium**\n\nConsider using early return:\n```suggestion\nif err != nil {\n    return nil, err\n}\n```",
			},
			wantNewCode: "if err != nil {\n    return nil, err\n}",
			wantSource:  "comment",
			wantFile:    "pkg/service.go",
		},
		{
			name: "comment with generic code block",
			finding: &domain.PRFinding{
				Path: "internal/handler.go",
				Body: "Use context:\n```go\nctx := context.Background()\n```",
			},
			wantNewCode: "ctx := context.Background()",
			wantSource:  "comment",
			wantFile:    "internal/handler.go",
		},
		{
			name: "comment without code block",
			finding: &domain.PRFinding{
				Path: "main.go",
				Body: "**Severity: low**\n\nThis function is too long.",
			},
			wantErr: ErrNoSuggestion,
		},
		{
			name: "comment with empty body",
			finding: &domain.PRFinding{
				Path: "main.go",
				Body: "",
			},
			wantErr: ErrNoSuggestion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestion, err := extractor.ExtractFromComment(tt.finding)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, suggestion)
			assert.Equal(t, tt.wantNewCode, suggestion.NewCode)
			assert.Equal(t, tt.wantSource, suggestion.Source)
			assert.Equal(t, tt.wantFile, suggestion.File)
		})
	}
}

func TestExtractExplanation(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "text before code block",
			text: "This is the explanation.\n```suggestion\ncode here\n```",
			want: "This is the explanation.",
		},
		{
			name: "strips metadata prefixes",
			text: "**Severity: high**\n**Category: security**\nCR_FP:abc123\n\nActual explanation here.\n```suggestion\ncode\n```",
			want: "Actual explanation here.",
		},
		{
			name: "text without code block",
			text: "**Severity: high**\n\nJust a plain comment with explanation.",
			want: "Just a plain comment with explanation.",
		},
		{
			name: "empty text",
			text: "",
			want: "",
		},
		{
			name: "only metadata",
			text: "**Severity: low**\n**Category: style**\n```suggestion\ncode\n```",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractExplanation(tt.text)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestExtractSuggestionCode(t *testing.T) {
	extractor := NewSuggestionExtractor()

	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "github suggestion block",
			text: "```suggestion\nreturn nil\n```",
			want: "return nil",
		},
		{
			name: "generic code block with language",
			text: "```go\nfunc main() {}\n```",
			want: "func main() {}",
		},
		{
			name: "generic code block without language",
			text: "```\nplain code\n```",
			want: "plain code",
		},
		{
			name: "prefers suggestion over generic",
			text: "```go\ngeneric\n```\n```suggestion\npreferred\n```",
			want: "preferred",
		},
		{
			name: "multiline code",
			text: "```suggestion\nline1\nline2\nline3\n```",
			want: "line1\nline2\nline3",
		},
		{
			name: "no code block",
			text: "Just plain text",
			want: "",
		},
		{
			name: "empty",
			text: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractor.extractSuggestionCode(tt.text)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestCleanMetadata(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "removes severity prefix",
			text: "**Severity: high**\nActual content",
			want: "Actual content",
		},
		{
			name: "removes category prefix",
			text: "**Category: security**\nActual content",
			want: "Actual content",
		},
		{
			name: "removes fingerprint prefix",
			text: "CR_FP:abc123\nActual content",
			want: "Actual content",
		},
		{
			name: "removes all metadata",
			text: "**Severity: high**\n**Category: security**\nCR_FP:abc123\nActual content here",
			want: "Actual content here",
		},
		{
			name: "preserves normal text",
			text: "This is normal text\nwith multiple lines",
			want: "This is normal text\nwith multiple lines",
		},
		{
			name: "empty text",
			text: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanMetadata(tt.text)
			assert.Equal(t, tt.want, result)
		})
	}
}
