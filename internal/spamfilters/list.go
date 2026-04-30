package spamfilters

import (
	"context"
	"fmt"
	"os"

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
		Short:   "[experimental] List spam filters",
		Long:    `List all spam filters in the specified dataset.` + internal.CONFIG_HINT,
		Example: `  # List spam filters (default: up to 50)
  dash0 --experimental spam-filters list

  # Output as YAML for backup or version control
  dash0 --experimental spam-filters list -o yaml > spam-filters.yaml

  # Output as JSON for scripting
  dash0 --experimental spam-filters list -o json

  # Output as CSV (pipe-friendly)
  dash0 --experimental spam-filters list -o csv

  # List without the header row (pipe-friendly)
  dash0 --experimental spam-filters list --skip-header`,
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

	apiUrl := client.ResolveApiUrl(ctx, flags.ApiUrl)
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)
	iter := apiClient.ListSpamFiltersIter(ctx, dataset)

	var filters []*dash0api.SpamFilter
	count := 0
	for iter.Next() {
		filters = append(filters, iter.Current())
		count++
		if !flags.All && flags.Limit > 0 && count >= flags.Limit {
			break
		}
	}

	if err := iter.Err(); err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "spam filter",
		})
	}

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout, output.WithSkipHeader(flags.SkipHeader))

	switch format {
	case output.FormatJSON:
		return formatter.PrintJSON(toInterfaceSlice(filters))
	case output.FormatYAML:
		return formatter.PrintMultiDocYAML(toInterfaceSlice(filters))
	default:
		return printSpamFilterTable(formatter, filters, format, apiUrl)
	}
}

func toInterfaceSlice(filters []*dash0api.SpamFilter) []interface{} {
	result := make([]interface{}, len(filters))
	for i, f := range filters {
		result[i] = f
	}
	return result
}

func printSpamFilterTable(f *output.Formatter, filters []*dash0api.SpamFilter, format output.Format, apiUrl string) error {
	columns := []output.Column{
		{Header: internal.HEADER_NAME, Width: 40, Value: func(item interface{}) string {
			r := item.(*dash0api.SpamFilter)
			return dash0api.GetSpamFilterName(r)
		}},
		{Header: internal.HEADER_ID, Width: 36, Value: func(item interface{}) string {
			r := item.(*dash0api.SpamFilter)
			return dash0api.GetSpamFilterID(r)
		}},
	}

	if format == output.FormatWide || format == output.FormatCSV {
		columns = append(columns,
			output.Column{Header: internal.HEADER_DATASET, Width: 15, Value: func(item interface{}) string {
				r := item.(*dash0api.SpamFilter)
				return dash0api.GetSpamFilterDataset(r)
			}},
			output.Column{Header: internal.HEADER_ORIGIN, Width: 20, Value: func(item interface{}) string {
				r := item.(*dash0api.SpamFilter)
				if r.Metadata.Labels != nil && r.Metadata.Labels.Dash0Comorigin != nil {
					return *r.Metadata.Labels.Dash0Comorigin
				}
				return ""
			}},
		)
	}

	if len(filters) == 0 {
		fmt.Println("No spam filters found.")
		return nil
	}

	data := make([]interface{}, len(filters))
	for i, r := range filters {
		data[i] = r
	}

	if format == output.FormatCSV {
		return f.PrintCSV(columns, data)
	}
	return f.PrintTable(columns, data)
}
