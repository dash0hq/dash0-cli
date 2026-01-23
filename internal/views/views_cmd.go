package views

import "github.com/spf13/cobra"

// NewViewsCmd creates the views parent command
func NewViewsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "views",
		Short: "Manage Dash0 views",
		Long:  `Create, list, update, and delete views in Dash0`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())
	cmd.AddCommand(newExportCmd())

	return cmd
}
