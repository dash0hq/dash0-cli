package metrics

import (
	"github.com/spf13/cobra"
)

// NewMetricsCmd creates the metrics command group.
func NewMetricsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Query Dash0 metrics",
		Long:  `Query metrics from the Dash0 API`,
	}

	cmd.AddCommand(newInstantCmd())

	return cmd
}
