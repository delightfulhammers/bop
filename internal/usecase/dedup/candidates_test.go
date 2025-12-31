package dedup

import (
	"testing"

	"github.com/bkyoung/code-reviewer/internal/domain"
)

func TestLinesOverlap(t *testing.T) {
	tests := []struct {
		name      string
		a1, a2    int // range A
		b1, b2    int // range B
		threshold int
		want      bool
	}{
		{
			name:      "direct overlap - same range",
			a1:        10,
			a2:        20,
			b1:        10,
			b2:        20,
			threshold: 5,
			want:      true,
		},
		{
			name:      "direct overlap - partial",
			a1:        10,
			a2:        20,
			b1:        15,
			b2:        25,
			threshold: 5,
			want:      true,
		},
		{
			name:      "direct overlap - contained",
			a1:        10,
			a2:        30,
			b1:        15,
			b2:        25,
			threshold: 5,
			want:      true,
		},
		{
			name:      "within threshold - a before b",
			a1:        10,
			a2:        15,
			b1:        20,
			b2:        25,
			threshold: 5,
			want:      true,
		},
		{
			name:      "within threshold - b before a",
			a1:        20,
			a2:        25,
			b1:        10,
			b2:        15,
			threshold: 5,
			want:      true,
		},
		{
			name:      "exactly at threshold",
			a1:        10,
			a2:        15,
			b1:        20,
			b2:        25,
			threshold: 5,
			want:      true,
		},
		{
			name:      "outside threshold",
			a1:        10,
			a2:        15,
			b1:        21,
			b2:        25,
			threshold: 5,
			want:      false,
		},
		{
			name:      "zero threshold - adjacent",
			a1:        10,
			a2:        15,
			b1:        16,
			b2:        20,
			threshold: 0,
			want:      false,
		},
		{
			name:      "zero threshold - overlapping",
			a1:        10,
			a2:        15,
			b1:        15,
			b2:        20,
			threshold: 0,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := linesOverlap(tt.a1, tt.a2, tt.b1, tt.b2, tt.threshold)
			if got != tt.want {
				t.Errorf("linesOverlap(%d,%d, %d,%d, %d) = %v, want %v",
					tt.a1, tt.a2, tt.b1, tt.b2, tt.threshold, got, tt.want)
			}
		})
	}
}

