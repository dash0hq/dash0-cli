package timeseriesaggregations

import (
	"context"
	"fmt"
	"os"
	"strconv"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var flags asset.ListFlags

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List time series aggregations",
		Long:    `List all time series aggregations in the specified dataset.` + internal.CONFIG_HINT,
		Example: `  # List time series aggregations (default: up to 50)
  dash0 tsa list

  # Output as YAML for backup or version control
  dash0 tsa list -o yaml > aggregations.yaml

  # Output as JSON for scripting
  dash0 tsa list -o json

  # Output as CSV (pipe-friendly)
  dash0 tsa list -o csv

  # List without the header row (pipe-friendly)
  dash0 tsa list --skip-header`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context(), &flags)
		},
	}

	asset.RegisterListFlags(cmd, &flags)
	return cmd
}

func runList(ctx context.Context, flags *asset.ListFlags) error {
	if err := output.ValidateSkipHeader(flags.SkipHeader, flags.Output); err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)
	items, err := apiClient.ListTimeSeriesAggregations(ctx, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType})
	}

	if !flags.All && flags.Limit > 0 && len(items) > flags.Limit {
		items = items[:flags.Limit]
	}

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout, output.WithSkipHeader(flags.SkipHeader))

	switch format {
	case output.FormatJSON:
		return formatter.PrintJSON(toInterfaceSlice(items))
	case output.FormatYAML:
		return formatter.PrintMultiDocYAML(toInterfaceSlice(items))
	default:
		return printTable(formatter, items, format)
	}
}

func toInterfaceSlice(items []*dash0api.TimeSeriesAggregationDefinition) []interface{} {
	result := make([]interface{}, len(items))
	for i, a := range items {
		result[i] = a
	}
	return result
}

// printTable renders the table, wide, and CSV formats.
//
// There is no URL column, unlike dashboards or views: the API client ships no
// deeplink helper for this asset kind, so there is no URL to render. Spam
// filters are in the same position and their wide output stops at ORIGIN.
func printTable(f *output.Formatter, items []*dash0api.TimeSeriesAggregationDefinition, format output.Format) error {
	columns := []output.Column{
		{Header: internal.HEADER_NAME, Width: 40, Value: func(item interface{}) string {
			return dash0api.GetTimeSeriesAggregationName(item.(*dash0api.TimeSeriesAggregationDefinition))
		}},
		{Header: internal.HEADER_ID, Width: 36, Value: func(item interface{}) string {
			return dash0api.GetTimeSeriesAggregationID(item.(*dash0api.TimeSeriesAggregationDefinition))
		}},
		{Header: internal.HEADER_INTERVAL, Width: 10, Value: func(item interface{}) string {
			return interval(item.(*dash0api.TimeSeriesAggregationDefinition))
		}},
		{Header: internal.HEADER_ENABLED, Width: 8, Value: func(item interface{}) string {
			return strconv.FormatBool(item.(*dash0api.TimeSeriesAggregationDefinition).Spec.Enabled)
		}},
	}

	if format == output.FormatWide || format == output.FormatCSV {
		columns = append(columns,
			output.Column{Header: internal.HEADER_DATASET, Width: 15, Value: func(item interface{}) string {
				return dash0api.GetTimeSeriesAggregationDataset(item.(*dash0api.TimeSeriesAggregationDefinition))
			}},
			output.Column{Header: internal.HEADER_ORIGIN, Width: 30, Value: func(item interface{}) string {
				return origin(item.(*dash0api.TimeSeriesAggregationDefinition))
			}},
		)
	}

	if len(items) == 0 {
		fmt.Println("No time series aggregations found.")
		return nil
	}

	return printRows(f, columns, items, format)
}

func printRows(f *output.Formatter, columns []output.Column, items []*dash0api.TimeSeriesAggregationDefinition, format output.Format) error {
	data := toInterfaceSlice(items)
	if format == output.FormatCSV {
		return f.PrintCSV(columns, data)
	}
	return f.PrintTable(columns, data)
}
