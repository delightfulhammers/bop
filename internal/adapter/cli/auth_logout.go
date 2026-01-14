package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/delightfulhammers/bop/internal/auth"
)

func newLogoutCommand(deps AuthDependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Clear stored authentication credentials",
		Long:  `Clear locally stored authentication credentials and optionally revoke the token.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogout(cmd, deps)
		},
	}

	return cmd
}

func runLogout(cmd *cobra.Command, deps AuthDependencies) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	if deps.TokenStore == nil {
		return errors.New("token store not initialized")
	}

	// Load existing auth to get refresh token for revocation
	existing, err := deps.TokenStore.Load()
	if errors.Is(err, auth.ErrNotLoggedIn) {
		_, _ = fmt.Fprintf(out, "Not logged in\n")
		return nil
	}
	if err != nil {
		// If we can't load it, just try to clear it
		if clearErr := deps.TokenStore.Clear(); clearErr != nil {
			return fmt.Errorf("failed to clear credentials: %w", clearErr)
		}
		_, _ = fmt.Fprintf(out, "Logged out (credentials cleared)\n")
		return nil
	}

	// Try to revoke the token if we have a client
	if deps.Client != nil && existing.RefreshToken != "" {
		// Best effort revocation - don't fail logout if this fails
		_ = deps.Client.RevokeToken(ctx, existing.TenantID, existing.RefreshToken)
	}

	// Clear local credentials
	if err := deps.TokenStore.Clear(); err != nil {
		return fmt.Errorf("failed to clear credentials: %w", err)
	}

	_, _ = fmt.Fprintf(out, "Logged out successfully\n")
	return nil
}
