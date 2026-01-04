package diff_test

import (
	"testing"

	"github.com/delightfulhammers/bop/internal/diff"
)

// equalIntPtr compares two *int values for equality (test helper).
func equalIntPtr(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func TestParse_SingleHunk(t *testing.T) {
	patch := `@@ -10,3 +10,4 @@ func example() {
 context line
+added line
 another context
+second addition
`

	parsed, err := diff.Parse(patch)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(parsed.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(parsed.Hunks))
	}

	hunk := parsed.Hunks[0]
	if hunk.NewStart != 10 {
		t.Errorf("expected NewStart=10, got %d", hunk.NewStart)
	}

	// Should have 4 lines: context, addition, context, addition
	if len(hunk.Lines) != 4 {
		t.Errorf("expected 4 lines, got %d", len(hunk.Lines))
	}
}

func TestParse_MultipleHunks(t *testing.T) {
	patch := `@@ -10,2 +10,3 @@ func first() {
 context
+added
@@ -20,2 +21,3 @@ func second() {
 context
+added
`

	parsed, err := diff.Parse(patch)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(parsed.Hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(parsed.Hunks))
	}

	if parsed.Hunks[0].NewStart != 10 {
		t.Errorf("hunk 0: expected NewStart=10, got %d", parsed.Hunks[0].NewStart)
	}
	if parsed.Hunks[1].NewStart != 21 {
		t.Errorf("hunk 1: expected NewStart=21, got %d", parsed.Hunks[1].NewStart)
	}
}

func TestParse_AdditionsOnly(t *testing.T) {
	// New file - all additions
	patch := `@@ -0,0 +1,3 @@
+line one
+line two
+line three
`

	parsed, err := diff.Parse(patch)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(parsed.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(parsed.Hunks))
	}

	hunk := parsed.Hunks[0]
	if len(hunk.Lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(hunk.Lines))
	}

	for i, line := range hunk.Lines {
		if line.Type != diff.LineAddition {
			t.Errorf("line %d: expected Addition, got %v", i, line.Type)
		}
	}
}

func TestParse_DeletionsOnly(t *testing.T) {
	// Deleted file - all deletions
	patch := `@@ -1,3 +0,0 @@
-line one
-line two
-line three
`

	parsed, err := diff.Parse(patch)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(parsed.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(parsed.Hunks))
	}

	hunk := parsed.Hunks[0]
	for i, line := range hunk.Lines {
		if line.Type != diff.LineDeletion {
			t.Errorf("line %d: expected Deletion, got %v", i, line.Type)
		}
		if line.NewLine != nil {
			t.Errorf("line %d: deletion should have nil NewLine", i)
		}
	}
}

func TestParse_MixedChanges(t *testing.T) {
	patch := `@@ -5,4 +5,4 @@ package main
 import "fmt"
-func old() {}
+func new() {}
 func main() {}
`

	parsed, err := diff.Parse(patch)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	hunk := parsed.Hunks[0]
	// context, deletion, addition, context = 4 lines
	if len(hunk.Lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(hunk.Lines))
	}

	expected := []diff.LineType{
		diff.LineContext,
		diff.LineDeletion,
		diff.LineAddition,
		diff.LineContext,
	}

	for i, line := range hunk.Lines {
		if line.Type != expected[i] {
			t.Errorf("line %d: expected %v, got %v", i, expected[i], line.Type)
		}
	}
}

func TestParse_EmptyPatch(t *testing.T) {
	parsed, err := diff.Parse("")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(parsed.Hunks) != 0 {
		t.Errorf("expected 0 hunks for empty patch, got %d", len(parsed.Hunks))
	}
}

func TestParsedDiff_FindPosition_InDiff(t *testing.T) {
	patch := `@@ -10,3 +10,4 @@ func example() {
 context line 10
+added line 11
 context line 12
+added line 13
`

	parsed, err := diff.Parse(patch)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	tests := []struct {
		name       string
		lineNumber int
		wantPos    *int
	}{
		{"context line 10", 10, diff.IntPtr(1)},
		{"added line 11", 11, diff.IntPtr(2)},
		{"context line 12", 12, diff.IntPtr(3)},
		{"added line 13", 13, diff.IntPtr(4)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsed.FindPosition(tt.lineNumber)
			if !equalIntPtr(got, tt.wantPos) {
				t.Errorf("FindPosition(%d) = %v, want %v", tt.lineNumber, got, tt.wantPos)
			}
		})
	}
}

