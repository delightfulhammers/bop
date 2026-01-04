package review

import (
	"testing"

	"github.com/delightfulhammers/bop/internal/domain"
)

func TestDetectChangeTypes(t *testing.T) {
	tests := []struct {
		name     string
		diff     domain.Diff
		expected []string
	}{
		{
			name: "auth changes",
			diff: domain.Diff{
				Files: []domain.FileDiff{
					{Path: "internal/auth/handler.go", Status: domain.FileStatusModified},
					{Path: "internal/auth/middleware.go", Status: domain.FileStatusAdded},
				},
			},
			expected: []string{"auth", "api"}, // handler.go is both auth and api
		},
		{
			name: "database changes",
			diff: domain.Diff{
				Files: []domain.FileDiff{
					{Path: "internal/adapter/store/sqlite/store.go", Status: domain.FileStatusModified},
					{Path: "migrations/001_create_users.sql", Status: domain.FileStatusAdded},
				},
			},
			expected: []string{"database"},
		},
		{
			name: "api changes",
			diff: domain.Diff{
				Files: []domain.FileDiff{
					{Path: "internal/adapter/api/handler.go", Status: domain.FileStatusModified},
					{Path: "internal/controller/user_controller.go", Status: domain.FileStatusAdded},
				},
			},
			expected: []string{"api"},
		},
		{
			name: "security changes",
			diff: domain.Diff{
				Files: []domain.FileDiff{
					{Path: "internal/security/encryption.go", Status: domain.FileStatusModified},
					{Path: "internal/redaction/engine.go", Status: domain.FileStatusModified},
				},
			},
			expected: []string{"security"},
		},
		{
			name: "config changes",
			diff: domain.Diff{
				Files: []domain.FileDiff{
					{Path: "internal/config/loader.go", Status: domain.FileStatusModified},
					{Path: "config.yaml", Status: domain.FileStatusModified},
				},
			},
			expected: []string{"config"},
		},
		{
			name: "testing changes",
			diff: domain.Diff{
				Files: []domain.FileDiff{
					{Path: "internal/adapter/store/sqlite/store_test.go", Status: domain.FileStatusModified},
				},
			},
			expected: []string{"testing", "database"}, // store_test.go is both testing and database
		},
		{
			name: "documentation changes",
			diff: domain.Diff{
				Files: []domain.FileDiff{
					{Path: "docs/ARCHITECTURE.md", Status: domain.FileStatusModified},
					{Path: "README.md", Status: domain.FileStatusModified},
				},
			},
			expected: []string{"documentation"},
		},
		{
			name: "frontend changes",
			diff: domain.Diff{
				Files: []domain.FileDiff{
					{Path: "ui/components/Header.tsx", Status: domain.FileStatusAdded},
					{Path: "frontend/views/Dashboard.jsx", Status: domain.FileStatusModified},
				},
			},
			expected: []string{"frontend"},
		},
		{
			name: "multiple change types",
			diff: domain.Diff{
				Files: []domain.FileDiff{
					{Path: "internal/auth/handler.go", Status: domain.FileStatusModified},
					{Path: "internal/adapter/store/sqlite/store.go", Status: domain.FileStatusModified},
					{Path: "internal/adapter/api/handler.go", Status: domain.FileStatusModified},
					{Path: "docs/API.md", Status: domain.FileStatusModified},
				},
			},
			expected: []string{"auth", "database", "api", "documentation"},
		},
		{
			name: "no recognizable change types",
			diff: domain.Diff{
				Files: []domain.FileDiff{
					{Path: "internal/util/helper.go", Status: domain.FileStatusModified},
				},
			},
			expected: []string{},
		},
		{
			name: "empty diff",
			diff: domain.Diff{
				Files: []domain.FileDiff{},
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gatherer := &ContextGatherer{}
			result := gatherer.detectChangeTypes(tt.diff)

			// Convert to map for easier comparison (order doesn't matter)
			resultMap := make(map[string]bool)
			for _, typ := range result {
				resultMap[typ] = true
			}

			expectedMap := make(map[string]bool)
			for _, typ := range tt.expected {
				expectedMap[typ] = true
			}

			// Check all expected types are present
			for expectedType := range expectedMap {
				if !resultMap[expectedType] {
					t.Errorf("expected change type %q not found in result %v", expectedType, result)
				}
			}

			// Check no unexpected types are present
			for resultType := range resultMap {
				if !expectedMap[resultType] {
					t.Errorf("unexpected change type %q found in result %v", resultType, result)
				}
			}
		})
	}
}

func TestDetectChangeTypes_CaseInsensitive(t *testing.T) {
	// Test that detection works regardless of case
	diff := domain.Diff{
		Files: []domain.FileDiff{
			{Path: "INTERNAL/AUTH/HANDLER.GO", Status: domain.FileStatusModified},
			{Path: "Internal/Database/Store.go", Status: domain.FileStatusModified},
		},
	}

	gatherer := &ContextGatherer{}
	result := gatherer.detectChangeTypes(diff)

	expected := map[string]bool{
		"auth":     true,
		"database": true,
	}

	resultMap := make(map[string]bool)
	for _, typ := range result {
		resultMap[typ] = true
	}

	for expectedType := range expected {
		if !resultMap[expectedType] {
			t.Errorf("expected change type %q not found (case-insensitive test)", expectedType)
		}
	}
}

