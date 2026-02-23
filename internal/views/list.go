package views

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var flags asset.ListFlags

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short: "List views",
		Long: `List all views in the specified dataset.` + internal.CONFIG_HINT,
		Example: `  # List views (default: up to 50)
  dash0 views list

  # Output as YAML for backup or version control
  dash0 views list -o yaml > views.yaml

  # Output as JSON for scripting
  dash0 views list -o json

  # Output as CSV (pipe-friendly)
  dash0 views list -o csv

  # List without the header row (pipe-friendly)
  dash0 views list --skip-header`,
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

	iter := apiClient.ListViewsIter(ctx, client.ResolveDataset(ctx, flags.Dataset))

	var views []*dash0api.ViewApiListItem
	count := 0
	for iter.Next() {
		views = append(views, iter.Current())
		count++
		if !flags.All && flags.Limit > 0 && count >= flags.Limit {
			break
		}
	}

	if err := iter.Err(); err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "view",
		})
	}

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout, output.WithSkipHeader(flags.SkipHeader))

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return formatter.Print(views)
	default:
		return printViewTable(formatter, views, format, apiUrl)
	}
}

func printViewTable(f *output.Formatter, views []*dash0api.ViewApiListItem, format output.Format, apiUrl string) error {
	columns := []output.Column{
		{Header: internal.HEADER_NAME, Width: 40, Value: func(item interface{}) string {
			v := item.(*dash0api.ViewApiListItem)
			if v.Name != nil {
				return *v.Name
			}
			return ""
		}},
		{Header: internal.HEADER_ID, Width: 36, Value: func(item interface{}) string {
			v := item.(*dash0api.ViewApiListItem)
			return v.Id
		}},
	}

	if format == output.FormatWide || format == output.FormatCSV {
		columns = append(columns,
			output.Column{Header: internal.HEADER_DATASET, Width: 15, Value: func(item interface{}) string {
				v := item.(*dash0api.ViewApiListItem)
				return v.Dataset
			}},
			output.Column{Header: internal.HEADER_ORIGIN, Width: 20, Value: func(item interface{}) string {
				v := item.(*dash0api.ViewApiListItem)
				if v.Origin != nil {
					return *v.Origin
				}
				return ""
			}},
			output.Column{Header: internal.HEADER_URL, Width: 70, Value: func(item interface{}) string {
				v := item.(*dash0api.ViewApiListItem)
				return asset.DeeplinkURL(apiUrl, "view", v.Id)
			}},
		)
	}

	if len(views) == 0 {
		fmt.Println("No views found.")
		return nil
	}

	data := make([]interface{}, len(views))
	for i, v := range views {
		data[i] = v
	}

	if format == output.FormatCSV {
		return f.PrintCSV(columns, data)
	}
	return f.PrintTable(columns, data)
}
