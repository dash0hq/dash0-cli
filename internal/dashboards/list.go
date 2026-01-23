package dashboards

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
		Short:   "List dashboards",
		Long:    `List all dashboards in the specified dataset`,
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

	// Fetch dashboards using iterator
	iter := apiClient.ListDashboardsIter(ctx, client.DatasetPtr(flags.Dataset))

	var dashboards []*dash0.DashboardApiListItem
	count := 0
	for iter.Next() {
		dashboards = append(dashboards, iter.Current())
		count++
		if !flags.All && flags.Limit > 0 && count >= flags.Limit {
			break
		}
	}

	if err := iter.Err(); err != nil {
		return client.HandleAPIError(err)
	}

	// Format output
	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout)

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return formatter.Print(dashboards)
	default:
		return printDashboardTable(formatter, dashboards)
	}
}

func printDashboardTable(f *output.Formatter, dashboards []*dash0.DashboardApiListItem) error {
	columns := []output.Column{
		{Header: "ID", Width: 36, Value: func(item interface{}) string {
			d := item.(*dash0.DashboardApiListItem)
			return d.Id
		}},
		{Header: "NAME", Width: 40, Value: func(item interface{}) string {
			d := item.(*dash0.DashboardApiListItem)
			if d.Name != nil {
				return *d.Name
			}
			return ""
		}},
	}

	// Convert to []interface{}
	data := make([]interface{}, len(dashboards))
	for i, d := range dashboards {
		data[i] = d
	}

	if len(data) == 0 {
		fmt.Println("No dashboards found.")
		return nil
	}

	return f.PrintTable(columns, data)
}
