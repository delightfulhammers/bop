package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/bkyoung/code-reviewer/internal/domain"
	"github.com/bkyoung/code-reviewer/internal/usecase/session"
)

// SessionManager defines the dependency required to run session commands.
type SessionManager interface {
	ListSessions(ctx context.Context) ([]domain.LocalSession, error)
	Prune(ctx context.Context, opts session.PruneOptions) (*session.PruneResult, error)
	Clean(ctx context.Context, sessionID string) error
}

// SessionsCommand creates the 'cr sessions' command group for managing local review sessions.
func SessionsCommand(sessionManager SessionManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "Manage local review sessions",
		Long: `Manage local review sessions stored in ~/.cache/code-reviewer/sessions.

Sessions track review history per repository/branch pair and enable deduplication
of findings across multiple review runs.`,
	}

	cmd.AddCommand(sessionsListCommand(sessionManager))
	cmd.AddCommand(sessionsPruneCommand(sessionManager))
	cmd.AddCommand(sessionsCleanCommand(sessionManager))

	return cmd
}

// sessionsListCommand creates the 'cr sessions list' subcommand.
func sessionsListCommand(sessionManager SessionManager) *cobra.Command {
	var jsonOutput bool
	var all bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List local review sessions",
		Long: `List all local review sessions.

By default, shows a summary of each session. Use --json for machine-readable output.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			sessions, err := sessionManager.ListSessions(ctx)
			if err != nil {
				return fmt.Errorf("list sessions: %w", err)
			}

			if len(sessions) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No sessions found.")
				return nil
			}

			if jsonOutput {
				return outputSessionsJSON(cmd, sessions)
			}

			return outputSessionsTable(cmd, sessions, all)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVar(&all, "all", false, "Show all details including session IDs")

	return cmd
}

// sessionsPruneCommand creates the 'cr sessions prune' subcommand.
func sessionsPruneCommand(sessionManager SessionManager) *cobra.Command {
	var olderThan string
	var pruneOrphans bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove stale review sessions",
		Long: `Remove stale review sessions based on age or orphaned status.

Orphaned sessions are those whose branch no longer exists. The prune command
first checks the remote for branch existence, falling back to local checks
if the network is unavailable.

Examples:
  cr sessions prune --older-than 30d
  cr sessions prune --orphans
  cr sessions prune --older-than 7d --orphans
  cr sessions prune --older-than 7d --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			opts := session.PruneOptions{
				PruneOrphans: pruneOrphans,
				DryRun:       dryRun,
			}

			if olderThan != "" {
				duration, err := parseDuration(olderThan)
				if err != nil {
					return fmt.Errorf("invalid --older-than value: %w", err)
				}
				opts.OlderThan = duration
			}

			if opts.OlderThan == 0 && !opts.PruneOrphans {
				return fmt.Errorf("must specify --older-than or --orphans (or both)")
			}

			result, err := sessionManager.Prune(ctx, opts)
			if err != nil {
				return fmt.Errorf("prune sessions: %w", err)
			}

			return outputPruneResult(cmd, result)
		},
	}

	cmd.Flags().StringVar(&olderThan, "older-than", "", "Prune sessions older than duration (e.g., 7d, 30d, 2w)")
	cmd.Flags().BoolVar(&pruneOrphans, "orphans", false, "Prune sessions for deleted branches")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be pruned without actually deleting")

	return cmd
}

