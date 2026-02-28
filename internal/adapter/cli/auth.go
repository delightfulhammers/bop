package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/delightfulhammers/bop/internal/platform"
)

// PlatformAuthenticator abstracts platform login/logout for testability.
type PlatformAuthenticator interface {
	Login(ctx context.Context, client *platform.Client, productID string, out io.Writer) (*platform.Credentials, error)
}

// AuthDeps holds dependencies for the auth subcommand tree.
type AuthDeps struct {
	DefaultPlatformURL string
}

// NewAuthCommand creates the "bop auth" subcommand tree.
func NewAuthCommand(deps AuthDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage platform authentication",
	}

	cmd.AddCommand(loginCommand(deps))
	cmd.AddCommand(logoutCommand())
	cmd.AddCommand(whoamiCommand())
	cmd.AddCommand(statusCommand())

	return cmd
}

func loginCommand(deps AuthDeps) *cobra.Command {
	var platformURL string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with the bop platform via GitHub",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			client := platform.NewClient(platformURL, nil)
			creds, err := platform.Login(ctx, client, "bop", out)
			if err != nil {
				return fmt.Errorf("login failed: %w", err)
			}

			if err := platform.SaveCredentials(creds); err != nil {
				return fmt.Errorf("save credentials: %w", err)
			}

			_, _ = fmt.Fprintf(out, "\nLogged in as %s\n", creds.Username)

			// Fetch user info for tier display
			authClient := platform.NewClient(platformURL, creds)
			info, err := authClient.GetUserInfo(ctx)
			if err == nil && info.PlanID != "" {
				_, _ = fmt.Fprintf(out, "Tier: %s\n", info.PlanID)
			}

			return nil
		},
	}

	defaultURL := deps.DefaultPlatformURL
	if defaultURL == "" {
		defaultURL = "https://api.delightfulhammers.com"
	}
	cmd.Flags().StringVar(&platformURL, "platform-url", defaultURL, "Platform API URL")

	return cmd
}

func logoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke tokens and clear stored credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			creds, err := platform.LoadCredentials()
			if err != nil {
				return fmt.Errorf("load credentials: %w", err)
			}
			if creds == nil {
				_, _ = fmt.Fprintln(out, "Not logged in.")
				return nil
			}

			// Best-effort token revocation
			client := platform.NewClient(creds.PlatformURL, creds)
			if err := client.RevokeToken(ctx); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: token revocation failed: %v\n", err)
			}

			if err := platform.ClearCredentials(); err != nil {
				return fmt.Errorf("clear credentials: %w", err)
			}

			_, _ = fmt.Fprintln(out, "Logged out successfully.")
			return nil
		},
	}
}

func whoamiCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show current user, tier, and team memberships",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			creds, err := platform.LoadCredentials()
			if err != nil {
				return fmt.Errorf("load credentials: %w", err)
			}
			if creds == nil {
				_, _ = fmt.Fprintln(out, "Not logged in. Run `bop auth login` to authenticate.")
				return nil
			}

			client := platform.NewClient(creds.PlatformURL, creds).
				WithOnRefresh(persistRefresh)

			info, err := client.GetUserInfo(ctx)
			if err != nil {
				return fmt.Errorf("get user info: %w", err)
			}

			_, _ = fmt.Fprintf(out, "Username:     %s\n", info.Username)
			if info.Email != "" {
				_, _ = fmt.Fprintf(out, "Email:        %s\n", info.Email)
			}
			_, _ = fmt.Fprintf(out, "Tier:         %s\n", displayTier(info.PlanID))
			if len(info.Entitlements) > 0 {
				_, _ = fmt.Fprintf(out, "Entitlements: %s\n", strings.Join(info.Entitlements, ", "))
			}

			teams, err := client.ListTeams(ctx)
			if err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not fetch teams: %v\n", err)
			} else if len(teams) > 0 {
				_, _ = fmt.Fprintf(out, "\nTeams:\n")
				for _, team := range teams {
					_, _ = fmt.Fprintf(out, "  - %s (%s, %s)\n", team.Name, team.Tier, team.Status)
				}
			}

			return nil
		},
	}
}

func statusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show authentication status and token validity",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			creds, err := platform.LoadCredentials()
			if err != nil {
				return fmt.Errorf("load credentials: %w", err)
			}
			if creds == nil {
				_, _ = fmt.Fprintln(out, "Status: not authenticated")
				_, _ = fmt.Fprintln(out, "Run `bop auth login` to authenticate.")
				return nil
			}

			_, _ = fmt.Fprintf(out, "Authenticated: yes\n")
			_, _ = fmt.Fprintf(out, "Username:      %s\n", creds.Username)
			_, _ = fmt.Fprintf(out, "Platform:      %s\n", creds.PlatformURL)

			if creds.IsExpired() {
				_, _ = fmt.Fprintf(out, "Token:         expired (run `bop auth login` to re-authenticate)\n")
			} else {
				remaining := time.Until(creds.ExpiresAt).Truncate(time.Minute)
				_, _ = fmt.Fprintf(out, "Token:         valid (expires in %s)\n", remaining)
			}

			return nil
		},
	}
}

// persistRefresh saves updated credentials after a token refresh.
func persistRefresh(creds *platform.Credentials) {
	_ = platform.SaveCredentials(creds)
}

// displayTier formats a plan ID for human display.
func displayTier(planID string) string {
	switch planID {
	case "pro":
		return "Pro"
	case "", "free":
		return "Free"
	default:
		return planID
	}
}
