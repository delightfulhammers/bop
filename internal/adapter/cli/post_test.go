package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bkyoung/code-reviewer/internal/adapter/cli"
	"github.com/bkyoung/code-reviewer/internal/usecase/post"
)

// =============================================================================
// Mock Poster
// =============================================================================

type mockFindingsPoster struct {
	result  *post.Result
	err     error
	lastReq *post.Request
}

func (m *mockFindingsPoster) PostFindings(ctx context.Context, req post.Request) (*post.Result, error) {
	m.lastReq = &req
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

// =============================================================================
// CLI Tests
// =============================================================================

func TestPostCommand_Success(t *testing.T) {
	// Create a temp file with findings
	tmpDir := t.TempDir()
	findingsFile := filepath.Join(tmpDir, "findings.json")
	err := os.WriteFile(findingsFile, []byte(`{
		"findings": [
			{"file": "main.go", "lineStart": 10, "lineEnd": 10, "severity": "high", "category": "bug", "description": "Test finding"}
		]
	}`), 0644)
	require.NoError(t, err)

	poster := &mockFindingsPoster{
		result: &post.Result{
			ReviewID:  12345,
			Posted:    1,
			ReviewURL: "https://github.com/owner/repo/pull/1#pullrequestreview-12345",
		},
	}

	var stdout bytes.Buffer
	cmd := cli.PostCommand(poster)
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{findingsFile, "--owner", "owner", "--repo", "repo", "--pr", "1"})

	err = cmd.Execute()
	require.NoError(t, err)

	// Verify output
	assert.Contains(t, stdout.String(), "Posted 1 finding(s)")
	assert.Contains(t, stdout.String(), "owner/repo#1")
	assert.Contains(t, stdout.String(), "https://github.com/owner/repo/pull/1#pullrequestreview-12345")

	// Verify request
	require.NotNil(t, poster.lastReq)
	assert.Equal(t, "owner", poster.lastReq.Owner)
	assert.Equal(t, "repo", poster.lastReq.Repo)
	assert.Equal(t, 1, poster.lastReq.PRNumber)
	assert.Len(t, poster.lastReq.Findings, 1)
}

func TestPostCommand_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	findingsFile := filepath.Join(tmpDir, "findings.json")
	err := os.WriteFile(findingsFile, []byte(`{
		"findings": [
			{"file": "main.go", "lineStart": 10, "severity": "high", "category": "bug", "description": "Test"}
		]
	}`), 0644)
	require.NoError(t, err)

	poster := &mockFindingsPoster{
		result: &post.Result{
			DryRun:    true,
			WouldPost: 1,
		},
	}

	var stdout bytes.Buffer
	cmd := cli.PostCommand(poster)
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{findingsFile, "--owner", "owner", "--repo", "repo", "--pr", "1", "--dry-run"})

	err = cmd.Execute()
	require.NoError(t, err)

	assert.Contains(t, stdout.String(), "Dry run")
	assert.Contains(t, stdout.String(), "would post 1 finding(s)")
	assert.True(t, poster.lastReq.DryRun)
}

func TestPostCommand_ReviewActionOverride(t *testing.T) {
	tmpDir := t.TempDir()
	findingsFile := filepath.Join(tmpDir, "findings.json")
	err := os.WriteFile(findingsFile, []byte(`{"findings": [{"file": "main.go", "lineStart": 10, "severity": "high", "category": "bug", "description": "Test"}]}`), 0644)
	require.NoError(t, err)

	poster := &mockFindingsPoster{
		result: &post.Result{Posted: 1},
	}

	cmd := cli.PostCommand(poster)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{findingsFile, "--owner", "owner", "--repo", "repo", "--pr", "1", "--review-action", "COMMENT"})

	err = cmd.Execute()
	require.NoError(t, err)

	require.NotNil(t, poster.lastReq.ReviewAction)
	assert.Equal(t, "COMMENT", *poster.lastReq.ReviewAction)
}

func TestPostCommand_InvalidReviewAction(t *testing.T) {
	tmpDir := t.TempDir()
	findingsFile := filepath.Join(tmpDir, "findings.json")
	err := os.WriteFile(findingsFile, []byte(`{"findings": []}`), 0644)
	require.NoError(t, err)

	poster := &mockFindingsPoster{}

	cmd := cli.PostCommand(poster)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{findingsFile, "--owner", "owner", "--repo", "repo", "--pr", "1", "--review-action", "INVALID"})

	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid review action")
}

func TestPostCommand_ParseRawFindingsArray(t *testing.T) {
	tmpDir := t.TempDir()
	findingsFile := filepath.Join(tmpDir, "findings.json")
	// Raw array format (not wrapped in object)
	err := os.WriteFile(findingsFile, []byte(`[
		{"file": "main.go", "lineStart": 10, "severity": "high", "category": "bug", "description": "Finding 1"},
		{"file": "util.go", "lineStart": 20, "severity": "medium", "category": "style", "description": "Finding 2"}
	]`), 0644)
	require.NoError(t, err)

	poster := &mockFindingsPoster{
		result: &post.Result{Posted: 2},
	}

	cmd := cli.PostCommand(poster)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{findingsFile, "--owner", "owner", "--repo", "repo", "--pr", "1"})

	err = cmd.Execute()
	require.NoError(t, err)

	assert.Len(t, poster.lastReq.Findings, 2)
}

