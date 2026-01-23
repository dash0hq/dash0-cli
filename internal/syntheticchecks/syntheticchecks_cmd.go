package syntheticchecks

import "github.com/spf13/cobra"

// NewSyntheticChecksCmd creates the synthetic-checks parent command
func NewSyntheticChecksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "synthetic-checks",
		Short: "Manage Dash0 synthetic checks",
		Long:  `Create, list, update, and delete synthetic checks in Dash0`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())
	cmd.AddCommand(newApplyCmd())
	cmd.AddCommand(newExportCmd())

	return cmd
}
