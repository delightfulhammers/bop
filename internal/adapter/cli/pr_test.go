package cli

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/delightfulhammers/bop/internal/usecase/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPRReviewer implements PRReviewer for testing
type mockPRReviewer struct {
	lastRequest review.PRRequest
	result      review.Result
	err         error
}

func (m *mockPRReviewer) ReviewPR(ctx context.Context, req review.PRRequest) (review.Result, error) {
	m.lastRequest = req
	return m.result, m.err
}

func TestPRCommand_ParsesShorthandFormat(t *testing.T) {
	mock := &mockPRReviewer{result: review.Result{}}
	var stdout, stderr bytes.Buffer

	cmd := NewRootCommand(Dependencies{
		PRReviewer: mock,
		Args: Arguments{
			OutWriter: &stdout,
			ErrWriter: &stderr,
		},
		DefaultOutput: "test-output",
	})

	cmd.SetArgs([]string{"review", "pr", "owner/repo#123"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "owner", mock.lastRequest.Owner)
	assert.Equal(t, "repo", mock.lastRequest.Repo)
	assert.Equal(t, 123, mock.lastRequest.PRNumber)
	assert.Equal(t, "test-output", mock.lastRequest.OutputDir)
}

func TestPRCommand_ParsesURLFormat(t *testing.T) {
	mock := &mockPRReviewer{result: review.Result{}}
	var stdout, stderr bytes.Buffer

	cmd := NewRootCommand(Dependencies{
		PRReviewer: mock,
		Args: Arguments{
			OutWriter: &stdout,
			ErrWriter: &stderr,
		},
		DefaultOutput: "test-output",
	})

	cmd.SetArgs([]string{"review", "pr", "https://github.com/owner/repo/pull/456"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "owner", mock.lastRequest.Owner)
	assert.Equal(t, "repo", mock.lastRequest.Repo)
	assert.Equal(t, 456, mock.lastRequest.PRNumber)
}

func TestPRCommand_RequiresPRIdentifier(t *testing.T) {
	mock := &mockPRReviewer{}
	var stdout, stderr bytes.Buffer

	cmd := NewRootCommand(Dependencies{
		PRReviewer: mock,
		Args: Arguments{
			OutWriter: &stdout,
			ErrWriter: &stderr,
		},
	})

	cmd.SetArgs([]string{"review", "pr"})
	err := cmd.Execute()

	require.Error(t, err)
	// Cobra's ExactArgs(1) provides its own error message
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestPRCommand_ReturnsReviewError(t *testing.T) {
	mock := &mockPRReviewer{err: errors.New("review failed")}
	var stdout, stderr bytes.Buffer

	cmd := NewRootCommand(Dependencies{
		PRReviewer: mock,
		Args: Arguments{
			OutWriter: &stdout,
			ErrWriter: &stderr,
		},
		DefaultOutput: "out",
	})

	cmd.SetArgs([]string{"review", "pr", "owner/repo#1"})
	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "review failed")
}

func TestPRCommand_InvalidIdentifier(t *testing.T) {
	mock := &mockPRReviewer{}
	var stdout, stderr bytes.Buffer

	cmd := NewRootCommand(Dependencies{
		PRReviewer: mock,
		Args: Arguments{
			OutWriter: &stdout,
			ErrWriter: &stderr,
		},
		DefaultOutput: "out",
	})

	cmd.SetArgs([]string{"review", "pr", "not-a-valid-identifier"})
	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid PR identifier")
}

func TestPRCommand_PostFlag(t *testing.T) {
	mock := &mockPRReviewer{result: review.Result{}}
	var stdout, stderr bytes.Buffer

	cmd := NewRootCommand(Dependencies{
		PRReviewer: mock,
		Args: Arguments{
			OutWriter: &stdout,
			ErrWriter: &stderr,
		},
		DefaultOutput: "test-output",
	})

	cmd.SetArgs([]string{"review", "pr", "--post", "owner/repo#123"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.True(t, mock.lastRequest.PostToGitHub)
}

func TestPRCommand_ReviewersFlag(t *testing.T) {
	mock := &mockPRReviewer{result: review.Result{}}
	var stdout, stderr bytes.Buffer

	cmd := NewRootCommand(Dependencies{
		PRReviewer: mock,
		Args: Arguments{
			OutWriter: &stdout,
			ErrWriter: &stderr,
		},
		DefaultOutput: "test-output",
	})

	cmd.SetArgs([]string{"review", "pr", "--reviewers", "security,architecture", "owner/repo#123"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, []string{"security", "architecture"}, mock.lastRequest.Reviewers)
}

func TestPRCommand_OutputFlag(t *testing.T) {
	mock := &mockPRReviewer{result: review.Result{}}
	var stdout, stderr bytes.Buffer

	cmd := NewRootCommand(Dependencies{
		PRReviewer: mock,
		Args: Arguments{
			OutWriter: &stdout,
			ErrWriter: &stderr,
		},
		DefaultOutput: "default-out",
	})

	cmd.SetArgs([]string{"review", "pr", "--output", "custom-output", "owner/repo#123"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "custom-output", mock.lastRequest.OutputDir)
}

func TestPRCommand_InstructionsFlag(t *testing.T) {
	mock := &mockPRReviewer{result: review.Result{}}
	var stdout, stderr bytes.Buffer

	cmd := NewRootCommand(Dependencies{
		PRReviewer: mock,
		Args: Arguments{
			OutWriter: &stdout,
			ErrWriter: &stderr,
		},
		DefaultOutput:       "out",
		DefaultInstructions: "default instructions",
	})

	cmd.SetArgs([]string{"review", "pr", "--instructions", "custom instructions", "owner/repo#123"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "custom instructions", mock.lastRequest.CustomInstructions)
}

func TestPRCommand_FallsBackToDefaultInstructions(t *testing.T) {
	mock := &mockPRReviewer{result: review.Result{}}
	var stdout, stderr bytes.Buffer

	cmd := NewRootCommand(Dependencies{
		PRReviewer: mock,
		Args: Arguments{
			OutWriter: &stdout,
			ErrWriter: &stderr,
		},
		DefaultOutput:       "out",
		DefaultInstructions: "default instructions from config",
	})

	cmd.SetArgs([]string{"review", "pr", "owner/repo#123"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "default instructions from config", mock.lastRequest.CustomInstructions)
}

func TestPRCommand_NoArchitectureFlag(t *testing.T) {
	mock := &mockPRReviewer{result: review.Result{}}
	var stdout, stderr bytes.Buffer

	cmd := NewRootCommand(Dependencies{
		PRReviewer: mock,
		Args: Arguments{
			OutWriter: &stdout,
			ErrWriter: &stderr,
		},
		DefaultOutput: "out",
	})

	cmd.SetArgs([]string{"review", "pr", "--no-architecture", "owner/repo#123"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.True(t, mock.lastRequest.NoArchitecture)
}

func TestPRCommand_NoAutoContextFlag(t *testing.T) {
	mock := &mockPRReviewer{result: review.Result{}}
	var stdout, stderr bytes.Buffer

	cmd := NewRootCommand(Dependencies{
		PRReviewer: mock,
		Args: Arguments{
			OutWriter: &stdout,
			ErrWriter: &stderr,
		},
		DefaultOutput: "out",
	})

	cmd.SetArgs([]string{"review", "pr", "--no-auto-context", "owner/repo#123"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.True(t, mock.lastRequest.NoAutoContext)
}

// mockGitRemoteResolver implements GitRemoteResolver for testing
type mockGitRemoteResolver struct {
	remoteURL string
	err       error
}

func (m *mockGitRemoteResolver) GetRemoteURL(ctx context.Context) (string, error) {
	return m.remoteURL, m.err
}

func TestPRCommand_NumberShorthand_Success(t *testing.T) {
	prMock := &mockPRReviewer{result: review.Result{}}
	gitMock := &mockGitRemoteResolver{remoteURL: "https://github.com/myorg/myrepo.git"}
	var stdout, stderr bytes.Buffer

	cmd := NewRootCommand(Dependencies{
		PRReviewer:        prMock,
		GitRemoteResolver: gitMock,
		Args: Arguments{
			OutWriter: &stdout,
			ErrWriter: &stderr,
		},
		DefaultOutput: "out",
	})

	cmd.SetArgs([]string{"review", "pr", "42"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "myorg", prMock.lastRequest.Owner)
	assert.Equal(t, "myrepo", prMock.lastRequest.Repo)
	assert.Equal(t, 42, prMock.lastRequest.PRNumber)
}

func TestPRCommand_NumberShorthand_SSHRemote(t *testing.T) {
	prMock := &mockPRReviewer{result: review.Result{}}
	gitMock := &mockGitRemoteResolver{remoteURL: "git@github.com:owner/repo.git"}
	var stdout, stderr bytes.Buffer

	cmd := NewRootCommand(Dependencies{
		PRReviewer:        prMock,
		GitRemoteResolver: gitMock,
		Args: Arguments{
			OutWriter: &stdout,
			ErrWriter: &stderr,
		},
		DefaultOutput: "out",
	})

	cmd.SetArgs([]string{"review", "pr", "123"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "owner", prMock.lastRequest.Owner)
	assert.Equal(t, "repo", prMock.lastRequest.Repo)
	assert.Equal(t, 123, prMock.lastRequest.PRNumber)
}

func TestPRCommand_NumberShorthand_NoResolver(t *testing.T) {
	prMock := &mockPRReviewer{result: review.Result{}}
	var stdout, stderr bytes.Buffer

	cmd := NewRootCommand(Dependencies{
		PRReviewer:        prMock,
		GitRemoteResolver: nil, // No resolver
		Args: Arguments{
			OutWriter: &stdout,
			ErrWriter: &stderr,
		},
		DefaultOutput: "out",
	})

	cmd.SetArgs([]string{"review", "pr", "123"})
	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "owner/repo#123")
}

func TestPRCommand_NumberShorthand_NoRemote(t *testing.T) {
	prMock := &mockPRReviewer{result: review.Result{}}
	gitMock := &mockGitRemoteResolver{remoteURL: ""} // No remote
	var stdout, stderr bytes.Buffer

	cmd := NewRootCommand(Dependencies{
		PRReviewer:        prMock,
		GitRemoteResolver: gitMock,
		Args: Arguments{
			OutWriter: &stdout,
			ErrWriter: &stderr,
		},
		DefaultOutput: "out",
	})

	cmd.SetArgs([]string{"review", "pr", "123"})
	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no git remote found")
}

func TestPRCommand_NumberShorthand_RemoteError(t *testing.T) {
	prMock := &mockPRReviewer{result: review.Result{}}
	gitMock := &mockGitRemoteResolver{err: errors.New("git error")}
	var stdout, stderr bytes.Buffer

	cmd := NewRootCommand(Dependencies{
		PRReviewer:        prMock,
		GitRemoteResolver: gitMock,
		Args: Arguments{
			OutWriter: &stdout,
			ErrWriter: &stderr,
		},
		DefaultOutput: "out",
	})

	cmd.SetArgs([]string{"review", "pr", "123"})
	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "git error")
}

func TestPRCommand_NumberShorthand_AnyRemoteAccepted(t *testing.T) {
	// Non-GitHub remotes are accepted to support GHE with custom domains.
	// Validation is deferred to the GitHub API call.
	prMock := &mockPRReviewer{result: review.Result{}}
	gitMock := &mockGitRemoteResolver{remoteURL: "https://gitlab.com/owner/repo.git"}
	var stdout, stderr bytes.Buffer

	cmd := NewRootCommand(Dependencies{
		PRReviewer:        prMock,
		GitRemoteResolver: gitMock,
		Args: Arguments{
			OutWriter: &stdout,
			ErrWriter: &stderr,
		},
		DefaultOutput: "out",
	})

	cmd.SetArgs([]string{"review", "pr", "123"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "owner", prMock.lastRequest.Owner)
	assert.Equal(t, "repo", prMock.lastRequest.Repo)
	assert.Equal(t, 123, prMock.lastRequest.PRNumber)
}
