package sqlite

import "testing"

func TestBuildDSN(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "memory database unchanged",
			input:    ":memory:",
			expected: ":memory:",
		},
		{
			name:     "plain file path",
			input:    "/path/to/db.sqlite",
			expected: "/path/to/db.sqlite?_busy_timeout=5000",
		},
		{
			name:     "relative file path",
			input:    "data.db",
			expected: "data.db?_busy_timeout=5000",
		},
		{
			name:     "trailing question mark",
			input:    "file.db?",
			expected: "file.db?_busy_timeout=5000",
		},
		{
			name:     "existing query params",
			input:    "file.db?mode=ro",
			expected: "file.db?mode=ro&_busy_timeout=5000",
		},
		{
			name:     "already has busy_timeout",
			input:    "file.db?_busy_timeout=3000",
			expected: "file.db?_busy_timeout=3000",
		},
		{
			name:     "busy_timeout with other params",
			input:    "file.db?mode=ro&_busy_timeout=3000",
			expected: "file.db?mode=ro&_busy_timeout=3000",
		},
		{
			name:     "multiple existing params",
			input:    "file.db?mode=ro&cache=shared",
			expected: "file.db?mode=ro&cache=shared&_busy_timeout=5000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildDSN(tt.input)
			if result != tt.expected {
				t.Errorf("buildDSN(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
