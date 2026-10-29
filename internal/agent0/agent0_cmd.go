package agent0

import (
	"github.com/dash0hq/dash0-cli/internal/agent0/skills"
	"github.com/spf13/cobra"
)

// NewAgent0Cmd creates the agent0 parent command.
func NewAgent0Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent0",
		Short: "[experimental] Manage Agent0, the Dash0 AI agent",
		Long:  `Manage the custom skills of Agent0, the Dash0 AI agent, in your Dash0 organization.`,
	}

	cmd.AddCommand(skills.NewSkillsCmd())

	return cmd
}
