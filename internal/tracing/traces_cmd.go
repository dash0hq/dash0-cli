package tracing

import "github.com/spf13/cobra"

// NewTracesCmd creates a new traces command.
func NewTracesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "traces",
		Short: "Query and inspect traces",
		Long:  `Query and inspect traces in Dash0.`,
	}

	cmd.AddCommand(newGetCmd())

	return cmd
}
