package github_test

import (
	"testing"

	"github.com/delightfulhammers/bop/internal/adapter/github"
	"github.com/delightfulhammers/bop/internal/diff"
	"github.com/delightfulhammers/bop/internal/domain"
)

func TestMapFindings_SingleFile(t *testing.T) {
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{
				Path:   "main.go",
				Status: domain.FileStatusModified,
				Patch: `@@ -10,3 +10,4 @@ func example() {
 context line 10
+added line 11
 context line 12
`,
			},
		},
	}

	findings := []domain.Finding{
		{
			ID:        "f1",
			File:      "main.go",
			LineStart: 11,
			LineEnd:   11,
			Severity:  "medium",
		},
	}

	result := github.MapFindings(findings, diff)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	if result[0].Finding.ID != "f1" {
		t.Errorf("expected finding ID 'f1', got '%s'", result[0].Finding.ID)
	}

	if result[0].DiffPosition == nil {
		t.Fatal("expected DiffPosition to be set")
	}

	if *result[0].DiffPosition != 2 {
		t.Errorf("expected DiffPosition=2, got %d", *result[0].DiffPosition)
	}
}

func TestMapFindings_MultipleFiles(t *testing.T) {
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{
				Path:   "file1.go",
				Status: domain.FileStatusModified,
				Patch: `@@ -1,2 +1,3 @@
 line 1
+added line 2
`,
			},
			{
				Path:   "file2.go",
				Status: domain.FileStatusModified,
				Patch: `@@ -5,2 +5,3 @@
 line 5
+added line 6
`,
			},
		},
	}

	findings := []domain.Finding{
		{ID: "f1", File: "file1.go", LineStart: 2},
		{ID: "f2", File: "file2.go", LineStart: 6},
	}

	result := github.MapFindings(findings, diff)

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	// file1.go: line 2 is at position 2
	if result[0].DiffPosition == nil || *result[0].DiffPosition != 2 {
		t.Errorf("f1: expected position 2, got %v", result[0].DiffPosition)
	}

	// file2.go: line 6 is at position 2 (within that file's diff)
	if result[1].DiffPosition == nil || *result[1].DiffPosition != 2 {
		t.Errorf("f2: expected position 2, got %v", result[1].DiffPosition)
	}
}

func TestMapFindings_FileNotInDiff(t *testing.T) {
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{
				Path:   "other.go",
				Status: domain.FileStatusModified,
				Patch: `@@ -1,2 +1,3 @@
 context
+added
`,
			},
		},
	}

	findings := []domain.Finding{
		{ID: "f1", File: "missing.go", LineStart: 10},
	}

	result := github.MapFindings(findings, diff)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	if result[0].DiffPosition != nil {
		t.Errorf("expected nil DiffPosition for missing file, got %d", *result[0].DiffPosition)
	}
}

func TestMapFindings_LineNotInDiff(t *testing.T) {
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{
				Path:   "main.go",
				Status: domain.FileStatusModified,
				Patch: `@@ -10,2 +10,3 @@
 context line 10
+added line 11
`,
			},
		},
	}

	findings := []domain.Finding{
		{ID: "f1", File: "main.go", LineStart: 50}, // Line not in diff
	}

	result := github.MapFindings(findings, diff)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	if result[0].DiffPosition != nil {
		t.Errorf("expected nil DiffPosition for line not in diff, got %d", *result[0].DiffPosition)
	}
}

func TestMapFindings_EmptyFindings(t *testing.T) {
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{Path: "main.go", Patch: "@@ -1 +1 @@\n+line"},
		},
	}

	result := github.MapFindings([]domain.Finding{}, diff)

	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func TestMapFindings_EmptyDiff(t *testing.T) {
	diff := domain.Diff{
		Files: []domain.FileDiff{},
	}

	findings := []domain.Finding{
		{ID: "f1", File: "main.go", LineStart: 10},
	}

	result := github.MapFindings(findings, diff)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	if result[0].DiffPosition != nil {
		t.Errorf("expected nil DiffPosition for empty diff, got %d", *result[0].DiffPosition)
	}
}

