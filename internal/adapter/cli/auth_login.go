package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/delightfulhammers/bop/internal/auth"
)

func newLoginCommand(deps AuthDependencies) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with the platform using GitHub",
		Long: `Authenticate with the Delightful Hammers platform using GitHub OAuth.

This command uses the device flow, which works in any environment including
SSH sessions and headless servers. You'll be given a code to enter at GitHub.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogin(cmd, deps, force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force re-authentication even if already logged in")

	return cmd
}

func runLogin(cmd *cobra.Command, deps AuthDependencies, force bool) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	// Check for nil dependencies
	if deps.Client == nil {
		return errors.New("authentication not configured - set auth.service_url in config")
	}
	if deps.TokenStore == nil {
		return errors.New("token store not initialized")
	}

	// Check if already logged in
	existing, err := deps.TokenStore.Load()
	if err == nil && existing != nil && !existing.IsExpired() && !force {
		_, _ = fmt.Fprintf(out, "Already logged in as %s (%s)\n", existing.User.GitHubLogin, existing.User.Email)
		_, _ = fmt.Fprintf(out, "Use --force to re-authenticate\n")
		return nil
	}

	var pollingStarted bool
	callbacks := auth.DeviceFlowCallbacks{
		OnUserCode: func(userCode, verificationURI string) {
			_, _ = fmt.Fprintf(out, "\nTo authenticate, visit:\n")
			_, _ = fmt.Fprintf(out, "  %s\n\n", verificationURI)
			_, _ = fmt.Fprintf(out, "And enter code: %s\n\n", userCode)
		},
		OnPolling: func(attempt int) {
			if !pollingStarted {
				_, _ = fmt.Fprintf(out, "Waiting for authorization...")
				pollingStarted = true
			} else if attempt%6 == 0 {
				// Print a dot every ~30 seconds (6 attempts at 5s intervals)
				_, _ = fmt.Fprintf(out, ".")
			}
		},
		OnSlowDown: func(newInterval time.Duration) {
			// Server asked us to slow down - we comply silently
		},
	}

	// Run device flow
	result, err := auth.RunDeviceFlow(ctx, deps.Client, callbacks)
	if pollingStarted {
		_, _ = fmt.Fprintf(out, "\n")
	}

	if err != nil {
		switch {
		case errors.Is(err, auth.ErrDeviceCodeExpired):
			return fmt.Errorf("authorization timed out - please try again")
		case errors.Is(err, auth.ErrAccessDenied):
			return fmt.Errorf("authorization was denied")
		case errors.Is(err, auth.ErrFlowCanceled):
			return fmt.Errorf("login canceled")
		default:
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	// Store the result
	if err := auth.StoreDeviceFlowResult(deps.TokenStore, result); err != nil {
		return fmt.Errorf("failed to save authentication: %w", err)
	}

	// Success message
	_, _ = fmt.Fprintf(out, "\nLogged in as %s (%s)\n", result.User.Username, result.User.Email)
	if result.User.PlanID != "" && result.User.PlanID != "free" {
		_, _ = fmt.Fprintf(out, "Plan: %s\n", result.User.PlanID)
	}

	return nil
}
