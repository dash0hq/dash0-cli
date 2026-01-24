package syntheticchecks

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
		Short:   "List synthetic checks",
		Long:    `List all synthetic checks in the specified dataset`,
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

	iter := apiClient.ListSyntheticChecksIter(ctx, client.DatasetPtr(flags.Dataset))

	var checks []*dash0.SyntheticChecksApiListItem
	count := 0
	for iter.Next() {
		checks = append(checks, iter.Current())
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
		return formatter.Print(checks)
	default:
		return printSyntheticCheckTable(formatter, checks, format)
	}
}

func printSyntheticCheckTable(f *output.Formatter, checks []*dash0.SyntheticChecksApiListItem, format output.Format) error {
	columns := []output.Column{
		{Header: "NAME", Width: 40, Value: func(item interface{}) string {
			c := item.(*dash0.SyntheticChecksApiListItem)
			if c.Name != nil {
				return *c.Name
			}
			return ""
		}},
		{Header: "ID", Width: 36, Value: func(item interface{}) string {
			c := item.(*dash0.SyntheticChecksApiListItem)
			return c.Id
		}},
	}

	if format == output.FormatWide {
		columns = append(columns,
			output.Column{Header: "DATASET", Width: 15, Value: func(item interface{}) string {
				c := item.(*dash0.SyntheticChecksApiListItem)
				return c.Dataset
			}},
			output.Column{Header: "ORIGIN", Width: 30, Value: func(item interface{}) string {
				c := item.(*dash0.SyntheticChecksApiListItem)
				if c.Origin != nil {
					return *c.Origin
				}
				return ""
			}},
		)
	}

	data := make([]interface{}, len(checks))
	for i, c := range checks {
		data[i] = c
	}

	if len(data) == 0 {
		fmt.Println("No synthetic checks found.")
		return nil
	}

	return f.PrintTable(columns, data)
}
