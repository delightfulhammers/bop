package dedup

import (
	"context"
	"errors"
	"testing"

	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/dedup"
)

// mockClient implements the Client interface for testing.
type mockClient struct {
	response string
	err      error
}

func (m *mockClient) Compare(ctx context.Context, prompt string, maxTokens int) (string, error) {
	return m.response, m.err
}

func TestComparer_Compare_EmptyCandidates(t *testing.T) {
	client := &mockClient{}
	comparer := NewComparer(client, 1000)

	result, err := comparer.Compare(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Duplicates) != 0 {
		t.Errorf("expected 0 duplicates, got %d", len(result.Duplicates))
	}
	if len(result.Unique) != 0 {
		t.Errorf("expected 0 unique, got %d", len(result.Unique))
	}
}

func TestComparer_Compare_Success(t *testing.T) {
	response := `{
		"comparisons": [
			{"pair_index": 0, "is_duplicate": true, "reason": "Same error handling issue"},
			{"pair_index": 1, "is_duplicate": false, "reason": "Different issues"}
		]
	}`

	client := &mockClient{response: response}
	comparer := NewComparer(client, 1000)

	candidates := []dedup.CandidatePair{
		{
			Existing: dedup.ExistingFinding{
				Fingerprint: "abc123",
				File:        "foo.go",
				Description: "Error not handled",
			},
			New: domain.Finding{
				File:        "foo.go",
				Description: "Missing error handling",
			},
		},
		{
			Existing: dedup.ExistingFinding{
				Fingerprint: "def456",
				File:        "bar.go",
				Description: "Performance issue",
			},
			New: domain.Finding{
				File:        "bar.go",
				Description: "Security vulnerability",
			},
		},
	}

	result, err := comparer.Compare(context.Background(), candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Duplicates) != 1 {
		t.Errorf("expected 1 duplicate, got %d", len(result.Duplicates))
	}
	if len(result.Unique) != 1 {
		t.Errorf("expected 1 unique, got %d", len(result.Unique))
	}

	if result.Duplicates[0].ExistingFingerprint != "abc123" {
		t.Errorf("expected fingerprint abc123, got %s", result.Duplicates[0].ExistingFingerprint)
	}
}

