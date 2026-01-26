package checkrules

import "github.com/spf13/cobra"

// NewCheckRulesCmd creates the check-rules parent command
func NewCheckRulesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check-rules",
		Short: "Manage Dash0 check rules (alerting rules)",
		Long:  `Create, list, update, and delete check rules (Prometheus alerting rules) in Dash0`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
