package timeseriesaggregations

import "github.com/spf13/cobra"

// NewTimeSeriesAggregationsCmd creates the time-series-aggregations parent command.
func NewTimeSeriesAggregationsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "time-series-aggregations",
		Aliases: []string{"tsa"},
		Short:   "Manage Dash0 time series aggregations",
		Long: `Create, list, get, update, and delete time series aggregations in Dash0.

Time series aggregations are part of Signal Control and reduce metric
cardinality by re-sampling matching series at a coarser interval.

All time series aggregation endpoints require the organization admin role,
which is stricter than the other asset types.`,
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newCreateCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newDeleteCmd())

	return cmd
}
