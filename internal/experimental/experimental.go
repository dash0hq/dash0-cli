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
