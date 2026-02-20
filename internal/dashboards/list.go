package dashboards

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
		Short: "List dashboards",
		Long: `List all dashboards in the specified dataset.` + internal.CONFIG_HINT,
		Example: `  # List dashboards (default: up to 50)
  dash0 dashboards list

  # List all dashboards across all pages
  dash0 dashboards list --all

  # Output as YAML for backup or version control
  dash0 dashboards list -o yaml > dashboards.yaml

  # Output as JSON for scripting
  dash0 dashboards list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context(), &flags)
		},
	}

	asset.RegisterListFlags(cmd, &flags)
	return cmd
}

// dashboardListItem holds the display information for a dashboard
type dashboardListItem struct {
	Id          string
	DisplayName string
	Dataset     string
	Origin      *string
}

func runList(ctx context.Context, flags *asset.ListFlags) error {
	apiUrl := client.ResolveApiUrl(ctx, flags.ApiUrl)
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	// Fetch dashboards using iterator
	iter := apiClient.ListDashboardsIter(ctx, dataset)

	var listItems []*dash0api.DashboardApiListItem
	count := 0
	for iter.Next() {
		listItems = append(listItems, iter.Current())
		count++
		if !flags.All && flags.Limit > 0 && count >= flags.Limit {
			break
		}
	}

	if err := iter.Err(); err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "dashboard",
		})
	}

	// Format output
	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout)

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return formatter.Print(listItems)
	default:
		// Fetch full dashboard details to get display names
		dashboards := make([]dashboardListItem, 0, len(listItems))
		for _, item := range listItems {
			displayName := getDisplayName(ctx, apiClient, item.Id, dataset)
			dashboards = append(dashboards, dashboardListItem{
				Id:          item.Id,
				DisplayName: displayName,
				Dataset:     item.Dataset,
				Origin:      item.Origin,
			})
		}
		return printDashboardTable(formatter, dashboards, format, apiUrl)
	}
}

// getDisplayName fetches the full dashboard and extracts spec.display.name
func getDisplayName(ctx context.Context, apiClient dash0api.Client, id string, dataset *string) string {
	dashboard, err := apiClient.GetDashboard(ctx, id, dataset)
	if err != nil {
		return "" // Fall back to empty if we can't fetch
	}
	return asset.ExtractDashboardDisplayName(dashboard)
}

func printDashboardTable(f *output.Formatter, dashboards []dashboardListItem, format output.Format, apiUrl string) error {
	columns := []output.Column{
		{Header: internal.HEADER_NAME, Width: 40, Value: func(item interface{}) string {
			d := item.(dashboardListItem)
			return d.DisplayName
		}},
		{Header: internal.HEADER_ID, Width: 36, Value: func(item interface{}) string {
			d := item.(dashboardListItem)
			return d.Id
		}},
	}

	if format == output.FormatWide {
		columns = append(columns,
			output.Column{Header: internal.HEADER_DATASET, Width: 15, Value: func(item interface{}) string {
				d := item.(dashboardListItem)
				return d.Dataset
			}},
			output.Column{Header: internal.HEADER_ORIGIN, Width: 20, Value: func(item interface{}) string {
				d := item.(dashboardListItem)
				if d.Origin != nil {
					return *d.Origin
				}
				return ""
			}},
			output.Column{Header: internal.HEADER_URL, Width: 70, Value: func(item interface{}) string {
				d := item.(dashboardListItem)
				return asset.DeeplinkURL(apiUrl, "dashboard", d.Id)
			}},
		)
	}

	if len(dashboards) == 0 {
		fmt.Println("No dashboards found.")
		return nil
	}

	data := make([]interface{}, len(dashboards))
	for i, d := range dashboards {
		data[i] = d
	}

	return f.PrintTable(columns, data)
}