func TestParsedDiff_FindPosition_NotInDiff(t *testing.T) {
	patch := `@@ -10,2 +10,3 @@ func example() {
 context line 10
+added line 11
`

	parsed, err := diff.Parse(patch)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	tests := []struct {
		name       string
		lineNumber int
	}{
		{"line before diff", 5},
		{"line after diff", 20},
		{"line 0 (invalid)", 0},
		{"negative line", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsed.FindPosition(tt.lineNumber)
			if got != nil {
				t.Errorf("FindPosition(%d) = %v, want nil", tt.lineNumber, *got)
			}
		})
	}
}

func TestParsedDiff_FindPosition_DeletedLine(t *testing.T) {
	// Deletions don't have new-side line numbers, so can't be found
	patch := `@@ -10,3 +10,2 @@ func example() {
 context line 10
-deleted line (was 11)
 context line 11 (was 12)
`

	parsed, err := diff.Parse(patch)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// Line 10 should be at position 1
	pos := parsed.FindPosition(10)
	if !equalIntPtr(pos, diff.IntPtr(1)) {
		t.Errorf("FindPosition(10) = %v, want 1", pos)
	}

	// Line 11 (the new context line) should be at position 3
	// Position 2 is the deletion
	pos = parsed.FindPosition(11)
	if !equalIntPtr(pos, diff.IntPtr(3)) {
		t.Errorf("FindPosition(11) = %v, want 3", pos)
	}
}

func TestParsedDiff_FindPosition_MultipleHunks(t *testing.T) {
	patch := `@@ -10,2 +10,3 @@ func first() {
 context 10
+added 11
@@ -20,2 +21,3 @@ func second() {
 context 21
+added 22
`

	parsed, err := diff.Parse(patch)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	tests := []struct {
		lineNumber int
		wantPos    *int
	}{
		{10, diff.IntPtr(1)}, // First hunk, position 1
		{11, diff.IntPtr(2)}, // First hunk, position 2
		{21, diff.IntPtr(3)}, // Second hunk, position 3 (continues from first)
		{22, diff.IntPtr(4)}, // Second hunk, position 4
		{15, nil},            // Between hunks - not in diff
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := parsed.FindPosition(tt.lineNumber)
			if !equalIntPtr(got, tt.wantPos) {
				t.Errorf("FindPosition(%d) = %v, want %v", tt.lineNumber, got, tt.wantPos)
			}
		})
	}
}

func TestParse_NoNewlineAtEOF(t *testing.T) {
	patch := `@@ -1,2 +1,2 @@
 line one
-line two
\ No newline at end of file
+line two modified
\ No newline at end of file
`

	parsed, err := diff.Parse(patch)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// Should have 1 hunk with lines (ignoring the "\ No newline" markers)
	if len(parsed.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(parsed.Hunks))
	}

	// The "\ No newline" lines should be skipped
	hunk := parsed.Hunks[0]
	for _, line := range hunk.Lines {
		if line.Type != diff.LineContext && line.Type != diff.LineAddition && line.Type != diff.LineDeletion {
			t.Errorf("unexpected line type: %v", line.Type)
		}
	}
}

func TestParse_WithFileHeaders(t *testing.T) {
	// Real diff with git headers
	patch := `diff --git a/file.go b/file.go
index 1234567..abcdefg 100644
--- a/file.go
+++ b/file.go
@@ -10,3 +10,4 @@ func example() {
 context
+added
 more context
`

	parsed, err := diff.Parse(patch)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(parsed.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(parsed.Hunks))
	}

	// Position should start from @@ line, not file headers
	pos := parsed.FindPosition(10)
	if !equalIntPtr(pos, diff.IntPtr(1)) {
		t.Errorf("FindPosition(10) = %v, want 1", pos)
	}
}
