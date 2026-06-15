package login

import (
	"time"

	"github.com/spf13/cobra"
)

// DefaultCallbackTimeout is how long `dash0 login` waits for the user to
// complete the authorization flow in their browser.
const DefaultCallbackTimeout = 2 * time.Minute

// NewLoginCmd creates the `dash0 login` command.
func NewLoginCmd() *cobra.Command {
	var (
		apiURL      string
		profileName string
		port        int
		timeout     time.Duration
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate via OAuth 2.0 in the browser",
		Long: `Authenticate to Dash0 via OAuth 2.0 with PKCE.

Opens the system browser to complete the authorization flow, then saves the
resulting access and refresh tokens into a profile. Subsequent commands use
that profile transparently; access tokens are refreshed automatically before
they expire.

This command requires an interactive terminal and cannot be used in CI or
in agent mode — set DASH0_AUTH_TOKEN to a static auth token in those
environments instead.`,
		Example: `  # Log in against the API URL of the active profile
  dash0 login

  # Log in against a specific Dash0 region/tenant
  dash0 login --api-url https://api.eu-west-1.aws.dash0.com

  # Save the resulting tokens under a named profile
  dash0 login --profile staging --api-url https://api.us-west-2.aws.dash0.com`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogin(cmd.Context(), loginOptions{
				APIURL:          apiURL,
				ProfileName:     profileName,
				Port:            port,
				Timeout:         timeout,
				TimeoutExplicit: cmd.Flags().Changed("timeout"),
			})
		},
	}

	cmd.Flags().StringVar(&apiURL, "api-url", "", "Dash0 API URL to authenticate against (overrides the active profile and DASH0_API_URL)")
	cmd.Flags().StringVar(&profileName, "profile", "", "Profile name to save the resulting tokens under (default: the active profile, or \"default\" if no profile exists)")
	cmd.Flags().IntVar(&port, "port", 0, "Local TCP port to use for the OAuth callback listener (default: an OS-assigned ephemeral port)")
	cmd.Flags().DurationVar(&timeout, "timeout", DefaultCallbackTimeout, "How long to wait for the browser callback before aborting")

	return cmd
}

// NewLogoutCmd creates the `dash0 logout` command.
func NewLogoutCmd() *cobra.Command {
	var (
		profileName string
		force       bool
	)

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Clear and revoke the OAuth tokens of a profile",
		Long: `Clear the OAuth access and refresh tokens of a profile and best-effort
revoke them server-side. The profile shell is kept so a future ` + "`dash0 login`" + `
can re-fill it. The active profile is used unless --profile is set.

To remove the profile entirely, use ` + "`dash0 config profiles delete <name>`" + `.
To switch a profile back to a static auth token without OAuth, use
` + "`dash0 config profiles update <name> --oauth=false`" + `.`,
		Example: `  # Log out of the active profile
  dash0 logout

  # Log out of a named profile without confirmation
  dash0 logout --profile staging --force`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogout(cmd.Context(), logoutOptions{
				ProfileName: profileName,
				Force:       force,
			})
		},
	}

	cmd.Flags().StringVar(&profileName, "profile", "", "Profile to log out of (default: the active profile)")
	cmd.Flags().BoolVar(&force, "force", false, "Skip the confirmation prompt")

	return cmd
}
