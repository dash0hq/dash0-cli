package metrics

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/config"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/dash0hq/dash0-cli/internal/query"
	"github.com/dash0hq/dash0-cli/internal/version"
	"github.com/spf13/cobra"
)

// LabelValuesResponse represents the response from the Prometheus label values API.
type LabelValuesResponse struct {
	Status string   `json:"status"`
	Data   []string `json:"data"`
	Error  string   `json:"error,omitempty"`
}

// MetadataResponse represents the response from the Prometheus metadata API.
type MetadataResponse struct {
	Status string                       `json:"status"`
	Data   map[string][]MetadataEntry   `json:"data"`
	Error  string                       `json:"error,omitempty"`
}

// MetadataEntry represents a single metadata entry for a metric.
type MetadataEntry struct {
	Type string `json:"type"`
	Help string `json:"help"`
	Unit string `json:"unit"`
}

// MetricInfo is the output representation for a metric with its metadata.
type MetricInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Unit string `json:"unit"`
	Help string `json:"help"`
}

type listFlags struct {
	ApiUrl     string
	AuthToken  string
	Dataset    string
	Output     string
	From       string
	To         string
	Filter     string
	Limit      int
	SkipHeader bool
}

type listFormat string

const (
	listFormatTable listFormat = "table"
	listFormatWide  listFormat = "wide"
	listFormatJSON  listFormat = "json"
	listFormatCSV   listFormat = "csv"
)

var listDefaultColumns = []query.ColumnDef{
	{Key: "name", Header: internal.HEADER_NAME, Width: 0},
}

var listWideColumns = []query.ColumnDef{
	{Key: "name", Header: internal.HEADER_NAME, Width: 60},
	{Key: "type", Header: internal.HEADER_TYPE, Width: 12},
	{Key: "unit", Header: "UNIT", Width: 16},
	{Key: "description", Header: "DESCRIPTION", Width: 0},
}

func parseListFormat(s string) (listFormat, error) {
	switch strings.ToLower(s) {
	case "":
		if agentmode.Enabled {
			return listFormatJSON, nil
		}
		return listFormatTable, nil
	case "table":
		return listFormatTable, nil
	case "wide":
		return listFormatWide, nil
	case "json":
		return listFormatJSON, nil
	case "csv":
		return listFormatCSV, nil
	default:
		return "", fmt.Errorf("unknown output format: %s (valid formats: table, wide, json, csv)", s)
	}
}

func newListCmd() *cobra.Command {
	flags := &listFlags{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "[experimental] List available metric names",
		Long: `List available metric names from the Dash0 API.

Discovers metric names via the Prometheus-compatible label values API.
The default table output shows only metric names; use -o wide to also
see type, unit, and description (fetched from the metadata API).

The --filter flag matches metric names by substring or regex. If the
filter compiles as a regular expression, regex matching is used;
otherwise it falls back to case-insensitive substring matching.` + internal.CONFIG_HINT,
		Example: `  # List all metric names (last 1 hour)
  dash0 -X metrics list

  # Filter by substring
  dash0 -X metrics list --filter http_server

  # Filter by regex
  dash0 -X metrics list --filter "^http_server.*total$"

  # Show type, unit, and description
  dash0 -X metrics list --filter http_server -o wide

  # Custom time range
  dash0 -X metrics list --from now-6h
  dash0 -X metrics list --from now-30m --to now

  # Limit results
  dash0 -X metrics list --limit 20

  # Output as JSON (includes metadata)
  dash0 -X metrics list --filter http_server -o json

  # Output as CSV (same columns as wide)
  dash0 -X metrics list --filter http_server -o csv

  # CSV without header
  dash0 -X metrics list -o csv --skip-header`,
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runList(cmd, flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVar(&flags.Dataset, "dataset", "", "Dataset name")
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table, wide, json, csv (default: table)")
	cmd.Flags().StringVar(&flags.From, "from", "now-1h", "Start of time range (e.g. now-1h, now-6h, 2024-01-25T10:00:00Z)")
	cmd.Flags().StringVar(&flags.To, "to", "now", "End of time range (e.g. now, 2024-01-25T11:00:00Z)")
	cmd.Flags().StringVar(&flags.Filter, "filter", "", "Substring or regex filter on metric names")
	cmd.Flags().IntVarP(&flags.Limit, "limit", "l", 0, "Maximum number of results (0 = no limit)")
	cmd.Flags().BoolVar(&flags.SkipHeader, "skip-header", false, "Omit the header row from table, wide, and CSV output")

	return cmd
}

func runList(_ *cobra.Command, flags *listFlags) error {
	if err := output.ValidateSkipHeader(flags.SkipHeader, flags.Output); err != nil {
		return err
	}

	format, err := parseListFormat(flags.Output)
	if err != nil {
		return err
	}

	cfg, err := config.ResolveConfiguration(flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}
	apiUrl := cfg.ApiUrl
	authToken := cfg.AuthToken

	dataset := flags.Dataset
	if dataset == "" {
		dataset = cfg.Dataset
	}

	startEpoch, err := query.ResolveToEpochSeconds(flags.From)
	if err != nil {
		return fmt.Errorf("invalid --from value: %w", err)
	}
	endEpoch, err := query.ResolveToEpochSeconds(flags.To)
	if err != nil {
		return fmt.Errorf("invalid --to value: %w", err)
	}

	needsMetadata := format == listFormatWide || format == listFormatJSON || format == listFormatCSV

	if needsMetadata {
		return runListWithMetadata(apiUrl, authToken, dataset, startEpoch, endEpoch, flags, format)
	}
	return runListNamesOnly(apiUrl, authToken, dataset, startEpoch, endEpoch, flags)
}

func runListNamesOnly(apiUrl, authToken, dataset, startEpoch, endEpoch string, flags *listFlags) error {
	names, err := fetchLabelValues(apiUrl, authToken, dataset, startEpoch, endEpoch)
	if err != nil {
		return err
	}

	names = filterNames(names, flags.Filter)
	names = applyLimit(names, flags.Limit)

	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "No metrics found.")
		return nil
	}

	rows := make([]map[string]string, len(names))
	for i, name := range names {
		rows[i] = map[string]string{"name": name}
	}

	query.RenderTable(os.Stdout, listDefaultColumns, rows, flags.SkipHeader)
	return nil
}

