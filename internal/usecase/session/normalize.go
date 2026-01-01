package session

import (
	"regexp"
	"strings"
)

// Pre-compiled regex patterns for URL normalization.
// These are compiled once at package init to avoid repeated compilation overhead.
var (
	protocolPattern = regexp.MustCompile(`^(https?|git|ssh)://`)
	sshColonPattern = regexp.MustCompile(`^([^@]+)@([^:/]+):(.+)$`)
	sshSlashPattern = regexp.MustCompile(`^([^@]+)@([^/]+)/(.+)$`)
)

// NormalizeRemoteURL converts a git remote URL to a canonical form.
// This ensures the same repository produces the same session ID regardless
// of how it was cloned (HTTPS, SSH, different protocols).
//
// Examples:
//   - "https://github.com/owner/repo.git" -> "github.com/owner/repo"
//   - "git@github.com:owner/repo.git" -> "github.com/owner/repo"
//   - "ssh://git@github.com/owner/repo.git" -> "github.com/owner/repo"
//   - "https://github.com/owner/repo" -> "github.com/owner/repo"
//   - "git://github.com/owner/repo.git" -> "github.com/owner/repo"
func NormalizeRemoteURL(url string) string {
	if url == "" {
		return ""
	}

	result := url

	// Normalize to lowercase first for case-insensitive protocol matching
	result = strings.ToLower(result)

	// Remove trailing .git suffix
	result = strings.TrimSuffix(result, ".git")

	// Remove protocol prefixes (case-insensitive due to earlier lowercase)
	// Handle: https://, http://, git://, ssh://
	result = protocolPattern.ReplaceAllString(result, "")

	// Handle user@host patterns - could be either:
	// 1. SSH format with colon: git@host:path -> host/path
	// 2. SSH URL format after protocol strip: git@host/path -> host/path
	if strings.Contains(result, "@") {
		// Try colon format first: user@host:path
		if sshMatch := sshColonPattern.FindStringSubmatch(result); sshMatch != nil {
			result = sshMatch[2] + "/" + sshMatch[3]
		} else if sshMatch := sshSlashPattern.FindStringSubmatch(result); sshMatch != nil {
			// Slash format: user@host/path (from ssh://git@host/path)
			result = sshMatch[2] + "/" + sshMatch[3]
		}
	}

	// Remove any leading slashes
	result = strings.TrimPrefix(result, "/")

	// Remove any trailing slashes
	result = strings.TrimSuffix(result, "/")

	return result
}
