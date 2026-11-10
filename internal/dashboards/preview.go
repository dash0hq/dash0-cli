package dashboards

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/spf13/cobra"
)

const defaultPreviewTTL = "1d"

type previewFlags struct {
	asset.CommonFlags
	File string
	TTL  string
}

type previewResult struct {
	Name   string `json:"name"`
	ID     string `json:"id"`
	Action string `json:"action"`
	URL    string `json:"url"`
}

func newPreviewCmd() *cobra.Command {
	var flags previewFlags

	cmd := &cobra.Command{
		Use:   "preview -f <file|directory>",
		Short: "[experimental] Preview dashboards as ephemeral dashboards",
		Long: `Create or update an EphemeralDashboard for each Dashboard and PersesDashboard in a file or directory, and print the URL of each preview. Documents of other kinds are skipped. Use '-f -' to read from stdin.

Dash0 deletes an ephemeral dashboard once --ttl has passed since its latest write. Running the command again updates the same previews and starts their --ttl again. A dashboard without an id gets a new preview on every run.

A preview is upserted under the dashboard's own id, so previewing a dashboard that already exists as a permanent dashboard fails: the API does not change the kind of an existing dashboard.` + internal.CONFIG_HINT,
		Example: `  # Preview every dashboard in a directory for one day
  dash0 --experimental dashboards preview -f dashboards/

  # Preview one dashboard for 12 hours
  dash0 --experimental dashboards preview -f dashboard.yaml --ttl 12h

  # Print the previews as JSON for a CI job
  dash0 --experimental dashboards preview -f dashboards/ -o json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runPreview(cmd.Context(), &flags)
		},
	}

	asset.RegisterCommonFlags(cmd, &flags.CommonFlags)
	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Path to a YAML file or a directory of YAML files (use '-' for stdin)")
	cmd.Flags().StringVar(&flags.TTL, "ttl", defaultPreviewTTL, "How long each preview lives after its latest write, as a duration such as 12h or 7d")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func runPreview(ctx context.Context, flags *previewFlags) error {
	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}
	if flags.TTL == "" {
		return fmt.Errorf("--ttl must not be empty")
	}

	dashboards, err := readPreviewDashboards(flags.File)
	if err != nil {
		return err
	}

	apiUrl := client.ResolveApiUrl(ctx, flags.ApiUrl)
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}
	dataset := client.ResolveDataset(ctx, flags.Dataset)

	results := make([]previewResult, 0, len(dashboards))
	for _, dashboard := range dashboards {
		asset.ToEphemeralDashboard(dashboard, flags.TTL)
		result, err := asset.ImportDashboard(ctx, apiClient, dashboard, dataset)
		if err != nil {
			return client.HandleAPIError(err, client.ErrorContext{
				AssetType: "ephemeral dashboard",
				AssetName: dash0api.GetDashboardName(dashboard),
			})
		}
		results = append(results, previewResult{
			Name:   result.Name,
			ID:     result.ID,
			Action: string(result.Action),
			URL:    dash0api.DeeplinkURL(apiUrl, dash0api.DeeplinkAssetTypeDashboard, result.ID, dataset),
		})
	}

	return printPreviewResults(output.NewFormatter(format, os.Stdout), results)
}

// readPreviewDashboards parses every Dashboard and PersesDashboard in file
// before any write, so a malformed document stops the run before the first
// preview is created.
func readPreviewDashboards(file string) ([]*dash0api.DashboardDefinition, error) {
	documents, _, _, err := asset.ReadInput(file, false)
	if err != nil {
		return nil, err
	}
	var dashboards []*dash0api.DashboardDefinition
	for _, doc := range documents {
		switch asset.NormalizeKind(doc.Kind) {
		case "dashboard", "persesdashboard":
			dashboard, err := dash0yaml.ParseAsDashboard(doc.Raw)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", doc.Location(), err)
			}
			dashboards = append(dashboards, dashboard)
		}
	}
	if len(dashboards) == 0 {
		return nil, fmt.Errorf("no Dashboard or PersesDashboard found in %s", file)
	}
	return dashboards, nil
}

func printPreviewResults(f *output.Formatter, results []previewResult) error {
	switch f.Format() {
	case output.FormatJSON, output.FormatYAML:
		return f.Print(results)
	}
	columns := []output.Column{
		{Header: internal.HEADER_NAME, Width: 40, Value: func(item interface{}) string { return item.(previewResult).Name }},
		{Header: internal.HEADER_ID, Width: 36, Value: func(item interface{}) string { return item.(previewResult).ID }},
		{Header: "ACTION", Width: 8, Value: func(item interface{}) string { return item.(previewResult).Action }},
		{Header: internal.HEADER_URL, Width: 70, Value: func(item interface{}) string { return item.(previewResult).URL }},
	}
	data := make([]interface{}, len(results))
	for i, r := range results {
		data[i] = r
	}
	if f.Format() == output.FormatCSV {
		return f.PrintCSV(columns, data)
	}
	return f.PrintTable(columns, data)
}