func TestMapFindings_PreservesAllFindingFields(t *testing.T) {
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{
				Path:  "main.go",
				Patch: "@@ -1,1 +1,2 @@\n context\n+added line 2",
			},
		},
	}

	original := domain.Finding{
		ID:          "abc123",
		File:        "main.go",
		LineStart:   2,
		LineEnd:     5,
		Severity:    "high",
		Category:    "security",
		Description: "SQL injection vulnerability",
		Suggestion:  "Use parameterized queries",
		Evidence:    true,
	}

	result := github.MapFindings([]domain.Finding{original}, diff)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	f := result[0].Finding
	if f.ID != original.ID {
		t.Errorf("ID mismatch: got %s, want %s", f.ID, original.ID)
	}
	if f.File != original.File {
		t.Errorf("File mismatch: got %s, want %s", f.File, original.File)
	}
	if f.LineStart != original.LineStart {
		t.Errorf("LineStart mismatch: got %d, want %d", f.LineStart, original.LineStart)
	}
	if f.LineEnd != original.LineEnd {
		t.Errorf("LineEnd mismatch: got %d, want %d", f.LineEnd, original.LineEnd)
	}
	if f.Severity != original.Severity {
		t.Errorf("Severity mismatch: got %s, want %s", f.Severity, original.Severity)
	}
	if f.Category != original.Category {
		t.Errorf("Category mismatch: got %s, want %s", f.Category, original.Category)
	}
	if f.Description != original.Description {
		t.Errorf("Description mismatch")
	}
	if f.Suggestion != original.Suggestion {
		t.Errorf("Suggestion mismatch")
	}
	if f.Evidence != original.Evidence {
		t.Errorf("Evidence mismatch")
	}
}

func TestMapFindings_MalformedPatch(t *testing.T) {
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{
				Path:  "main.go",
				Patch: "this is not a valid diff",
			},
		},
	}

	findings := []domain.Finding{
		{ID: "f1", File: "main.go", LineStart: 10},
	}

	// Should not panic, should return nil position
	result := github.MapFindings(findings, diff)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	if result[0].DiffPosition != nil {
		t.Errorf("expected nil DiffPosition for malformed patch")
	}
}

func TestPositionedFinding_InDiff(t *testing.T) {
	pf := github.PositionedFinding{
		Finding:      domain.Finding{ID: "test"},
		DiffPosition: diff.IntPtr(5),
	}

	if !pf.InDiff() {
		t.Error("expected InDiff() to return true when DiffPosition is set")
	}
}

func TestPositionedFinding_InDiff_False(t *testing.T) {
	pf := github.PositionedFinding{
		Finding:      domain.Finding{ID: "test"},
		DiffPosition: nil,
	}

	if pf.InDiff() {
		t.Error("expected InDiff() to return false when DiffPosition is nil")
	}
}

func TestMapFindings_RenamedFile_NewPath(t *testing.T) {
	// Finding uses the new path - should match
	d := domain.Diff{
		Files: []domain.FileDiff{
			{
				Path:    "new_name.go",
				OldPath: "old_name.go",
				Status:  domain.FileStatusRenamed,
				Patch: `@@ -1,2 +1,3 @@
 context line 1
+added line 2
`,
			},
		},
	}

	findings := []domain.Finding{
		{ID: "f1", File: "new_name.go", LineStart: 2},
	}

	result := github.MapFindings(findings, d)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	if result[0].DiffPosition == nil {
		t.Fatal("expected DiffPosition to be set for finding on new path")
	}

	if *result[0].DiffPosition != 2 {
		t.Errorf("expected position 2, got %d", *result[0].DiffPosition)
	}
}

func TestMapFindings_RenamedFile_OldPath(t *testing.T) {
	// Finding uses the old path - should also match (LLM might reference old path)
	d := domain.Diff{
		Files: []domain.FileDiff{
			{
				Path:    "new_name.go",
				OldPath: "old_name.go",
				Status:  domain.FileStatusRenamed,
				Patch: `@@ -1,2 +1,3 @@
 context line 1
+added line 2
`,
			},
		},
	}

	findings := []domain.Finding{
		{ID: "f1", File: "old_name.go", LineStart: 2},
	}

	result := github.MapFindings(findings, d)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	if result[0].DiffPosition == nil {
		t.Fatal("expected DiffPosition to be set for finding on old path of renamed file")
	}

	if *result[0].DiffPosition != 2 {
		t.Errorf("expected position 2, got %d", *result[0].DiffPosition)
	}
}

func TestMapFindings_BinaryFile_Skipped(t *testing.T) {
	// Binary files should be skipped when mapping positions
	d := domain.Diff{
		Files: []domain.FileDiff{
			{
				Path:     "image.png",
				Status:   domain.FileStatusModified,
				Patch:    "Binary files a/image.png and b/image.png differ",
				IsBinary: true,
			},
		},
	}

	findings := []domain.Finding{
		{ID: "f1", File: "image.png", LineStart: 1},
	}

	result := github.MapFindings(findings, d)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	// Position should be nil for binary files
	if result[0].DiffPosition != nil {
		t.Errorf("expected nil DiffPosition for binary file, got %d", *result[0].DiffPosition)
	}
}
