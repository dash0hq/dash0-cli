package experimental

import (
	"fmt"

	"github.com/spf13/cobra"
)

// RequireExperimental checks whether the --experimental (-X) flag has been set
// on the command (or any of its parents, since it is a persistent flag).
// It returns a descriptive error when the flag is not set, guiding the user to
// opt in explicitly.
// Note: This function assumes that what is "experimental" is a subcommand, rather
// than a flag on an otherwise non-experimental command.
func RequireExperimental(cmd *cobra.Command) error {
	enabled, err := cmd.Flags().GetBool("experimental")
	if err != nil {
		// Flag not registered — treat as not enabled.
		enabled = false
	}
	if !enabled {
		return fmt.Errorf(
			"%q is an experimental command; pass --experimental (or -X) to enable it",
			cmd.CommandPath(),
		)
	}
	return nil
}

// RequireExperimentalFlag checks whether the --experimental (-X) flag has
// been set, for a single flag on an otherwise-stable command. Unlike
// RequireExperimental, it is a no-op — returning nil — when flagName was not
// passed at all, so the rest of the command remains fully stable and
// ungated. It only requires --experimental once the caller actually uses the
// experimental flag.
func RequireExperimentalFlag(cmd *cobra.Command, flagName string) error {
	if !cmd.Flags().Changed(flagName) {
		return nil
	}
	enabled, err := cmd.Flags().GetBool("experimental")
	if err != nil {
		enabled = false
	}
	if !enabled {
		return fmt.Errorf(
			"--%s is an experimental flag on %q; pass --experimental (or -X) to enable it",
			flagName, cmd.CommandPath(),
		)
	}
	return nil
}
