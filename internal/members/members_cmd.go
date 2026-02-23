package members

import "github.com/spf13/cobra"

// NewMembersCmd creates a new members command.
func NewMembersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "members",
		Short: "Manage organization members",
		Long:  `List, invite, and remove members in your Dash0 organization.`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newInviteCmd())
	cmd.AddCommand(newRemoveCmd())

	return cmd
}
