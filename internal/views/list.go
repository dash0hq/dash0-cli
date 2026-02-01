package views

import (
	"context"
	"fmt"
	"os"

	dash0 "github.com/dash0hq/dash0-api-client-go"
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
		Short:   "List views",
		Long:    `List all views in the specified dataset`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context(), &flags)
		},
	}

	asset.RegisterListFlags(cmd, &flags)
	return cmd
}

func runList(ctx context.Context, flags *asset.ListFlags) error {
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	iter := apiClient.ListViewsIter(ctx, client.DatasetPtr(flags.Dataset))

	var views []*dash0.ViewApiListItem
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

	formatter := output.NewFormatter(format, os.Stdout)

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return formatter.Print(views)
	default:
		return printViewTable(formatter, views, format)
	}
}

func printViewTable(f *output.Formatter, views []*dash0.ViewApiListItem, format output.Format) error {
	columns := []output.Column{
		{Header: internal.HEADER_NAME, Width: 40, Value: func(item interface{}) string {
			v := item.(*dash0.ViewApiListItem)
			if v.Name != nil {
				return *v.Name
			}
			return ""
		}},
		{Header: internal.HEADER_ID, Width: 36, Value: func(item interface{}) string {
			v := item.(*dash0.ViewApiListItem)
			return v.Id
		}},
	}

	if format == output.FormatWide {
		columns = append(columns,
			output.Column{Header: internal.HEADER_DATASET, Width: 15, Value: func(item interface{}) string {
				v := item.(*dash0.ViewApiListItem)
				return v.Dataset
			}},
			output.Column{Header: internal.HEADER_ORIGIN, Width: 30, Value: func(item interface{}) string {
				v := item.(*dash0.ViewApiListItem)
				if v.Origin != nil {
					return *v.Origin
				}
				return ""
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

	return f.PrintTable(columns, data)
}
