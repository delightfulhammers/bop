package cli

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/delightfulhammers/bop/internal/domain"
	"github.com/delightfulhammers/bop/internal/usecase/session"
)

// mockSessionManager implements SessionManager for testing.
type mockSessionManager struct {
	sessions    []domain.LocalSession
	pruneResult *session.PruneResult
	cleanErr    error
}

func (m *mockSessionManager) ListSessions(ctx context.Context) ([]domain.LocalSession, error) {
	return m.sessions, nil
}

func (m *mockSessionManager) Prune(ctx context.Context, opts session.PruneOptions) (*session.PruneResult, error) {
	if m.pruneResult != nil {
		m.pruneResult.DryRun = opts.DryRun
		return m.pruneResult, nil
	}
	return &session.PruneResult{DryRun: opts.DryRun}, nil
}

func (m *mockSessionManager) Clean(ctx context.Context, sessionID string) error {
	return m.cleanErr
}

func TestSessionsListCommand(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		manager := &mockSessionManager{sessions: []domain.LocalSession{}}
		cmd := SessionsCommand(manager)

		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"list"})

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, out.String(), "No sessions found")
	})

	t.Run("with sessions", func(t *testing.T) {
		manager := &mockSessionManager{
			sessions: []domain.LocalSession{
				{
					ID:           "abc123",
					Repository:   "github.com/owner/repo",
					Branch:       "main",
					ReviewCount:  5,
					LastReviewAt: time.Now().Add(-1 * time.Hour),
				},
			},
		}
		cmd := SessionsCommand(manager)

		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"list"})

		err := cmd.Execute()
		require.NoError(t, err)

		output := out.String()
		assert.Contains(t, output, "github.com/owner/repo")
		assert.Contains(t, output, "main")
		assert.Contains(t, output, "5")
		assert.Contains(t, output, "1 session(s)")
	})

	t.Run("with --all flag", func(t *testing.T) {
		manager := &mockSessionManager{
			sessions: []domain.LocalSession{
				{
					ID:           "abc123",
					Repository:   "github.com/owner/repo",
					Branch:       "main",
					ReviewCount:  5,
					LastReviewAt: time.Now(),
				},
			},
		}
		cmd := SessionsCommand(manager)

		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"list", "--all"})

		err := cmd.Execute()
		require.NoError(t, err)

		output := out.String()
		assert.Contains(t, output, "abc123") // Session ID shown with --all
	})

	t.Run("with --json flag", func(t *testing.T) {
		manager := &mockSessionManager{
			sessions: []domain.LocalSession{
				{
					ID:           "abc123",
					Repository:   "github.com/owner/repo",
					Branch:       "main",
					ReviewCount:  5,
					LastReviewAt: time.Now(),
				},
			},
		}
		cmd := SessionsCommand(manager)

		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"list", "--json"})

		err := cmd.Execute()
		require.NoError(t, err)

		output := out.String()
		assert.Contains(t, output, `"id": "abc123"`)
		assert.Contains(t, output, `"repository": "github.com/owner/repo"`)
	})
}

