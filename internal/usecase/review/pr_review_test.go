package review

import (
	"context"
	"errors"
	"testing"

	"github.com/delightfulhammers/bop/internal/domain"
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

	t.Run("returns error when RepoAccessChecker denies access", func(t *testing.T) {
		mockClient := &mockRemoteGitHubClient{
			metadata: &domain.PRMetadata{
				HeadSHA:   "abc123",
				BaseSHA:   "def456",
				IsPrivate: true,
				OwnerType: "Organization",
			},
		}
		mockChecker := &mockRepoAccessChecker{
			err: errors.New("Private repository access requires Solo plan or higher"),
		}
		orch := &Orchestrator{deps: OrchestratorDeps{
			RemoteGitHubClient: mockClient,
		}}

		_, err := orch.ReviewPR(context.Background(), PRRequest{
			Owner:             "owner",
			Repo:              "repo",
			PRNumber:          123,
			RepoAccessChecker: mockChecker,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "Private repository access requires Solo plan")
		// Verify the checker was called with correct metadata
		assert.True(t, mockChecker.isPrivate)
		assert.Equal(t, "Organization", mockChecker.ownerType)
	})

	t.Run("proceeds when RepoAccessChecker allows access", func(t *testing.T) {
		mockClient := &mockRemoteGitHubClient{
			metadata: &domain.PRMetadata{
				HeadSHA:   "abc123",
				BaseSHA:   "def456",
				IsPrivate: true,
				OwnerType: "User",
			},
			// Diff fetch will fail, but that's after entitlement check succeeds
			diffErr: errors.New("diff fetch failed"),
		}
		mockChecker := &mockRepoAccessChecker{
			err: nil, // Access allowed
		}
		orch := &Orchestrator{deps: OrchestratorDeps{
			RemoteGitHubClient: mockClient,
		}}

		_, err := orch.ReviewPR(context.Background(), PRRequest{
			Owner:             "owner",
			Repo:              "repo",
			PRNumber:          123,
			RepoAccessChecker: mockChecker,
		})

		// Should fail on diff fetch (after entitlement check passed)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "diff fetch failed")
		// Verify the checker was called
		assert.True(t, mockChecker.called)
	})

	t.Run("proceeds when RepoAccessChecker is nil (legacy mode)", func(t *testing.T) {
		mockClient := &mockRemoteGitHubClient{
			metadata: &domain.PRMetadata{
				HeadSHA:   "abc123",
				BaseSHA:   "def456",
				IsPrivate: true,
				OwnerType: "Organization",
			},
			// Diff fetch will fail, but that's after the nil check passes
			diffErr: errors.New("diff fetch failed"),
		}
		orch := &Orchestrator{deps: OrchestratorDeps{
			RemoteGitHubClient: mockClient,
		}}

		_, err := orch.ReviewPR(context.Background(), PRRequest{
			Owner:             "owner",
			Repo:              "repo",
			PRNumber:          123,
			RepoAccessChecker: nil, // Legacy mode - no checker
		})

		// Should fail on diff fetch (entitlement check skipped)
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
			input:      "delightfulhammers/bop#172",
			wantOwner:  "delightfulhammers",
			wantRepo:   "bop",
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

// mockRepoAccessChecker implements RepoAccessChecker for testing
type mockRepoAccessChecker struct {
	err       error  // Error to return from CanAccessRepo
	called    bool   // Whether CanAccessRepo was called
	isPrivate bool   // Captured isPrivate argument
	ownerType string // Captured ownerType argument
}

func (m *mockRepoAccessChecker) CanAccessRepo(isPrivate bool, ownerType string) error {
	m.called = true
	m.isPrivate = isPrivate
	m.ownerType = ownerType
	return m.err
}

func TestParseGitHubRemoteURL(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		// HTTPS formats
		{
			name:      "HTTPS URL with .git suffix",
			input:     "https://github.com/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "HTTPS URL without .git suffix",
			input:     "https://github.com/owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "HTTPS URL with trailing slash",
			input:     "https://github.com/owner/repo/",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		// SSH formats
		{
			name:      "SSH URL with .git suffix",
			input:     "git@github.com:owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "SSH URL without .git suffix",
			input:     "git@github.com:owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		// GHE formats
		{
			name:      "GHE HTTPS URL",
			input:     "https://github.mycompany.com/team/project.git",
			wantOwner: "team",
			wantRepo:  "project",
		},
		{
			name:      "GHE SSH URL",
			input:     "git@github.mycompany.com:team/project.git",
			wantOwner: "team",
			wantRepo:  "project",
		},
		// Edge cases
		{
			name:      "owner with hyphens and numbers",
			input:     "https://github.com/my-org-123/my-repo-456.git",
			wantOwner: "my-org-123",
			wantRepo:  "my-repo-456",
		},
		// Non-GitHub hosts (accepted to support GHE with custom domains)
		{
			name:      "GitLab URL accepted (validation deferred to API)",
			input:     "https://gitlab.com/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "custom GHE domain without github in name",
			input:     "https://git.mycompany.com/team/project.git",
			wantOwner: "team",
			wantRepo:  "project",
		},
		// Error cases
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid URL format",
			input:   "not-a-url",
			wantErr: true,
		},
		{
			name:    "URL with missing repo",
			input:   "https://github.com/owner",
			wantErr: true,
		},
		{
			name:    "URL with extra path segments",
			input:   "https://github.com/owner/repo/extra",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := ParseGitHubRemoteURL(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOwner, owner)
			assert.Equal(t, tt.wantRepo, repo)
		})
	}
}