func runListWithMetadata(apiUrl, authToken, dataset, startEpoch, endEpoch string, flags *listFlags, format listFormat) error {
	// Use label values API as the authoritative name list (respects time range),
	// then enrich with metadata. This keeps results consistent across all formats.
	names, err := fetchLabelValues(apiUrl, authToken, dataset, startEpoch, endEpoch)
	if err != nil {
		return err
	}

	metadata, err := fetchMetadata(apiUrl, authToken, dataset)
	if err != nil {
		return err
	}

	names = filterNames(names, flags.Filter)
	names = applyLimit(names, flags.Limit)

	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "No metrics found.")
		return nil
	}

	metrics := make([]MetricInfo, len(names))
	for i, name := range names {
		entry := metadata[name]
		metrics[i] = MetricInfo{
			Name: name,
			Type: entry.Type,
			Unit: entry.Unit,
			Help: entry.Help,
		}
	}

	switch format {
	case listFormatJSON:
		return renderJSON(metrics)
	case listFormatCSV:
		return renderCSV(metrics, flags.SkipHeader)
	default:
		return renderWideTable(metrics, flags.SkipHeader)
	}
}

func renderJSON(metrics []MetricInfo) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(metrics)
}

func renderWideTable(metrics []MetricInfo, skipHeader bool) error {
	rows := make([]map[string]string, len(metrics))
	for i, m := range metrics {
		rows[i] = map[string]string{
			"name":        m.Name,
			"type":        m.Type,
			"unit":        m.Unit,
			"description": m.Help,
		}
	}
	query.RenderTable(os.Stdout, listWideColumns, rows, skipHeader)
	return nil
}

func renderCSV(metrics []MetricInfo, skipHeader bool) error {
	w := csv.NewWriter(os.Stdout)
	if !skipHeader {
		if err := w.Write([]string{"name", "type", "unit", "description"}); err != nil {
			return err
		}
	}
	for _, m := range metrics {
		if err := w.Write([]string{m.Name, m.Type, m.Unit, m.Help}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func fetchLabelValues(apiUrl, authToken, dataset, startEpoch, endEpoch string) ([]string, error) {
	u, err := url.Parse(apiUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid API URL: %w", err)
	}
	u.Path = "/api/prometheus/api/v1/label/__name__/values"

	params := url.Values{}
	params.Set("start", startEpoch)
	params.Set("end", endEpoch)
	if dataset != "" && dataset != "default" {
		params.Set("dataset", dataset)
	}
	u.RawQuery = params.Encode()

	body, err := doGet(u.String(), authToken)
	if err != nil {
		return nil, err
	}

	var resp LabelValuesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse label values response: %w", err)
	}
	if resp.Status != "success" {
		return nil, fmt.Errorf("label values query failed: %s", resp.Error)
	}
	return resp.Data, nil
}

func fetchMetadata(apiUrl, authToken, dataset string) (map[string]MetadataEntry, error) {
	u, err := url.Parse(apiUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid API URL: %w", err)
	}
	u.Path = "/api/prometheus/api/v1/metadata"

	params := url.Values{}
	if dataset != "" && dataset != "default" {
		params.Set("dataset", dataset)
	}
	u.RawQuery = params.Encode()

	body, err := doGet(u.String(), authToken)
	if err != nil {
		return nil, err
	}

	var resp MetadataResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse metadata response: %w", err)
	}
	if resp.Status != "success" {
		return nil, fmt.Errorf("metadata query failed: %s", resp.Error)
	}

	// Flatten: take the first metadata entry for each metric name
	result := make(map[string]MetadataEntry, len(resp.Data))
	for name, entries := range resp.Data {
		if len(entries) > 0 {
			result[name] = entries[0]
		}
	}
	return result, nil
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

func doGet(requestURL, authToken string) ([]byte, error) {
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))
	req.Header.Set("User-Agent", version.UserAgent())

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func filterNames(names []string, filter string) []string {
	if filter == "" {
		return names
	}

	re, regexErr := regexp.Compile(filter)

	var result []string
	for _, name := range names {
		if regexErr != nil {
			// Regex didn't compile — fall back to case-insensitive substring match
			if strings.Contains(strings.ToLower(name), strings.ToLower(filter)) {
				result = append(result, name)
			}
		} else {
			if re.MatchString(name) {
				result = append(result, name)
			}
		}
	}
	return result
}

func applyLimit(names []string, limit int) []string {
	if limit > 0 && len(names) > limit {
		return names[:limit]
	}
	return names
}
