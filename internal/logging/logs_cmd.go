package logging

import "github.com/spf13/cobra"

// NewLogsCmd creates a new logs command
func NewLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Send and query log records",
		Long:  `Send and query log records to and from Dash0.`,
	}

	cmd.AddCommand(newSendCmd())
	cmd.AddCommand(newQueryCmd())

	return cmd
}
