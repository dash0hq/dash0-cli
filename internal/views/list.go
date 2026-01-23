package views

import (
	"context"
	"fmt"
	"os"

	dash0 "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/dash0hq/dash0-cli/internal/resource"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var flags resource.ListFlags

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List views",
		Long:    `List all views in the specified dataset`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context(), &flags)
		},
	}

	resource.RegisterListFlags(cmd, &flags)
	return cmd
}

func runList(ctx context.Context, flags *resource.ListFlags) error {
	apiClient, err := client.NewClient(flags.ApiUrl, flags.AuthToken)
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
		return client.HandleAPIError(err)
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
		return printViewTable(formatter, views)
	}
}

func printViewTable(f *output.Formatter, views []*dash0.ViewApiListItem) error {
	columns := []output.Column{
		{Header: "NAME", Width: 40, Value: func(item interface{}) string {
			v := item.(*dash0.ViewApiListItem)
			if v.Name != nil {
				return *v.Name
			}
			return ""
		}},
		{Header: "ID", Width: 36, Value: func(item interface{}) string {
			v := item.(*dash0.ViewApiListItem)
			return v.Id
		}},
	}

	data := make([]interface{}, len(views))
	for i, v := range views {
		data[i] = v
	}

	if len(data) == 0 {
		fmt.Println("No views found.")
		return nil
	}

	return f.PrintTable(columns, data)
}
