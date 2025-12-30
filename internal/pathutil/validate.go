// Package pathutil provides utilities for path validation and normalization.
package pathutil

import (
	"errors"
	"path/filepath"
	"strings"
)

// ValidateRelativePath normalizes and validates that a path is safe for use
// as a repository-relative path. It returns the cleaned path with forward
// slashes as separators, or an error if the path is invalid.
//
// The function rejects:
//   - Empty paths
//   - Absolute paths (Unix-style leading "/" or Windows drive letters)
//   - Path traversal attempts ("../" components that escape the root)
//
// Valid paths are normalized:
//   - Backslashes converted to forward slashes
//   - Redundant slashes removed
//   - "." components resolved
func ValidateRelativePath(path string) (string, error) {
	if path == "" {
		return "", errors.New("path cannot be empty")
	}

	// Normalize backslashes to forward slashes first (for cross-platform consistency)
	// This must happen before filepath.Clean because on Unix, backslashes are
	// valid filename characters and won't be converted by filepath functions.
	normalized := strings.ReplaceAll(path, "\\", "/")

	// Clean the path to handle ".", "..", and redundant separators
	// Then ensure forward slashes (for Windows where Clean uses backslashes)
	clean := filepath.ToSlash(filepath.Clean(normalized))

	// Check for traversal escape (path starts with ".." after cleaning)
	if strings.HasPrefix(clean, "..") {
		return "", errors.New("path escapes repository root")
	}

	// Check for absolute paths (Unix-style)
	if strings.HasPrefix(clean, "/") {
		return "", errors.New("path must be relative (no leading '/')")
	}

	// Check for Windows drive letters (e.g., "C:" or "c:")
	// After ToSlash, "C:\foo" becomes "C:/foo"
	if len(clean) >= 2 && clean[1] == ':' {
		return "", errors.New("path must be relative (no drive letter)")
	}

	return clean, nil
}
