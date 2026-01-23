package dashboards

import "github.com/spf13/cobra"

// NewDashboardsCmd creates the dashboards parent command
func NewDashboardsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboards",
		Short: "Manage Dash0 dashboards",
		Long:  `Create, list, update, and delete dashboards in Dash0`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())
	cmd.AddCommand(newExportCmd())

	return cmd
}
