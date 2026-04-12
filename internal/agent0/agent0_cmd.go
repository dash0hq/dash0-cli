package agent0

import (
	"github.com/spf13/cobra"
)

// NewAgent0Cmd creates the top-level "agent0" command group.
func NewAgent0Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent0",
		Short: "[experimental] Interact with Agent0",
		Long:  "Commands for interacting with Agent0, Dash0's AI observability assistant.",
	}

	cmd.AddCommand(newChatCmd())

	return cmd
}
