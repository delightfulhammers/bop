package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/delightfulhammers/bop/internal/auth"
)

// AuthDependencies captures the collaborators for auth commands.
type AuthDependencies struct {
	Client       *auth.Client
	TokenStore   *auth.TokenStore
	PlatformMode bool // Explicit flag to enable platform auth enforcement
}

// RequireAuth loads and validates auth from the token store.
// Returns an EntitlementChecker on success.
// In legacy mode (PlatformMode=false), returns (nil, nil) - no auth required.
func (d AuthDependencies) RequireAuth() (*auth.EntitlementChecker, error) {
	if !d.PlatformMode {
		return nil, nil // Legacy mode - no platform auth configured
	}

	if d.TokenStore == nil {
		return nil, fmt.Errorf("platform mode enabled but token store not configured")
	}

	stored, err := d.TokenStore.Load()
	if err != nil {
		if errors.Is(err, auth.ErrNotLoggedIn) {
			return nil, fmt.Errorf("not authenticated - run 'bop auth login' first")
		}
		return nil, fmt.Errorf("auth error: %w", err)
	}

	if stored.IsExpired() {
		return nil, fmt.Errorf("authentication expired - run 'bop auth login' to re-authenticate")
	}

	return auth.NewEntitlementChecker(stored), nil
}

// NewAuthCommand creates the auth command group.
func NewAuthCommand(deps AuthDependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage platform authentication",
		Long: `Manage authentication with the Delightful Hammers platform.

Use 'bop auth login' to authenticate using GitHub.
Use 'bop auth logout' to clear stored credentials.
Use 'bop auth status' to check your current authentication state.`,
	}

	cmd.AddCommand(newLoginCommand(deps))
	cmd.AddCommand(newLogoutCommand(deps))
	cmd.AddCommand(newStatusCommand(deps))

	return cmd
}
