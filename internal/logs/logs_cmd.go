package logs

import "github.com/spf13/cobra"

// NewLogsCmd creates a new logs command
func NewLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Manage log records",
		Long:  `Manage log records to Dash0 via OTLP`,
	}

	cmd.AddCommand(newCreateCmd())

	return cmd
}