func TestFindCandidates(t *testing.T) {
	tests := []struct {
		name           string
		newFindings    []domain.Finding
		existing       []ExistingFinding
		lineThreshold  int
		maxCandidates  int
		wantCandidates int
		wantOverflow   int
	}{
		{
			name:           "no existing findings",
			newFindings:    []domain.Finding{{File: "foo.go", LineStart: 10, LineEnd: 15}},
			existing:       nil,
			lineThreshold:  10,
			maxCandidates:  50,
			wantCandidates: 0,
			wantOverflow:   0,
		},
		{
			name:        "no new findings",
			newFindings: nil,
			existing: []ExistingFinding{
				{File: "foo.go", LineStart: 10, LineEnd: 15},
			},
			lineThreshold:  10,
			maxCandidates:  50,
			wantCandidates: 0,
			wantOverflow:   0,
		},
		{
			name: "same file overlapping lines",
			newFindings: []domain.Finding{
				{File: "foo.go", LineStart: 10, LineEnd: 15, Description: "new finding"},
			},
			existing: []ExistingFinding{
				{File: "foo.go", LineStart: 12, LineEnd: 18, Description: "existing finding"},
			},
			lineThreshold:  10,
			maxCandidates:  50,
			wantCandidates: 1,
			wantOverflow:   0,
		},
		{
			name: "same file within threshold",
			newFindings: []domain.Finding{
				{File: "foo.go", LineStart: 30, LineEnd: 35, Description: "new finding"},
			},
			existing: []ExistingFinding{
				{File: "foo.go", LineStart: 10, LineEnd: 15, Description: "existing finding"},
			},
			lineThreshold:  20,
			maxCandidates:  50,
			wantCandidates: 1,
			wantOverflow:   0,
		},
		{
			name: "same file outside threshold with different content",
			newFindings: []domain.Finding{
				{File: "foo.go", LineStart: 50, LineEnd: 55, Category: "bug", Severity: "high", Description: "new finding"},
			},
			existing: []ExistingFinding{
				{File: "foo.go", LineStart: 10, LineEnd: 15, Category: "security", Severity: "medium", Description: "existing finding"},
			},
			lineThreshold:  10,
			maxCandidates:  50,
			wantCandidates: 0, // No spatial match (outside threshold) and no content match (different category/severity)
			wantOverflow:   0,
		},
		{
			name: "different files",
			newFindings: []domain.Finding{
				{File: "foo.go", LineStart: 10, LineEnd: 15, Description: "new finding"},
			},
			existing: []ExistingFinding{
				{File: "bar.go", LineStart: 10, LineEnd: 15, Description: "existing finding"},
			},
			lineThreshold:  10,
			maxCandidates:  50,
			wantCandidates: 0,
			wantOverflow:   0,
		},
		{
			name: "content match catches distant findings with same category and severity",
			newFindings: []domain.Finding{
				{File: "foo.go", LineStart: 500, LineEnd: 510, Category: "security", Severity: "high", Description: "SQL injection risk"},
			},
			existing: []ExistingFinding{
				{File: "foo.go", LineStart: 10, LineEnd: 15, Category: "security", Severity: "high", Description: "SQL injection vulnerability"},
			},
			lineThreshold:  10,
			maxCandidates:  50,
			wantCandidates: 1, // Content match: same category+severity, even though 490 lines apart
			wantOverflow:   0,
		},
		{
			name: "multiple candidates for one finding",
			newFindings: []domain.Finding{
				{File: "foo.go", LineStart: 20, LineEnd: 25, Description: "new finding"},
			},
			existing: []ExistingFinding{
				{File: "foo.go", LineStart: 10, LineEnd: 15, Description: "existing 1"},
				{File: "foo.go", LineStart: 22, LineEnd: 28, Description: "existing 2"},
			},
			lineThreshold:  10,
			maxCandidates:  50,
			wantCandidates: 2, // One new finding pairs with two existing
			wantOverflow:   0,
		},
		{
			name: "max candidates exceeded",
			newFindings: []domain.Finding{
				{File: "foo.go", LineStart: 10, LineEnd: 15, Category: "bug", Severity: "high", Description: "new 1"},
				{File: "foo.go", LineStart: 100, LineEnd: 105, Category: "perf", Severity: "low", Description: "new 2"},
				{File: "foo.go", LineStart: 200, LineEnd: 205, Category: "style", Severity: "info", Description: "new 3"},
			},
			existing: []ExistingFinding{
				{File: "foo.go", LineStart: 12, LineEnd: 18, Category: "bug", Severity: "high", Description: "existing 1"},
				{File: "foo.go", LineStart: 102, LineEnd: 108, Category: "perf", Severity: "low", Description: "existing 2"},
				{File: "foo.go", LineStart: 202, LineEnd: 208, Category: "style", Severity: "info", Description: "existing 3"},
			},
			lineThreshold:  10,
			maxCandidates:  2,
			wantCandidates: 2,
			wantOverflow:   1, // Third new finding goes to overflow (each matches only its corresponding existing)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidates, overflow := FindCandidates(
				tt.newFindings,
				tt.existing,
				tt.lineThreshold,
				tt.maxCandidates,
			)

			if len(candidates) != tt.wantCandidates {
				t.Errorf("FindCandidates() got %d candidates, want %d",
					len(candidates), tt.wantCandidates)
			}

			if len(overflow) != tt.wantOverflow {
				t.Errorf("FindCandidates() got %d overflow, want %d",
					len(overflow), tt.wantOverflow)
			}
		})
	}
}

func TestExtractUnpairedFindings(t *testing.T) {
	tests := []struct {
		name        string
		newFindings []domain.Finding
		candidates  []CandidatePair
		wantCount   int
	}{
		{
			name:        "no findings",
			newFindings: nil,
			candidates:  nil,
			wantCount:   0,
		},
		{
			name: "no candidates - all unpaired",
			newFindings: []domain.Finding{
				{File: "foo.go", Category: "error", Severity: "high", Description: "desc1"},
				{File: "bar.go", Category: "error", Severity: "high", Description: "desc2"},
			},
			candidates: nil,
			wantCount:  2,
		},
		{
			name: "all paired",
			newFindings: []domain.Finding{
				{File: "foo.go", Category: "error", Severity: "high", Description: "desc1"},
			},
			candidates: []CandidatePair{
				{
					New: domain.Finding{File: "foo.go", Category: "error", Severity: "high", Description: "desc1"},
				},
			},
			wantCount: 0,
		},
		{
			name: "mixed paired and unpaired",
			newFindings: []domain.Finding{
				{File: "foo.go", Category: "error", Severity: "high", Description: "desc1"},
				{File: "bar.go", Category: "warn", Severity: "low", Description: "desc2"},
				{File: "baz.go", Category: "info", Severity: "low", Description: "desc3"},
			},
			candidates: []CandidatePair{
				{
					New: domain.Finding{File: "foo.go", Category: "error", Severity: "high", Description: "desc1"},
				},
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unpaired := ExtractUnpairedFindings(tt.newFindings, tt.candidates)
			if len(unpaired) != tt.wantCount {
				t.Errorf("ExtractUnpairedFindings() got %d, want %d",
					len(unpaired), tt.wantCount)
			}
		})
	}
}