func TestPostCommand_MissingRequiredFlags(t *testing.T) {
	tmpDir := t.TempDir()
	findingsFile := filepath.Join(tmpDir, "findings.json")
	err := os.WriteFile(findingsFile, []byte(`{"findings": []}`), 0644)
	require.NoError(t, err)

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing owner",
			args:    []string{findingsFile, "--repo", "repo", "--pr", "1"},
			wantErr: "owner",
		},
		{
			name:    "missing repo",
			args:    []string{findingsFile, "--owner", "owner", "--pr", "1"},
			wantErr: "repo",
		},
		{
			name:    "missing pr",
			args:    []string{findingsFile, "--owner", "owner", "--repo", "repo"},
			wantErr: "pr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			poster := &mockFindingsPoster{}
			cmd := cli.PostCommand(poster)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestPostCommand_FileNotFound(t *testing.T) {
	poster := &mockFindingsPoster{}
	cmd := cli.PostCommand(poster)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"/nonexistent/file.json", "--owner", "owner", "--repo", "repo", "--pr", "1"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read file")
}

func TestPostCommand_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	findingsFile := filepath.Join(tmpDir, "findings.json")
	err := os.WriteFile(findingsFile, []byte(`not valid json`), 0644)
	require.NoError(t, err)

	poster := &mockFindingsPoster{}
	cmd := cli.PostCommand(poster)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{findingsFile, "--owner", "owner", "--repo", "repo", "--pr", "1"})

	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse findings")
}

func TestPostCommand_EmptyFindings(t *testing.T) {
	tmpDir := t.TempDir()
	findingsFile := filepath.Join(tmpDir, "findings.json")
	err := os.WriteFile(findingsFile, []byte(`{"findings": []}`), 0644)
	require.NoError(t, err)

	poster := &mockFindingsPoster{
		result: &post.Result{Posted: 0},
	}

	var stdout bytes.Buffer
	cmd := cli.PostCommand(poster)
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{findingsFile, "--owner", "owner", "--repo", "repo", "--pr", "1"})

	err = cmd.Execute()
	require.NoError(t, err)

	assert.Contains(t, stdout.String(), "Posted 0 finding(s)")
	assert.Empty(t, poster.lastReq.Findings)
}

func TestPostCommand_SkippedAndDuplicates(t *testing.T) {
	tmpDir := t.TempDir()
	findingsFile := filepath.Join(tmpDir, "findings.json")
	err := os.WriteFile(findingsFile, []byte(`{"findings": [
		{"file": "main.go", "lineStart": 10, "severity": "high", "category": "bug", "description": "Test"}
	]}`), 0644)
	require.NoError(t, err)

	poster := &mockFindingsPoster{
		result: &post.Result{
			Posted:     1,
			Skipped:    2,
			Duplicates: 1,
			ReviewURL:  "https://github.com/owner/repo/pull/1",
		},
	}

	var stdout bytes.Buffer
	cmd := cli.PostCommand(poster)
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{findingsFile, "--owner", "owner", "--repo", "repo", "--pr", "1"})

	err = cmd.Execute()
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Skipped: 2")
	assert.Contains(t, output, "Duplicates: 1")
}

// =============================================================================
// parseFindings Tests (exported for testing via command)
// =============================================================================

func TestParseFindings_FullReviewOutput(t *testing.T) {
	tmpDir := t.TempDir()
	findingsFile := filepath.Join(tmpDir, "review.json")

	// Full review output format
	data := `{
		"providerName": "anthropic",
		"modelName": "claude-sonnet-4-5",
		"summary": "Found some issues",
		"findings": [
			{
				"id": "f1",
				"file": "main.go",
				"lineStart": 10,
				"lineEnd": 15,
				"severity": "high",
				"category": "security",
				"description": "SQL injection vulnerability"
			}
		],
		"tokensIn": 1000,
		"tokensOut": 500
	}`

	err := os.WriteFile(findingsFile, []byte(data), 0644)
	require.NoError(t, err)

	poster := &mockFindingsPoster{result: &post.Result{Posted: 1}}
	cmd := cli.PostCommand(poster)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{findingsFile, "--owner", "o", "--repo", "r", "--pr", "1"})

	err = cmd.Execute()
	require.NoError(t, err)

	require.Len(t, poster.lastReq.Findings, 1)
	finding := poster.lastReq.Findings[0]
	assert.Equal(t, "main.go", finding.File)
	assert.Equal(t, 10, finding.LineStart)
	assert.Equal(t, 15, finding.LineEnd)
	assert.Equal(t, "high", finding.Severity)
	assert.Equal(t, "security", finding.Category)
}
