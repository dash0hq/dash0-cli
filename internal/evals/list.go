package evals

import (
	"context"
	"fmt"
	"os"
	"strconv"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var flags asset.ListFlags

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "[experimental] List evals",
		Long:    `List all generative AI evaluations (evals) in the specified dataset. Deleted evals are not listed.` + internal.CONFIG_HINT,
		Example: `  # List evals (default: up to 50)
  dash0 --experimental evals list

  # Include the dataset and origin columns
  dash0 --experimental evals list -o wide

  # Output as YAML for backup or version control
  dash0 --experimental evals list -o yaml > evals.yaml

  # Output as JSON for scripting
  dash0 --experimental evals list -o json

  # Output as CSV (pipe-friendly)
  dash0 --experimental evals list -o csv`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
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
	items, err := apiClient.ListGenerativeAiEvaluations(ctx, dataset)
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

	data := make([]any, len(items))
	for i, e := range items {
		data[i] = e
	}

	switch format {
	case output.FormatJSON:
		return formatter.PrintJSON(data)
	case output.FormatYAML:
		return formatter.PrintMultiDocYAML(data)
	default:
		return printTable(formatter, data, format)
	}
}

// printTable renders the table, wide, and CSV formats.
//
// There is no URL column, because the API client ships no deeplink helper for
// this kind, the same as for time series aggregations.
func printTable(f *output.Formatter, data []any, format output.Format) error {
	columns := []output.Column{
		{Header: internal.HEADER_NAME, Width: 40, Value: func(item any) string {
			return dash0api.GetGenerativeAiEvaluationName(item.(*dash0api.GenerativeAiEvaluationDefinition))
		}},
		{Header: internal.HEADER_ID, Width: 36, Value: func(item any) string {
			return dash0api.GetGenerativeAiEvaluationID(item.(*dash0api.GenerativeAiEvaluationDefinition))
		}},
		{Header: internal.HEADER_ENABLED, Width: 8, Value: func(item any) string {
			return strconv.FormatBool(item.(*dash0api.GenerativeAiEvaluationDefinition).Spec.Enabled)
		}},
	}

	if format == output.FormatWide || format == output.FormatCSV {
		columns = append(columns,
			output.Column{Header: internal.HEADER_DATASET, Width: 15, Value: func(item any) string {
				return dash0api.GetGenerativeAiEvaluationDataset(item.(*dash0api.GenerativeAiEvaluationDefinition))
			}},
			output.Column{Header: internal.HEADER_ORIGIN, Width: 30, Value: func(item any) string {
				return dash0api.GetGenerativeAiEvaluationOrigin(item.(*dash0api.GenerativeAiEvaluationDefinition))
			}},
		)
	}

	if len(data) == 0 {
		fmt.Println("No evals found.")
		return nil
	}

	if format == output.FormatCSV {
		return f.PrintCSV(columns, data)
	}
	return f.PrintTable(columns, data)
}
