package slos

import (
	"context"
	"fmt"
	"os"

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
		Short:   "List SLOs",
		Long:    `List all SLOs in the specified dataset.` + internal.CONFIG_HINT,
		Example: `  # List SLOs (default: up to 50)
  dash0 slos list

  # Output as YAML for backup or version control
  dash0 slos list -o yaml > slos.yaml

  # Output as JSON for scripting
  dash0 slos list -o json

  # Output as CSV (pipe-friendly)
  dash0 slos list -o csv

  # List without the header row (pipe-friendly)
  dash0 slos list --skip-header`,
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

	apiUrl := client.ResolveApiUrl(ctx, flags.ApiUrl)
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)
	iter := apiClient.ListSLOsIter(ctx, dataset)

	var items []*dash0api.SloDefinition
	count := 0
	for iter.Next() {
		items = append(items, iter.Current())
		count++
		if !flags.All && flags.Limit > 0 && count >= flags.Limit {
			break
		}
	}

	if err := iter.Err(); err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "SLO",
		})
	}

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout, output.WithSkipHeader(flags.SkipHeader))

	switch format {
	case output.FormatJSON, output.FormatYAML:
		definitions := make([]interface{}, 0, len(items))
		for _, slo := range items {
			definitions = append(definitions, slo)
		}
		if format == output.FormatYAML {
			return formatter.PrintMultiDocYAML(definitions)
		}
		return formatter.PrintJSON(definitions)
	default:
		return printSLOTable(formatter, items, format, apiUrl, dataset)
	}
}

func printSLOTable(f *output.Formatter, items []*dash0api.SloDefinition, format output.Format, apiUrl string, dataset *string) error {
	columns := []output.Column{
		{Header: internal.HEADER_NAME, Width: 40, Value: func(item interface{}) string {
			return dash0api.GetSLOName(item.(*dash0api.SloDefinition))
		}},
		{Header: internal.HEADER_ID, Width: 36, Value: func(item interface{}) string {
			return dash0api.GetSLOID(item.(*dash0api.SloDefinition))
		}},
	}

	if format == output.FormatWide || format == output.FormatCSV {
		columns = append(columns,
			output.Column{Header: internal.HEADER_DATASET, Width: 15, Value: func(item interface{}) string {
				return dash0api.GetSLODataset(item.(*dash0api.SloDefinition))
			}},
			output.Column{Header: internal.HEADER_URL, Width: 70, Value: func(item interface{}) string {
				return dash0api.DeeplinkURL(apiUrl, dash0api.DeeplinkAssetTypeSLO, dash0api.GetSLOID(item.(*dash0api.SloDefinition)), dataset)
			}},
		)
	}

	if len(items) == 0 {
		fmt.Println("No SLOs found.")
		return nil
	}

	data := make([]interface{}, len(items))
	for i, s := range items {
		data[i] = s
	}

	if format == output.FormatCSV {
		return f.PrintCSV(columns, data)
	}
	return f.PrintTable(columns, data)
}
