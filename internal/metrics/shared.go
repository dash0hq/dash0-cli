package metrics

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/dash0hq/dash0-cli/internal/agentmode"
	dashcolor "github.com/dash0hq/dash0-cli/internal/color"
	"github.com/dash0hq/dash0-cli/internal/query"
	versionpkg "github.com/dash0hq/dash0-cli/internal/version"
)

// QueryInstantResponse represents the response from the Prometheus instant query API.
type QueryInstantResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []any     `json:"value"`
		} `json:"result"`
	} `json:"data"`
	Error     string `json:"error,omitempty"`
	ErrorType string `json:"errorType,omitempty"`
}

// QueryRangeResponse represents the response from the Prometheus range query API.
type QueryRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][]any   `json:"values"`
		} `json:"result"`
	} `json:"data"`
	Error     string `json:"error,omitempty"`
	ErrorType string `json:"errorType,omitempty"`
}

// queryFormat represents the output format for metrics commands.
type queryFormat string

const (
	queryFormatTable queryFormat = "table"
	queryFormatJSON  queryFormat = "json"
	queryFormatCSV   queryFormat = "csv"
)

// parseQueryFormat parses the output format string, defaulting to JSON in agent mode.
func parseQueryFormat(s string) (queryFormat, error) {
	switch strings.ToLower(s) {
	case "":
		if agentmode.Enabled {
			return queryFormatJSON, nil
		}
		return queryFormatTable, nil
	case "table":
		return queryFormatTable, nil
	case "json":
		return queryFormatJSON, nil
	case "csv":
		return queryFormatCSV, nil
	default:
		return "", fmt.Errorf("unsupported output format %q; supported formats: table, json, csv", s)
	}
}

var timestampColumn = query.ColumnDef{Key: query.AliasTimestamp, Aliases: []string{query.AliasTime, query.AliasTimestamp}, Header: "TIMESTAMP", Width: 28}
var valueColumn = query.ColumnDef{Key: query.AliasValue, Aliases: []string{query.AliasValue}, Header: "VALUE", Width: 0}

var metricDefaultColumns = []query.ColumnDef{timestampColumn, valueColumn}

var metricDefaultCSVColumns = []query.ColumnDef{
	timestampColumn,
	{Key: "__name__", Header: "__NAME__"},
	valueColumn,
}

var metricKnownColumns = []query.ColumnDef{
	{Key: "__name__", Header: "__NAME__", Width: 80},
	{Key: "job", Aliases: []string{"job"}, Header: "JOB", Width: 60},
	{Key: "instance", Aliases: []string{"instance"}, Header: "INSTANCE", Width: 60},
}

// unsupportedMetricColumns maps column names that users might try but that are not
// available in Prometheus query results, along with a helpful suggestion.
var unsupportedMetricColumns = map[string]string{
	"otel_metric_name": "the Prometheus API does not include OTel metric names; use __name__ for the Prometheus-normalized metric name",
}

// resolveMetricColumns resolves --column flags into column definitions.
// The timestamp column is always first and the value column is always last.
// When no custom columns are specified, the defaults depend on the output format:
// CSV includes __name__ by default; table does not (it uses the verbose label-per-line format).
func resolveMetricColumns(columns []string, format queryFormat) ([]query.ColumnDef, error) {
	for _, col := range columns {
		if hint, ok := unsupportedMetricColumns[strings.TrimSpace(col)]; ok {
			return nil, fmt.Errorf("column %q is not available: %s", col, hint)
		}
	}
	if len(columns) == 0 {
		if format == queryFormatCSV {
			return metricDefaultCSVColumns, nil
		}
		return metricDefaultColumns, nil
	}

	// Reject timestamp/value — they are always included automatically.
	for _, col := range columns {
		c := strings.TrimSpace(strings.ToLower(col))
		if c == query.AliasTimestamp || c == query.AliasTime || c == query.AliasValue {
			return nil, fmt.Errorf("column %q is always included and cannot be specified with --column", col)
		}
	}

	specs, err := query.ParseColumns(columns)
	if err != nil {
		return nil, err
	}
	middle := query.ResolveColumns(specs, metricKnownColumns)

	// Always: timestamp first, user columns in the middle, value last.
	result := make([]query.ColumnDef, 0, len(middle)+2)
	result = append(result, timestampColumn)
	result = append(result, middle...)
	result = append(result, valueColumn)
	return result, nil
}

