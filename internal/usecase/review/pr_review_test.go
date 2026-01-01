package review

import (
	"context"
	"errors"
	"testing"

	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrchestrator_ReviewPR(t *testing.T) {
	t.Run("returns error when RemoteGitHubClient is not configured", func(t *testing.T) {
		orch := &Orchestrator{deps: OrchestratorDeps{
			RemoteGitHubClient: nil,
		}}

		_, err := orch.ReviewPR(context.Background(), PRRequest{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "RemoteGitHubClient")
	})

	t.Run("returns error for invalid PR number", func(t *testing.T) {
		mockClient := &mockRemoteGitHubClient{}
		orch := &Orchestrator{deps: OrchestratorDeps{
			RemoteGitHubClient: mockClient,
		}}

		_, err := orch.ReviewPR(context.Background(), PRRequest{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 0,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "PR number")
	})

	t.Run("returns error for empty owner", func(t *testing.T) {
		mockClient := &mockRemoteGitHubClient{}
		orch := &Orchestrator{deps: OrchestratorDeps{
			RemoteGitHubClient: mockClient,
		}}

		_, err := orch.ReviewPR(context.Background(), PRRequest{
			Owner:    "",
			Repo:     "repo",
			PRNumber: 123,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "owner")
	})

	t.Run("returns error for empty repo", func(t *testing.T) {
		mockClient := &mockRemoteGitHubClient{}
		orch := &Orchestrator{deps: OrchestratorDeps{
			RemoteGitHubClient: mockClient,
		}}

		_, err := orch.ReviewPR(context.Background(), PRRequest{
			Owner:    "owner",
			Repo:     "",
			PRNumber: 123,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "repo")
	})

	t.Run("returns error when metadata fetch fails", func(t *testing.T) {
		mockClient := &mockRemoteGitHubClient{
			metadataErr: errors.New("PR not found"),
		}
		orch := &Orchestrator{deps: OrchestratorDeps{
			RemoteGitHubClient: mockClient,
		}}

		_, err := orch.ReviewPR(context.Background(), PRRequest{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "PR not found")
	})

	t.Run("returns error when diff fetch fails", func(t *testing.T) {
		mockClient := &mockRemoteGitHubClient{
			metadata: &domain.PRMetadata{
				HeadSHA: "abc123",
				BaseSHA: "def456",
			},
			diffErr: errors.New("diff fetch failed"),
		}
		orch := &Orchestrator{deps: OrchestratorDeps{
			RemoteGitHubClient: mockClient,
		}}

		_, err := orch.ReviewPR(context.Background(), PRRequest{
			Owner:    "owner",
			Repo:     "repo",
			PRNumber: 123,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "diff fetch failed")
	})
}

func TestParsePRIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantOwner  string
		wantRepo   string
		wantNumber int
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "parses owner/repo#number format",
			input:      "bkyoung/code-reviewer#172",
			wantOwner:  "bkyoung",
			wantRepo:   "code-reviewer",
			wantNumber: 172,
			wantErr:    false,
		},
		{
			name:       "parses github.com URL",
			input:      "https://github.com/owner/repo/pull/123",
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantNumber: 123,
			wantErr:    false,
		},
		{
			name:       "parses github.com URL without https",
			input:      "github.com/owner/repo/pull/456",
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantNumber: 456,
			wantErr:    false,
		},
		{
			name:       "parses GHE URL",
			input:      "https://github.mycompany.com/team/project/pull/789",
			wantOwner:  "team",
			wantRepo:   "project",
			wantNumber: 789,
			wantErr:    false,
		},
		{
			name:       "handles trailing slash in URL",
			input:      "https://github.com/owner/repo/pull/123/",
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantNumber: 123,
			wantErr:    false,
		},
		{
			name:       "errors on invalid format",
			input:      "not-a-valid-format",
			wantErr:    true,
			wantErrMsg: "invalid PR identifier",
		},
		{
			name:       "errors on missing PR number",
			input:      "owner/repo",
			wantErr:    true,
			wantErrMsg: "invalid PR identifier",
		},
		{
			name:       "errors on non-numeric PR number in shorthand",
			input:      "owner/repo#abc",
			wantErr:    true,
			wantErrMsg: "invalid PR number",
		},
		{
			name:       "errors on empty input",
			input:      "",
			wantErr:    true,
			wantErrMsg: "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, number, err := ParsePRIdentifier(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrMsg != "" {
					assert.Contains(t, err.Error(), tt.wantErrMsg)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOwner, owner)
			assert.Equal(t, tt.wantRepo, repo)
			assert.Equal(t, tt.wantNumber, number)
		})
	}
}

// mockRemoteGitHubClient implements RemoteGitHubClient for testing
type mockRemoteGitHubClient struct {
	metadata    *domain.PRMetadata
	metadataErr error
	diff        domain.Diff
	diffErr     error
	content     string
	contentErr  error
}

func (m *mockRemoteGitHubClient) GetPRMetadata(ctx context.Context, owner, repo string, prNumber int) (*domain.PRMetadata, error) {
	return m.metadata, m.metadataErr
}

func (m *mockRemoteGitHubClient) GetPRDiff(ctx context.Context, owner, repo string, prNumber int) (domain.Diff, error) {
	return m.diff, m.diffErr
}

func (m *mockRemoteGitHubClient) GetFileContent(ctx context.Context, owner, repo, path, ref string) (string, error) {
	return m.content, m.contentErr
}