func TestComparer_Compare_FailOpen(t *testing.T) {
	client := &mockClient{err: errors.New("API error")}
	comparer := NewComparer(client, 1000)

	candidates := []dedup.CandidatePair{
		{
			Existing: dedup.ExistingFinding{File: "foo.go", Description: "existing"},
			New:      domain.Finding{File: "foo.go", Description: "new"},
		},
	}

	result, err := comparer.Compare(context.Background(), candidates)
	if err != nil {
		t.Fatalf("should not return error on fail-open: %v", err)
	}

	// Fail-open means all treated as unique
	if len(result.Duplicates) != 0 {
		t.Errorf("expected 0 duplicates on fail-open, got %d", len(result.Duplicates))
	}
	if len(result.Unique) != 1 {
		t.Errorf("expected 1 unique on fail-open, got %d", len(result.Unique))
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantJSON string
	}{
		{
			name:     "json code block",
			input:    "Here's the analysis:\n```json\n{\"key\": \"value\"}\n```\nDone.",
			wantJSON: `{"key": "value"}`,
		},
		{
			name:     "plain code block",
			input:    "Result:\n```\n{\"key\": \"value\"}\n```",
			wantJSON: `{"key": "value"}`,
		},
		{
			name:     "raw json",
			input:    "The result is {\"key\": \"value\"} as shown.",
			wantJSON: `{"key": "value"}`,
		},
		{
			name:     "nested json",
			input:    `{"outer": {"inner": "value"}}`,
			wantJSON: `{"outer": {"inner": "value"}}`,
		},
		{
			name:     "no json",
			input:    "No JSON here",
			wantJSON: "",
		},
		{
			name:     "json with comparisons",
			input:    "```json\n{\"comparisons\": [{\"pair_index\": 0, \"is_duplicate\": true}]}\n```",
			wantJSON: `{"comparisons": [{"pair_index": 0, "is_duplicate": true}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			if got != tt.wantJSON {
				t.Errorf("extractJSON() = %q, want %q", got, tt.wantJSON)
			}
		})
	}
}

func TestParseComparisonResponse(t *testing.T) {
	tests := []struct {
		name           string
		response       string
		candidates     []dedup.CandidatePair
		wantDuplicates int
		wantUnique     int
		wantError      bool
	}{
		{
			name:     "all duplicates",
			response: `{"comparisons": [{"pair_index": 0, "is_duplicate": true, "reason": "same"}]}`,
			candidates: []dedup.CandidatePair{
				{Existing: dedup.ExistingFinding{Fingerprint: "fp1"}, New: domain.Finding{File: "f1"}},
			},
			wantDuplicates: 1,
			wantUnique:     0,
			wantError:      false,
		},
		{
			name:     "all unique",
			response: `{"comparisons": [{"pair_index": 0, "is_duplicate": false, "reason": "different"}]}`,
			candidates: []dedup.CandidatePair{
				{Existing: dedup.ExistingFinding{Fingerprint: "fp1"}, New: domain.Finding{File: "f1"}},
			},
			wantDuplicates: 0,
			wantUnique:     1,
			wantError:      false,
		},
		{
			name:     "mixed results",
			response: `{"comparisons": [{"pair_index": 0, "is_duplicate": true}, {"pair_index": 1, "is_duplicate": false}]}`,
			candidates: []dedup.CandidatePair{
				{Existing: dedup.ExistingFinding{Fingerprint: "fp1"}, New: domain.Finding{File: "f1", Description: "d1"}},
				{Existing: dedup.ExistingFinding{Fingerprint: "fp2"}, New: domain.Finding{File: "f2", Description: "d2"}},
			},
			wantDuplicates: 1,
			wantUnique:     1,
			wantError:      false,
		},
		{
			name:           "no json",
			response:       "I don't understand",
			candidates:     []dedup.CandidatePair{{New: domain.Finding{File: "f1"}}},
			wantDuplicates: 0,
			wantUnique:     0,
			wantError:      true,
		},
		{
			name:           "invalid json",
			response:       `{"comparisons": invalid}`,
			candidates:     []dedup.CandidatePair{{New: domain.Finding{File: "f1"}}},
			wantDuplicates: 0,
			wantUnique:     0,
			wantError:      true,
		},
		{
			name:     "out of bounds pair_index ignored",
			response: `{"comparisons": [{"pair_index": 5, "is_duplicate": true}]}`,
			candidates: []dedup.CandidatePair{
				{Existing: dedup.ExistingFinding{Fingerprint: "fp1"}, New: domain.Finding{File: "f1"}},
			},
			wantDuplicates: 0,
			wantUnique:     1, // The only candidate isn't marked as duplicate
			wantError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseComparisonResponse(tt.response, tt.candidates)

			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result.Duplicates) != tt.wantDuplicates {
				t.Errorf("got %d duplicates, want %d", len(result.Duplicates), tt.wantDuplicates)
			}

			if len(result.Unique) != tt.wantUnique {
				t.Errorf("got %d unique, want %d", len(result.Unique), tt.wantUnique)
			}
		})
	}
}

func TestFailOpen(t *testing.T) {
	candidates := []dedup.CandidatePair{
		{New: domain.Finding{File: "a.go", Description: "first"}},
		{New: domain.Finding{File: "b.go", Description: "second"}},
		{New: domain.Finding{File: "a.go", Description: "first"}}, // Duplicate new finding
	}

	result := failOpen(candidates)

	if len(result.Duplicates) != 0 {
		t.Errorf("failOpen should produce 0 duplicates, got %d", len(result.Duplicates))
	}

	// Should dedupe identical new findings
	if len(result.Unique) != 2 {
		t.Errorf("failOpen should produce 2 unique (deduped), got %d", len(result.Unique))
	}
}

func TestBuildComparisonPrompt(t *testing.T) {
	candidates := []dedup.CandidatePair{
		{
			Existing: dedup.ExistingFinding{
				File:        "foo.go",
				LineStart:   10,
				LineEnd:     15,
				Severity:    "high",
				Category:    "security",
				Description: "SQL injection vulnerability",
			},
			New: domain.Finding{
				File:        "foo.go",
				LineStart:   12,
				LineEnd:     18,
				Severity:    "critical",
				Category:    "security",
				Description: "Potential SQL injection",
			},
		},
	}

	prompt := buildComparisonPrompt(candidates)

	// Check that key elements are present
	if !contains(prompt, "semantic duplicates") {
		t.Error("prompt should mention semantic duplicates")
	}
	if !contains(prompt, "foo.go") {
		t.Error("prompt should include file name")
	}
	if !contains(prompt, "SQL injection vulnerability") {
		t.Error("prompt should include existing description")
	}
	if !contains(prompt, "Potential SQL injection") {
		t.Error("prompt should include new description")
	}
	if !contains(prompt, "Pair 0") {
		t.Error("prompt should include pair index")
	}
	if !contains(prompt, "json") {
		t.Error("prompt should request JSON response")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
