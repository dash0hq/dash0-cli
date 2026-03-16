package recordingrulegroups

import "github.com/spf13/cobra"

// NewRecordingRuleGroupsCmd creates the recording-rule-groups parent command.
func NewRecordingRuleGroupsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "recording-rule-groups",
		Aliases: []string{"rrg"},
		Short:   "Manage Dash0 recording rule groups",
		Long:    `Create, list, get, update, and delete recording rule groups in Dash0`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
