package recordingrulegroups

import "github.com/spf13/cobra"

// NewRecordingRuleGroupsCmd creates the recording-rules parent command.
func NewRecordingRuleGroupsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "recording-rules",
		Aliases: []string{"rr"},
		Short:   "Manage Dash0 recording rules",
		Long:    `Create, list, get, update, and delete recording rules in Dash0`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
