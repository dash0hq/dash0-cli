package tracing

import "github.com/spf13/cobra"

// NewSpansCmd creates a new spans command.
func NewSpansCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spans",
		Short: "Send and query spans",
		Long:  `Send and query spans to and from Dash0.`,
	}

	cmd.AddCommand(newQueryCmd())
	cmd.AddCommand(newSendCmd())

	return cmd
}