// runInstantQuery executes an instant PromQL query against the Prometheus API.
func runInstantQuery(apiURL, authToken, promql, evalTime string, dataset *string) (*QueryInstantResponse, error) {
	parsedURL, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("invalid API URL: %w", err)
	}
	parsedURL.Path = "/api/prometheus/api/v1/query"

	if evalTime == "" {
		evalTime = "now"
	}

	params := url.Values{}
	params.Set("query", promql)
	params.Set("time", evalTime)
	if dataset != nil && *dataset != "" && *dataset != "default" {
		params.Set("dataset", *dataset)
	}
	parsedURL.RawQuery = params.Encode()

	body, err := executePrometheusRequest(parsedURL.String(), authToken)
	if err != nil {
		return nil, err
	}

	var response QueryInstantResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Status != "success" {
		return nil, fmt.Errorf("query failed: %s", response.Error)
	}

	return &response, nil
}

// executePrometheusRequest sends an authenticated GET request and returns the response body.
func executePrometheusRequest(requestURL, authToken string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))
	req.Header.Set("User-Agent", versionpkg.UserAgent())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
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

// flattenInstantResult converts an instant query result into a row map suitable for table/CSV rendering.
func flattenInstantResult(metric map[string]string, value []any) map[string]string {
	row := make(map[string]string, len(metric)+2)

	// Copy all labels.
	maps.Copy(row, metric)

	// Extract timestamp and value from the [timestamp, value] pair.
	if len(value) >= 2 {
		if ts, ok := value[0].(float64); ok {
			row[query.AliasTimestamp] = query.FormatTimestamp(fmt.Sprintf("%d", int64(ts*1e9)))
		}
		row[query.AliasValue] = fmt.Sprintf("%v", value[1])
	}

	return row
}

// renderInstantTable renders an instant query response as a table.
// When custom columns are specified, it renders a columnar table.
// Otherwise, it uses the verbose label-per-line format for backwards compatibility.
func renderInstantTable(response *QueryInstantResponse, cols []query.ColumnDef, skipHeader bool, customColumns bool) {
	if len(response.Data.Result) == 0 {
		fmt.Println("No results found.")
		return
	}

	if customColumns {
		rows := make([]map[string]string, 0, len(response.Data.Result))
		for _, result := range response.Data.Result {
			rows = append(rows, flattenInstantResult(result.Metric, result.Value))
		}
		query.RenderTable(os.Stdout, cols, rows, skipHeader)
		return
	}

	// Verbose label-per-line format (backwards compatible with the original metrics instant output).
	for _, result := range response.Data.Result {
		fmt.Println("Metric:")
		for k, v := range result.Metric {
			fmt.Printf("  %s: %s\n", k, v)
		}
		if len(result.Value) >= 2 {
			fmt.Printf("Value: %v\n", result.Value[1])
		} else {
			fmt.Printf("Value: %v\n", result.Value)
		}
		fmt.Println()
	}
}

// renderInstantCSV renders an instant query response as CSV.
func renderInstantCSV(response *QueryInstantResponse, cols []query.ColumnDef, skipHeader bool) {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	if !skipHeader {
		_ = query.WriteCSVHeader(w, cols)
	}

	for _, result := range response.Data.Result {
		row := flattenInstantResult(result.Metric, result.Value)
		_ = query.WriteCSVRow(w, cols, row)
	}
}

// renderInstantJSON renders an instant query response as JSON.
func renderInstantJSON(response *QueryInstantResponse) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(response)
}

// printAvailableColumnsHint collects all label keys from an instant query result
// and prints a hint to stderr showing which columns are available via --column.
func printAvailableColumnsHint(response *QueryInstantResponse) {
	keys := collectInstantLabelKeys(response)
	if len(keys) == 0 {
		return
	}
	o := dashcolor.StderrOutput()
	hintPrefix := o.String("Hint:").Foreground(o.Color("6")).String()
	fmt.Fprintf(os.Stderr, "%s use --column to select columns; available labels: %s\n\n", hintPrefix, strings.Join(keys, ", "))
}

// collectInstantLabelKeys returns the deduplicated, sorted label keys across all results.
func collectInstantLabelKeys(response *QueryInstantResponse) []string {
	seen := make(map[string]struct{})
	for _, result := range response.Data.Result {
		for k := range result.Metric {
			seen[k] = struct{}{}
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
