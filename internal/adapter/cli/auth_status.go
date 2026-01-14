package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/delightfulhammers/bop/internal/auth"
)

func newStatusCommand(deps AuthDependencies) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current authentication status",
		Long:  `Display information about the current authentication state including user, plan, and token expiry.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, deps, jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	return cmd
}

func runStatus(cmd *cobra.Command, deps AuthDependencies, jsonOutput bool) error {
	out := cmd.OutOrStdout()

	if deps.TokenStore == nil {
		return errors.New("token store not initialized")
	}

	// Load existing auth
	stored, err := deps.TokenStore.Load()
	if errors.Is(err, auth.ErrNotLoggedIn) {
		if jsonOutput {
			_, _ = fmt.Fprintf(out, `{"logged_in": false}`+"\n")
		} else {
			_, _ = fmt.Fprintf(out, "Not logged in\n")
			_, _ = fmt.Fprintf(out, "Run 'bop auth login' to authenticate\n")
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to load auth: %w", err)
	}

	if jsonOutput {
		return outputStatusJSON(out, stored)
	}

	return outputStatusText(out, stored)
}

func outputStatusText(out interface{ Write([]byte) (int, error) }, stored *auth.StoredAuth) error {
	_, _ = fmt.Fprintf(out, "Logged in as: %s\n", stored.User.GitHubLogin)
	_, _ = fmt.Fprintf(out, "Email: %s\n", stored.User.Email)
	_, _ = fmt.Fprintf(out, "User ID: %s\n", stored.User.ID)
	_, _ = fmt.Fprintf(out, "Tenant ID: %s\n", stored.TenantID)

	// Plan info
	plan := stored.Plan
	if plan == "" {
		plan = "free"
	}
	_, _ = fmt.Fprintf(out, "Plan: %s\n", plan)

	// Entitlements
	if len(stored.Entitlements) > 0 {
		_, _ = fmt.Fprintf(out, "Entitlements: %s\n", strings.Join(stored.Entitlements, ", "))
	} else {
		_, _ = fmt.Fprintf(out, "Entitlements: (platform not enforcing)\n")
	}

	// Token status
	_, _ = fmt.Fprintf(out, "\nToken Status:\n")
	if stored.IsExpired() {
		_, _ = fmt.Fprintf(out, "  Status: EXPIRED\n")
		_, _ = fmt.Fprintf(out, "  Run 'bop auth login' to re-authenticate\n")
	} else if stored.NeedsRefresh() {
		_, _ = fmt.Fprintf(out, "  Status: Needs refresh (expires soon)\n")
		_, _ = fmt.Fprintf(out, "  Expires: %s\n", stored.ExpiresAt.Format(time.RFC3339))
	} else {
		_, _ = fmt.Fprintf(out, "  Status: Valid\n")
		_, _ = fmt.Fprintf(out, "  Expires: %s\n", stored.ExpiresAt.Format(time.RFC3339))
		remaining := time.Until(stored.ExpiresAt).Round(time.Minute)
		_, _ = fmt.Fprintf(out, "  Remaining: %s\n", remaining)
	}

	return nil
}

func outputStatusJSON(out interface{ Write([]byte) (int, error) }, stored *auth.StoredAuth) error {
	// Simple JSON output without importing encoding/json to avoid complexity
	// For a full implementation, we'd use json.Marshal
	expired := stored.IsExpired()
	needsRefresh := stored.NeedsRefresh()

	entitlements := "[]"
	if len(stored.Entitlements) > 0 {
		quoted := make([]string, len(stored.Entitlements))
		for i, e := range stored.Entitlements {
			quoted[i] = fmt.Sprintf("%q", e)
		}
		entitlements = "[" + strings.Join(quoted, ", ") + "]"
	}

	plan := stored.Plan
	if plan == "" {
		plan = "free"
	}

	_, _ = fmt.Fprintf(out, `{
  "logged_in": true,
  "user": {
    "id": %q,
    "github_login": %q,
    "email": %q
  },
  "tenant_id": %q,
  "plan": %q,
  "entitlements": %s,
  "token": {
    "expired": %t,
    "needs_refresh": %t,
    "expires_at": %q
  }
}
`, stored.User.ID, stored.User.GitHubLogin, stored.User.Email,
		stored.TenantID, plan, entitlements,
		expired, needsRefresh, stored.ExpiresAt.Format(time.RFC3339))

	return nil
}