func TestSessionsPruneCommand(t *testing.T) {
	t.Run("requires --older-than or --orphans", func(t *testing.T) {
		manager := &mockSessionManager{}
		cmd := SessionsCommand(manager)

		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		cmd.SetArgs([]string{"prune"})

		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must specify --older-than or --orphans")
	})

	t.Run("with --older-than", func(t *testing.T) {
		manager := &mockSessionManager{
			pruneResult: &session.PruneResult{
				Pruned: []domain.LocalSession{
					{
						ID:         "abc123",
						Repository: "github.com/owner/repo",
						Branch:     "old-branch",
					},
				},
			},
		}
		cmd := SessionsCommand(manager)

		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"prune", "--older-than", "30d"})

		err := cmd.Execute()
		require.NoError(t, err)

		output := out.String()
		assert.Contains(t, output, "Pruned 1 session(s)")
		assert.Contains(t, output, "abc123")
	})

	t.Run("with --dry-run", func(t *testing.T) {
		manager := &mockSessionManager{
			pruneResult: &session.PruneResult{
				Pruned: []domain.LocalSession{
					{
						ID:         "abc123",
						Repository: "github.com/owner/repo",
						Branch:     "old-branch",
					},
				},
			},
		}
		cmd := SessionsCommand(manager)

		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"prune", "--older-than", "30d", "--dry-run"})

		err := cmd.Execute()
		require.NoError(t, err)

		output := out.String()
		assert.Contains(t, output, "Would prune 1 session(s)")
	})

	t.Run("no sessions to prune", func(t *testing.T) {
		manager := &mockSessionManager{
			pruneResult: &session.PruneResult{
				Pruned: []domain.LocalSession{},
			},
		}
		cmd := SessionsCommand(manager)

		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"prune", "--older-than", "30d"})

		err := cmd.Execute()
		require.NoError(t, err)

		output := out.String()
		assert.Contains(t, output, "No sessions to prune")
	})

	t.Run("with --orphans", func(t *testing.T) {
		manager := &mockSessionManager{
			pruneResult: &session.PruneResult{
				Pruned: []domain.LocalSession{},
			},
		}
		cmd := SessionsCommand(manager)

		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"prune", "--orphans"})

		err := cmd.Execute()
		require.NoError(t, err)
	})
}

func TestSessionsCleanCommand(t *testing.T) {
	t.Run("requires session ID", func(t *testing.T) {
		manager := &mockSessionManager{}
		cmd := SessionsCommand(manager)

		cmd.SetArgs([]string{"clean"})

		err := cmd.Execute()
		require.Error(t, err)
	})

	t.Run("removes session", func(t *testing.T) {
		manager := &mockSessionManager{}
		cmd := SessionsCommand(manager)

		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"clean", "abc123"})

		err := cmd.Execute()
		require.NoError(t, err)

		output := out.String()
		assert.Contains(t, output, "abc123 removed")
	})
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{
			name:     "days",
			input:    "7d",
			expected: 7 * 24 * time.Hour,
		},
		{
			name:     "weeks",
			input:    "2w",
			expected: 14 * 24 * time.Hour,
		},
		{
			name:     "hours",
			input:    "24h",
			expected: 24 * time.Hour,
		},
		{
			name:     "minutes",
			input:    "30m",
			expected: 30 * time.Minute,
		},
		{
			name:     "standard go duration seconds",
			input:    "90s",
			expected: 90 * time.Second,
		},
		{
			name:    "invalid - too short",
			input:   "1",
			wantErr: true,
		},
		{
			name:    "invalid - negative",
			input:   "-7d",
			wantErr: true,
		},
		{
			name:    "invalid - zero",
			input:   "0d",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseDuration(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestFormatRelativeTime(t *testing.T) {
	tests := []struct {
		name     string
		offset   time.Duration
		expected string
	}{
		{
			name:     "just now",
			offset:   0,
			expected: "just now",
		},
		{
			name:     "minutes",
			offset:   -5 * time.Minute,
			expected: "5 minutes ago",
		},
		{
			name:     "1 hour",
			offset:   -1 * time.Hour,
			expected: "1 hour ago",
		},
		{
			name:     "hours",
			offset:   -3 * time.Hour,
			expected: "3 hours ago",
		},
		{
			name:     "1 day",
			offset:   -24 * time.Hour,
			expected: "1 day ago",
		},
		{
			name:     "days",
			offset:   -3 * 24 * time.Hour,
			expected: "3 days ago",
		},
		{
			name:     "1 week",
			offset:   -7 * 24 * time.Hour,
			expected: "1 week ago",
		},
		{
			name:     "weeks",
			offset:   -14 * 24 * time.Hour,
			expected: "2 weeks ago",
		},
		{
			name:     "1 month",
			offset:   -30 * 24 * time.Hour,
			expected: "1 month ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatRelativeTime(time.Now().Add(tt.offset))
			assert.Equal(t, tt.expected, result)
		})
	}
}
