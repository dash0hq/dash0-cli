package spamfilters

import "github.com/spf13/cobra"

// NewSpamFiltersCmd creates the spam-filters parent command
func NewSpamFiltersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spam-filters",
		Short: "[experimental] Manage Dash0 spam filters",
		Long:  `Create, list, get, update, and delete spam filters in Dash0`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
