package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeRemoteURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "HTTPS with .git suffix",
			input:    "https://github.com/owner/repo.git",
			expected: "github.com/owner/repo",
		},
		{
			name:     "HTTPS without .git suffix",
			input:    "https://github.com/owner/repo",
			expected: "github.com/owner/repo",
		},
		{
			name:     "SSH format with .git",
			input:    "git@github.com:owner/repo.git",
			expected: "github.com/owner/repo",
		},
		{
			name:     "SSH format without .git",
			input:    "git@github.com:owner/repo",
			expected: "github.com/owner/repo",
		},
		{
			name:     "SSH protocol URL",
			input:    "ssh://git@github.com/owner/repo.git",
			expected: "github.com/owner/repo",
		},
		{
			name:     "git protocol URL",
			input:    "git://github.com/owner/repo.git",
			expected: "github.com/owner/repo",
		},
		{
			name:     "HTTP URL",
			input:    "http://github.com/owner/repo.git",
			expected: "github.com/owner/repo",
		},
		{
			name:     "GitHub Enterprise HTTPS",
			input:    "https://github.mycompany.com/org/project.git",
			expected: "github.mycompany.com/org/project",
		},
		{
			name:     "GitHub Enterprise SSH",
			input:    "git@github.mycompany.com:org/project.git",
			expected: "github.mycompany.com/org/project",
		},
		{
			name:     "GitLab HTTPS",
			input:    "https://gitlab.com/group/subgroup/repo.git",
			expected: "gitlab.com/group/subgroup/repo",
		},
		{
			name:     "GitLab SSH",
			input:    "git@gitlab.com:group/subgroup/repo.git",
			expected: "gitlab.com/group/subgroup/repo",
		},
		{
			name:     "Bitbucket HTTPS",
			input:    "https://bitbucket.org/owner/repo.git",
			expected: "bitbucket.org/owner/repo",
		},
		{
			name:     "trailing slash removed",
			input:    "https://github.com/owner/repo/",
			expected: "github.com/owner/repo",
		},
		{
			name:     "case normalized to lowercase",
			input:    "https://GitHub.com/Owner/Repo.git",
			expected: "github.com/owner/repo",
		},
		{
			name:     "custom SSH user",
			input:    "user@myserver.com:path/to/repo.git",
			expected: "myserver.com/path/to/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeRemoteURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeRemoteURL_SameRepoSameResult(t *testing.T) {
	// All these should normalize to the same result
	urls := []string{
		"https://github.com/delightfulhammers/bop.git",
		"https://github.com/delightfulhammers/bop",
		"git@github.com:delightfulhammers/bop.git",
		"git@github.com:delightfulhammers/bop",
		"ssh://git@github.com/delightfulhammers/bop.git",
		"git://github.com/delightfulhammers/bop.git",
		"HTTPS://GitHub.com/delightfulhammers/bop.git",
	}

	expected := "github.com/delightfulhammers/bop"

	for _, url := range urls {
		result := NormalizeRemoteURL(url)
		assert.Equal(t, expected, result, "URL: %s", url)
	}
}
