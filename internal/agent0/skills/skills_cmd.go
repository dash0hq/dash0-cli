package skills

import "github.com/spf13/cobra"

const assetType = "Agent0 skill"

// NewSkillsCmd creates the agent0 skills parent command.
func NewSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "[experimental] Manage Agent0 custom skills",
		Long: `Create, list, get, update, and delete the custom skills of Agent0 in your Dash0 organization.

Custom skills belong to the organization, not to a dataset.`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