// sessionsCleanCommand creates the 'cr sessions clean' subcommand.
func sessionsCleanCommand(sessionManager SessionManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean <session-id>",
		Short: "Remove a specific session",
		Long: `Remove a specific session by its ID.

Use 'cr sessions list --all' to see session IDs.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			sessionID := args[0]

			if err := sessionManager.Clean(ctx, sessionID); err != nil {
				return fmt.Errorf("clean session: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Session %s removed.\n", sessionID)
			return nil
		},
	}

	return cmd
}

// Helper functions

func outputSessionsJSON(cmd *cobra.Command, sessions []domain.LocalSession) error {
	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sessions: %w", err)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}

func outputSessionsTable(cmd *cobra.Command, sessions []domain.LocalSession, showAll bool) error {
	w := cmd.OutOrStdout()

	if showAll {
		_, _ = fmt.Fprintf(w, "%-16s  %-40s  %-20s  %-8s  %s\n", "ID", "REPOSITORY", "BRANCH", "REVIEWS", "LAST REVIEW")
		_, _ = fmt.Fprintf(w, "%-16s  %-40s  %-20s  %-8s  %s\n", "----------------", "----------------------------------------", "--------------------", "--------", "-----------")
	} else {
		_, _ = fmt.Fprintf(w, "%-40s  %-20s  %-8s  %s\n", "REPOSITORY", "BRANCH", "REVIEWS", "LAST REVIEW")
		_, _ = fmt.Fprintf(w, "%-40s  %-20s  %-8s  %s\n", "----------------------------------------", "--------------------", "--------", "-----------")
	}

	for _, s := range sessions {
		repo := truncateString(s.Repository, 40)
		branch := truncateString(s.Branch, 20)
		lastReview := formatRelativeTime(s.LastReviewAt)

		if showAll {
			_, _ = fmt.Fprintf(w, "%-16s  %-40s  %-20s  %-8d  %s\n", s.ID, repo, branch, s.ReviewCount, lastReview)
		} else {
			_, _ = fmt.Fprintf(w, "%-40s  %-20s  %-8d  %s\n", repo, branch, s.ReviewCount, lastReview)
		}
	}

	_, _ = fmt.Fprintf(w, "\n%d session(s)\n", len(sessions))
	return nil
}

func outputPruneResult(cmd *cobra.Command, result *session.PruneResult) error {
	w := cmd.OutOrStdout()

	action := "Pruned"
	if result.DryRun {
		action = "Would prune"
	}

	if len(result.Pruned) == 0 {
		_, _ = fmt.Fprintln(w, "No sessions to prune.")
	} else {
		_, _ = fmt.Fprintf(w, "%s %d session(s):\n", action, len(result.Pruned))
		for _, s := range result.Pruned {
			_, _ = fmt.Fprintf(w, "  - %s (%s/%s)\n", s.ID, s.Repository, s.Branch)
		}
	}

	if len(result.Errors) > 0 {
		_, _ = fmt.Fprintf(w, "\n%d error(s):\n", len(result.Errors))
		for _, e := range result.Errors {
			_, _ = fmt.Fprintf(w, "  - %s: %v\n", e.Session.ID, e.Error)
		}
	}

	return nil
}

// Maximum duration for prune operations (5 years).
const maxPruneDuration = 5 * 365 * 24 * time.Hour

// parseDuration parses a duration string like "7d", "30d", "2w" into a time.Duration.
func parseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("duration too short: %s", s)
	}

	unit := s[len(s)-1]
	value := s[:len(s)-1]

	var multiplier time.Duration
	switch unit {
	case 'd':
		multiplier = 24 * time.Hour
	case 'w':
		multiplier = 7 * 24 * time.Hour
	case 'h':
		multiplier = time.Hour
	case 'm':
		multiplier = time.Minute
	default:
		// Try standard Go duration parsing
		return time.ParseDuration(s)
	}

	var n int
	if _, err := fmt.Sscanf(value, "%d", &n); err != nil {
		return 0, fmt.Errorf("invalid number: %s", value)
	}

	if n <= 0 {
		return 0, fmt.Errorf("duration must be positive: %s", s)
	}

	duration := time.Duration(n) * multiplier
	if duration > maxPruneDuration {
		return 0, fmt.Errorf("duration exceeds maximum of 5 years: %s", s)
	}

	return duration, nil
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

func formatRelativeTime(t time.Time) string {
	d := time.Since(t)

	// Handle future timestamps (clock skew or bad data)
	if d < 0 {
		return "in the future"
	}

	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	case d < 30*24*time.Hour:
		weeks := int(d.Hours() / (24 * 7))
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	default:
		months := int(d.Hours() / (24 * 30))
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	}
}
