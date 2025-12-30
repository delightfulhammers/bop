package pathutil

import (
	"testing"
)

func TestValidateRelativePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
		errMsg  string
	}{
		// Valid paths
		{
			name: "simple file",
			path: "file.go",
			want: "file.go",
		},
		{
			name: "nested path",
			path: "internal/adapter/github/client.go",
			want: "internal/adapter/github/client.go",
		},
		{
			name: "path with dot component",
			path: "foo/./bar/file.go",
			want: "foo/bar/file.go",
		},
		{
			name: "path with redundant slashes normalized",
			path: "foo//bar///file.go",
			want: "foo/bar/file.go",
		},
		{
			name: "windows backslashes normalized",
			path: "foo\\bar\\file.go",
			want: "foo/bar/file.go",
		},
		{
			name: "current directory",
			path: ".",
			want: ".",
		},

		// Empty path
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
			errMsg:  "empty",
		},

		// Path traversal attacks
		{
			name:    "simple parent traversal",
			path:    "../secret.txt",
			wantErr: true,
			errMsg:  "escapes",
		},
		{
			name:    "nested parent traversal",
			path:    "foo/../../secret.txt",
			wantErr: true,
			errMsg:  "escapes",
		},
		{
			name:    "traversal that cleans to parent",
			path:    "foo/bar/../../../secret.txt",
			wantErr: true,
			errMsg:  "escapes",
		},
		{
			name:    "just double dots",
			path:    "..",
			wantErr: true,
			errMsg:  "escapes",
		},

		// Absolute paths
		{
			name:    "unix absolute path",
			path:    "/etc/passwd",
			wantErr: true,
			errMsg:  "relative",
		},
		{
			name:    "windows absolute path",
			path:    "C:\\Windows\\System32",
			wantErr: true,
			errMsg:  "drive",
		},
		{
			name:    "windows lowercase drive",
			path:    "c:/users/file.txt",
			wantErr: true,
			errMsg:  "drive",
		},
		{
			name:    "windows drive with backslash",
			path:    "D:\\data\\file.txt",
			wantErr: true,
			errMsg:  "drive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateRelativePath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateRelativePath(%q) expected error containing %q, got nil", tt.path, tt.errMsg)
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateRelativePath(%q) error = %q, want error containing %q", tt.path, err.Error(), tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Errorf("ValidateRelativePath(%q) unexpected error: %v", tt.path, err)
				return
			}
			if got != tt.want {
				t.Errorf("ValidateRelativePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// contains checks if s contains substr (case-insensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsLower(toLower(s), toLower(substr))))
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func containsLower(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
