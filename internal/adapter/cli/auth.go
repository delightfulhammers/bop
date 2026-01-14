package cli

import (
	"github.com/spf13/cobra"

	"github.com/delightfulhammers/bop/internal/auth"
)

// AuthDependencies captures the collaborators for auth commands.
type AuthDependencies struct {
	Client     *auth.Client
	TokenStore *auth.TokenStore
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
