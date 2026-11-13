package samplingrules

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
		Short:   "List sampling rules",
		Long: `List the sampling rules in the specified dataset.

The CONDITIONS column summarizes each rule's conditions, for example
"and(error, probabilistic 0.25)". OTTL expressions are shown as "ottl"; use
'-o yaml' for the full definitions.` + internal.CONFIG_HINT,
		Example: `  # List sampling rules (default: up to 50)
  dash0 sampling-rules list

  # Show the rate limit, dataset, and origin too
  dash0 sampling-rules list -o wide

  # Output as YAML for backup or version control
  dash0 sampling-rules list -o yaml > sampling-rules.yaml

  # Output as CSV (pipe-friendly)
  dash0 sampling-rules list -o csv`,
		Args: cobra.NoArgs,
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

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	items, err := apiClient.ListSamplingRules(ctx, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType})
	}

	if !flags.All && flags.Limit > 0 && len(items) > flags.Limit {
		items = items[:flags.Limit]
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

func toInterfaceSlice(items []*dash0api.SamplingDefinition) []interface{} {
	result := make([]interface{}, len(items))
	for i, r := range items {
		result[i] = r
	}
	return result
}

// printTable renders the table, wide, and CSV formats.
//
// There is no URL column, because the API client ships no deeplink helper for
// this kind, as for signal-to-metrics rules and time series aggregations.
func printTable(f *output.Formatter, items []*dash0api.SamplingDefinition, format output.Format) error {
	columns := []output.Column{
		{Header: internal.HEADER_NAME, Width: 40, Value: func(item interface{}) string {
			return dash0api.GetSamplingRuleName(item.(*dash0api.SamplingDefinition))
		}},
		{Header: internal.HEADER_ID, Width: 36, Value: func(item interface{}) string {
			return dash0api.GetSamplingRuleID(item.(*dash0api.SamplingDefinition))
		}},
		{Header: internal.HEADER_ENABLED, Width: 8, Value: func(item interface{}) string {
			return strconv.FormatBool(item.(*dash0api.SamplingDefinition).Spec.Enabled)
		}},
		{Header: internal.HEADER_CONDITIONS, Width: 40, Value: func(item interface{}) string {
			return asset.DescribeSamplingRuleConditions(item.(*dash0api.SamplingDefinition))
		}},
	}

	if format == output.FormatWide || format == output.FormatCSV {
		columns = append(columns,
			output.Column{Header: internal.HEADER_RATE_LIMIT, Width: 10, Value: func(item interface{}) string {
				return rateLimit(item.(*dash0api.SamplingDefinition))
			}},
			output.Column{Header: internal.HEADER_DATASET, Width: 15, Value: func(item interface{}) string {
				return dash0api.GetSamplingRuleDataset(item.(*dash0api.SamplingDefinition))
			}},
			output.Column{Header: internal.HEADER_ORIGIN, Width: 40, Value: func(item interface{}) string {
				return origin(item.(*dash0api.SamplingDefinition))
			}},
		)
	}

	if len(items) == 0 {
		fmt.Println("No sampling rules found.")
		return nil
	}

	data := toInterfaceSlice(items)
	if format == output.FormatCSV {
		return f.PrintCSV(columns, data)
	}
	return f.PrintTable(columns, data)
}
