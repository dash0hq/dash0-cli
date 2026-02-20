package syntheticchecks

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
		Short: "List synthetic checks",
		Long: `List all synthetic checks in the specified dataset.` + internal.CONFIG_HINT,
		Example: `  # List synthetic checks (default: up to 50)
  dash0 synthetic-checks list

  # Output as YAML for backup or version control
  dash0 synthetic-checks list -o yaml > synthetic-checks.yaml

  # Output as JSON for scripting
  dash0 synthetic-checks list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context(), &flags)
		},
	}

	asset.RegisterListFlags(cmd, &flags)
	return cmd
}

func runList(ctx context.Context, flags *asset.ListFlags) error {
	apiUrl := client.ResolveApiUrl(ctx, flags.ApiUrl)
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	iter := apiClient.ListSyntheticChecksIter(ctx, client.ResolveDataset(ctx, flags.Dataset))

	var checks []*dash0api.SyntheticChecksApiListItem
	count := 0
	for iter.Next() {
		checks = append(checks, iter.Current())
		count++
		if !flags.All && flags.Limit > 0 && count >= flags.Limit {
			break
		}
	}

	if err := iter.Err(); err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "synthetic check",
		})
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
		return printSyntheticCheckTable(formatter, checks, format, apiUrl)
	}
}

func printSyntheticCheckTable(f *output.Formatter, checks []*dash0api.SyntheticChecksApiListItem, format output.Format, apiUrl string) error {
	columns := []output.Column{
		{Header: internal.HEADER_NAME, Width: 40, Value: func(item interface{}) string {
			c := item.(*dash0api.SyntheticChecksApiListItem)
			if c.Name != nil {
				return *c.Name
			}
			return ""
		}},
		{Header: internal.HEADER_ID, Width: 36, Value: func(item interface{}) string {
			c := item.(*dash0api.SyntheticChecksApiListItem)
			return c.Id
		}},
	}

	if format == output.FormatWide {
		columns = append(columns,
			output.Column{Header: internal.HEADER_DATASET, Width: 15, Value: func(item interface{}) string {
				c := item.(*dash0api.SyntheticChecksApiListItem)
				return c.Dataset
			}},
			output.Column{Header: internal.HEADER_ORIGIN, Width: 20, Value: func(item interface{}) string {
				c := item.(*dash0api.SyntheticChecksApiListItem)
				if c.Origin != nil {
					return *c.Origin
				}
				return ""
			}},
			output.Column{Header: internal.HEADER_URL, Width: 70, Value: func(item interface{}) string {
				c := item.(*dash0api.SyntheticChecksApiListItem)
				return asset.DeeplinkURL(apiUrl, "synthetic check", c.Id)
			}},
		)
	}

	if len(checks) == 0 {
		fmt.Println("No synthetic checks found.")
		return nil
	}

	data := make([]interface{}, len(checks))
	for i, c := range checks {
		data[i] = c
	}

	return f.PrintTable(columns, data)
}
