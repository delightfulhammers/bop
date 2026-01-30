package markdown

import "testing"

func TestSanitise(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "unknown",
		},
		{
			name:     "simple string",
			input:    "main",
			expected: "main",
		},
		{
			name:     "spaces replaced",
			input:    "feature branch",
			expected: "feature-branch",
		},
		{
			name:     "uppercase converted",
			input:    "MAIN",
			expected: "main",
		},
		{
			name:     "path traversal blocked",
			input:    "../../../etc/passwd",
			expected: "etc-passwd",
		},
		{
			name:     "null bytes removed",
			input:    "safe\x00path",
			expected: "safepath",
		},
		{
			name:     "unix separators replaced",
			input:    "path/to/branch",
			expected: "path-to-branch",
		},
		{
			name:     "windows separators replaced",
			input:    "path\\to\\branch",
			expected: "path-to-branch",
		},
		{
			name:     "mixed attack patterns",
			input:    "../foo/../bar\x00/baz",
			expected: "foo-bar-baz",
		},
		{
			name:     "only dots results in unknown",
			input:    "..",
			expected: "unknown",
		},
		{
			name:     "consecutive dashes collapsed",
			input:    "a  b  c",
			expected: "a-b-c",
		},
		{
			name:     "leading/trailing dashes trimmed",
			input:    "/leading/trailing/",
			expected: "leading-trailing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitise(tt.input)
			if result != tt.expected {
				t.Errorf("sanitise(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
