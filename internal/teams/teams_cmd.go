package teams

import "github.com/spf13/cobra"

// NewTeamsCmd creates a new teams command.
func NewTeamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "teams",
		Short: "Manage teams",
		Long:  `Create, list, update, and delete teams in your Dash0 organization.`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())
	cmd.AddCommand(newListMembersCmd())
	cmd.AddCommand(newAddMembersCmd())
	cmd.AddCommand(newRemoveMembersCmd())

	return cmd
}
