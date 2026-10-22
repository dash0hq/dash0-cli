package datasets

import "github.com/spf13/cobra"

// NewDatasetsCmd creates the datasets parent command.
func NewDatasetsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "datasets",
		Short: "[experimental] Manage datasets",
		Long:  `Create, list, get, update, and delete datasets in your Dash0 organization.`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
