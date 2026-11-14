package dashboards

import (
	"context"
	"fmt"
	"os"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

const defaultPreviewTTL = "1d"

// previewIDNamespace must never change: every preview id derives from it, so a
// new value would orphan existing previews until their ttl passes.
var previewIDNamespace = uuid.MustParse("3b74d5ed-0dbc-4f89-b86e-d520992cca3a")

type previewFlags struct {
	asset.CommonFlags
	File   string
	TTL    string
	Prefix string
}

type previewResult struct {
	Name     string `json:"name"`
	ID       string `json:"id"`
	Action   string `json:"action"`
	URL      string `json:"url"`
	Current  string `json:"current,omitempty"`
	sourceID string
}

func newPreviewCmd() *cobra.Command {
	var flags previewFlags

	cmd := &cobra.Command{
		Use:   "preview -f <file|directory>",
		Short: "[experimental] Preview dashboards as ephemeral dashboards",
		Long: `Create or update an EphemeralDashboard for each Dashboard and PersesDashboard in a file or directory, and print the URL of each preview. Documents of other kinds are skipped. Use '-f -' to read from stdin.

Dash0 deletes an ephemeral dashboard once --ttl has passed since its latest write. Each preview gets an id derived from the dashboard's id, so the preview never replaces the dashboard itself, and 'apply' of the dashboard still works while the preview exists. Running the command again updates the same previews and starts their --ttl again. A dashboard without an id gets a new preview on every run.

--prefix puts a prefix in front of each preview's name and display name, and gives the previews their own ids, so previews with different prefixes, such as one per pull request, do not replace each other. When the dashboard itself exists in Dash0, the output also links to it.` + internal.CONFIG_HINT,
		Example: `  # Preview every dashboard in a directory for one day
  dash0 --experimental dashboards preview -f dashboards/

  # Preview one dashboard for 12 hours
  dash0 --experimental dashboards preview -f dashboard.yaml --ttl 12h

  # Preview the dashboards of a pull request, apart from other pull requests
  dash0 --experimental dashboards preview -f dashboards/ --prefix pr-376

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
	cmd.Flags().StringVar(&flags.Prefix, "prefix", "", "Prefix for the name and display name of each preview, which also gives the previews their own ids")
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
		sourceID := dashboardID(dashboard)
		asset.ToEphemeralDashboard(dashboard, flags.TTL)
		usePrefix(dashboard, flags.Prefix)
		usePreviewID(dashboard, flags.Prefix)
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
			URL:      dash0api.DeeplinkURL(apiUrl, dash0api.DeeplinkAssetTypeDashboard, result.ID, dataset),
			sourceID: sourceID,
		})
	}

	linkPreviews(ctx, apiClient, apiUrl, dataset, results)
	return printPreviewResults(output.NewFormatter(format, os.Stdout), results)
}

func dashboardID(dashboard *dash0api.DashboardDefinition) string {
	extensions := dashboard.Metadata.Dash0Extensions
	if extensions == nil || extensions.Id == nil {
		return ""
	}
	return *extensions.Id
}

// prefixSlug matches the name prefix of `percli dac preview --prefix`.
func prefixSlug(prefix string) string {
	return strings.ReplaceAll(strings.ToLower(prefix), " ", "-")
}

func usePrefix(dashboard *dash0api.DashboardDefinition, prefix string) {
	if prefix == "" {
		return
	}
	if dashboard.Metadata.Name != "" {
		dashboard.Metadata.Name = prefixSlug(prefix) + "-" + dashboard.Metadata.Name
	}
	if display, ok := dashboard.Spec["display"].(map[string]interface{}); ok {
		if name, ok := display["name"].(string); ok && name != "" {
			display["name"] = prefix + "-" + name
		}
	}
}

// usePreviewID replaces the dashboard's id with a UUIDv5 of it, because the API
// rejects an EphemeralDashboard under the id of a permanent dashboard and the
// reverse. The prefix is part of the UUIDv5 input only when set, so previews made
// without --prefix keep the ids they had before the flag existed.
func usePreviewID(dashboard *dash0api.DashboardDefinition, prefix string) {
	id := dashboardID(dashboard)
	if id == "" {
		return
	}
	if prefix != "" {
		id = prefixSlug(prefix) + "-" + id
	}
	previewID := uuid.NewSHA1(previewIDNamespace, []byte(id)).String()
	dashboard.Metadata.Dash0Extensions.Id = &previewID
}

// linkPreviews rewrites each URL with the server id from one list call, because
// the API keeps an id that is not a UUID as the origin and assigns its own UUID,
// and a link with the origin does not open. It also links each preview to its
// permanent dashboard. A failed list keeps the links from the write responses.
func linkPreviews(ctx context.Context, apiClient dash0api.Client, apiUrl string, dataset *string, results []previewResult) {
	items, err := apiClient.ListDashboards(ctx, dataset)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not list dashboards, so the preview links may not open and no link to the dashboard of each preview is shown:\n  %v\n", err)
		return
	}
	byIDOrOrigin := make(map[string]*dash0api.DashboardApiListItem, 2*len(items))
	for _, item := range items {
		byIDOrOrigin[item.Id] = item
		if item.Origin != nil && *item.Origin != "" {
			byIDOrOrigin[*item.Origin] = item
		}
	}
	for i := range results {
		if preview, ok := byIDOrOrigin[results[i].ID]; ok {
			results[i].URL = dash0api.DeeplinkURL(apiUrl, dash0api.DeeplinkAssetTypeDashboard, preview.Id, dataset)
		}
		if results[i].sourceID == "" {
			continue
		}
		if current, ok := byIDOrOrigin[results[i].sourceID]; ok && current.Kind == dash0api.Dashboard {
			results[i].Current = dash0api.DeeplinkURL(apiUrl, dash0api.DeeplinkAssetTypeDashboard, current.Id, dataset)
		}
	}
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
	if f.Format() == output.FormatWide || f.Format() == output.FormatCSV {
		columns = append(columns, output.Column{Header: "CURRENT", Width: 70, Value: func(item interface{}) string { return item.(previewResult).Current }})
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