func TestLoadFile(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		expectError bool
		expectEmpty bool
	}{
		{
			name:        "load README",
			path:        "README.md",
			expectError: false,
			expectEmpty: false,
		},
		{
			name:        "load architecture doc",
			path:        "docs/ARCHITECTURE.md",
			expectError: false,
			expectEmpty: false,
		},
		{
			name:        "load design doc",
			path:        "docs/AUTH_DESIGN.md",
			expectError: false,
			expectEmpty: false,
		},
		{
			name:        "missing file",
			path:        "docs/NONEXISTENT.md",
			expectError: true,
			expectEmpty: true,
		},
	}

	gatherer := NewContextGatherer("testdata")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := gatherer.loadFile(tt.path)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if tt.expectEmpty && content != "" {
				t.Errorf("expected empty content but got %d bytes", len(content))
			}

			if !tt.expectEmpty && !tt.expectError && content == "" {
				t.Error("expected non-empty content but got empty string")
			}

			// Verify content contains expected text (for non-error cases)
			if !tt.expectError && !tt.expectEmpty {
				switch tt.path {
				case "README.md":
					if !contains(content, "Test Project") {
						t.Error("README content doesn't contain expected text")
					}
				case "docs/ARCHITECTURE.md":
					if !contains(content, "clean architecture") {
						t.Error("ARCHITECTURE content doesn't contain expected text")
					}
				case "docs/AUTH_DESIGN.md":
					if !contains(content, "JWT-based") {
						t.Error("AUTH_DESIGN content doesn't contain expected text")
					}
				}
			}
		})
	}
}

func TestLoadDesignDocs(t *testing.T) {
	gatherer := NewContextGatherer("testdata")
	gatherer.config.DesignDocsGlob = "docs/*_DESIGN.md"

	docs, err := gatherer.loadDesignDocs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(docs) != 2 {
		t.Errorf("expected 2 design docs, got %d", len(docs))
	}

	// Verify docs contain expected content
	foundAuth := false
	foundDatabase := false

	for _, doc := range docs {
		if contains(doc, "AUTH_DESIGN.md") && contains(doc, "JWT-based") {
			foundAuth = true
		}
		if contains(doc, "DATABASE_DESIGN.md") && contains(doc, "SQLite") {
			foundDatabase = true
		}
	}

	if !foundAuth {
		t.Error("AUTH_DESIGN.md not found or missing expected content")
	}

	if !foundDatabase {
		t.Error("DATABASE_DESIGN.md not found or missing expected content")
	}
}

func TestFindRelevantDocs(t *testing.T) {
	tests := []struct {
		name         string
		changeTypes  []string
		expectDocs   []string // Document names that should be found
		expectNotDoc []string // Document names that should NOT be found
	}{
		{
			name:        "auth changes load security docs",
			changeTypes: []string{"auth"},
			expectDocs:  []string{"SECURITY.md", "AUTH_DESIGN.md"},
		},
		{
			name:        "database changes load database docs",
			changeTypes: []string{"database"},
			expectDocs:  []string{"DATABASE_DESIGN.md"},
		},
		{
			name:        "security changes load security docs",
			changeTypes: []string{"security"},
			expectDocs:  []string{"SECURITY.md"},
		},
		{
			name:         "multiple change types load multiple docs",
			changeTypes:  []string{"auth", "database"},
			expectDocs:   []string{"SECURITY.md", "AUTH_DESIGN.md", "DATABASE_DESIGN.md"},
			expectNotDoc: []string{"ARCHITECTURE.md"}, // Not in relevant docs
		},
		{
			name:        "unknown change type returns empty",
			changeTypes: []string{"unknown_type"},
			expectDocs:  []string{},
		},
		{
			name:        "empty change types returns empty",
			changeTypes: []string{},
			expectDocs:  []string{},
		},
	}

	gatherer := NewContextGatherer("testdata")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs, err := gatherer.findRelevantDocs([]string{}, tt.changeTypes)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check expected docs are present
			for _, expectedDoc := range tt.expectDocs {
				found := false
				for _, doc := range docs {
					if contains(doc, expectedDoc) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected document %q not found in results", expectedDoc)
				}
			}

			// Check unexpected docs are NOT present
			for _, unexpectedDoc := range tt.expectNotDoc {
				for _, doc := range docs {
					if contains(doc, unexpectedDoc) {
						t.Errorf("unexpected document %q found in results", unexpectedDoc)
					}
				}
			}
		})
	}
}

func TestFindRelevantDocs_DeduplicatesDocs(t *testing.T) {
	// Test that same doc isn't loaded twice for overlapping change types
	gatherer := NewContextGatherer("testdata")

	// Both "auth" and "security" should load SECURITY.md
	// But it should only appear once
	docs, err := gatherer.findRelevantDocs([]string{}, []string{"auth", "security"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Count occurrences of SECURITY.md
	count := 0
	for _, doc := range docs {
		if contains(doc, "SECURITY.md") {
			count++
		}
	}

	if count != 1 {
		t.Errorf("expected SECURITY.md to appear once, but appeared %d times", count)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && hasSubstring(s, substr))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
