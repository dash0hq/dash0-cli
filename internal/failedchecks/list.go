package failedchecks

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/client"
	colorpkg "github.com/dash0hq/dash0-cli/internal/color"
	"github.com/dash0hq/dash0-cli/internal/query"
	"github.com/spf13/cobra"
)

const (
	apiPath = "/api/alerting/failed-checks"
)

type queryFlags struct {
	ApiUrl     string
	AuthToken  string
	Dataset    string
	Output     string
	Priority   []string
	Status     []string
	Active     bool
	Filter     []string
	From       string
	To         string
	Limit      int
	SkipHeader bool
}

type queryFormat string

const (
	queryFormatTable queryFormat = "table"
	queryFormatJSON  queryFormat = "json"
	queryFormatCSV   queryFormat = "csv"
)

// Request body types for /api/alerting/failed-checks.

type failedChecksRequest struct {
	TimeRange  requestTimeRange        `json:"timeRange"`
	Ordering   []orderingItem          `json:"ordering"`
	Filter     dash0api.FilterCriteria `json:"filter,omitempty"`
	Pagination paginationItem          `json:"pagination"`
	Dataset    string                  `json:"dataset,omitempty"`
}

type requestTimeRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type orderingItem struct {
	Key       string `json:"key"`
	Direction string `json:"direction"`
}

type paginationItem struct {
	Limit int `json:"limit"`
}

// Response types.

type failedChecksResponse struct {
	Cursors       *cursorsObj        `json:"cursors,omitempty"`
	ExecutionTime string             `json:"executionTime,omitempty"`
	Issues        []issue            `json:"issues"`
	TimeRange     *responseTimeRange `json:"timeRange,omitempty"`
}

type cursorsObj struct {
	Before *string `json:"before,omitempty"`
	After  *string `json:"after,omitempty"`
}

type responseTimeRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type issue struct {
	ID                        string         `json:"id"`
	CheckRule                 issueCheckRule `json:"checkRule"`
	InstanceStatus            string         `json:"instanceStatus"`
	Summary                   string         `json:"summary"`
	Description               string         `json:"description"`
	Start                     string         `json:"start"`
	End                       string         `json:"end"`
	Labels                    []issueLabel   `json:"labels"`
	EarliestEvaluatedTime     string         `json:"earliestEvaluatedTime,omitempty"`
	IssueIdentifier           string         `json:"issueIdentifier,omitempty"`
	AffectedResourceSummaries []any          `json:"affectedResourceSummaries,omitempty"`
	Annotations               []any          `json:"annotations,omitempty"`
}

type issueCheckRule struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Version             int      `json:"version"`
	Modes               []string `json:"modes,omitempty"`
	SummaryTemplate     string   `json:"summaryTemplate,omitempty"`
	DescriptionTemplate string   `json:"descriptionTemplate,omitempty"`
}

type issueLabel struct {
	Key   string     `json:"key"`
	Value labelValue `json:"value"`
}

type labelValue struct {
	StringValue *string `json:"stringValue,omitempty"`
	IntValue    *int64  `json:"intValue,omitempty"`
	BoolValue   *bool   `json:"boolValue,omitempty"`
}

func newQueryCmd() *cobra.Command {
	flags := &queryFlags{}

	cmd := &cobra.Command{
		Use:     "query",
		Aliases: []string{"list", "ls"},
		Short:   "Query failed check instances",
		Long:    `Query failed check instances from Dash0 alerting within a time range.` + internal.CONFIG_HINT,
		Example: `  # List all active P1 and P2 issues
  dash0 failed-checks query --priority p1,p2 --active

  # List all active issues regardless of priority
  dash0 failed-checks query --active

  # Filter by status
  dash0 failed-checks query --status critical,degraded

  # List P1 issues from the last hour (active and resolved)
  dash0 failed-checks query --priority p1 --from now-1h

  # Use a generic filter expression
  dash0 failed-checks query --filter "owner is sre"

  # Output as JSON
  dash0 failed-checks query --active -o json

  # Output as CSV
  dash0 failed-checks query --priority p1,p2 --active -o csv`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQuery(cmd, flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile) (env: DASH0_API_URL)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile) (env: DASH0_AUTH_TOKEN)")
	cmd.Flags().StringVar(&flags.Dataset, "dataset", "", "Dataset identifier (overrides active profile) (env: DASH0_DATASET)")
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table (default), json, csv")
	cmd.Flags().StringSliceVar(&flags.Priority, "priority", nil, "Filter by priority label: comma-separated list (e.g. --priority p1,p2)")
	cmd.Flags().StringSliceVar(&flags.Status, "status", nil, "Filter by instance status: comma-separated list of resolved, degraded, critical")
	cmd.Flags().BoolVar(&flags.Active, "active", false, "Only show currently active (unresolved) issues")
	cmd.Flags().StringArrayVar(&flags.Filter, "filter", nil, "Filter expression: key [operator] value (repeatable); accepts JSON from the Dash0 UI")
	cmd.Flags().StringVar(&flags.From, "from", "now-15m", "Start of time range (relative or ISO 8601)")
	cmd.Flags().StringVar(&flags.To, "to", "now", "End of time range (relative or ISO 8601)")
	cmd.Flags().IntVarP(&flags.Limit, "limit", "l", 50, "Maximum number of results")
	cmd.Flags().BoolVar(&flags.SkipHeader, "skip-header", false, "Omit the header row from table and csv output")

	return cmd
}

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
		return "", fmt.Errorf("unknown output format: %s (valid formats: table, json, csv)", s)
	}
}

func runQuery(cmd *cobra.Command, flags *queryFlags) error {
	format, err := parseQueryFormat(flags.Output)
	if err != nil {
		return err
	}

	filters, err := buildFilters(flags)
	if err != nil {
		return err
	}

	cfg, err := client.NewRawHTTPConfig(cmd.Context(), flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := cfg.Dataset
	if cmd.Flags().Changed("dataset") {
		dataset = flags.Dataset
	}

	reqBody := failedChecksRequest{
		TimeRange:  requestTimeRange{From: flags.From, To: flags.To},
		Ordering:   []orderingItem{{Key: "dash0.issue.start_time", Direction: "descending"}},
		Pagination: paginationItem{Limit: flags.Limit},
		Dataset:    dataset,
		Filter:     filters,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}

	url := strings.TrimRight(cfg.ApiUrl, "/") + apiPath
	req, err := http.NewRequestWithContext(cmd.Context(), http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", cfg.UserAgent)

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("authentication failed; check your auth token")
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("API error (status: %d):\n  %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}

	var result failedChecksResponse
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	switch format {
	case queryFormatJSON:
		return printJSON(respBytes)
	case queryFormatCSV:
		return printCSV(result.Issues, flags.SkipHeader)
	default:
		return printTable(result.Issues, flags.SkipHeader)
	}
}

// buildFilters combines --filter, --priority, --status, and --active into a FilterCriteria.
func buildFilters(flags *queryFlags) (dash0api.FilterCriteria, error) {
	var filters dash0api.FilterCriteria

	parsed, err := query.ParseFilters(flags.Filter)
	if err != nil {
		return nil, err
	}
	if parsed != nil {
		filters = append(filters, *parsed...)
	}

	if len(flags.Priority) > 0 {
		f, err := query.ParseFilter("priority is_one_of " + strings.Join(flags.Priority, " "))
		if err != nil {
			return nil, fmt.Errorf("invalid --priority value: %w", err)
		}
		filters = append(filters, f)
	}

	if len(flags.Status) > 0 {
		f, err := query.ParseFilter("instanceStatus is_one_of " + strings.Join(flags.Status, " "))
		if err != nil {
			return nil, fmt.Errorf("invalid --status value: %w", err)
		}
		filters = append(filters, f)
	}

	if flags.Active {
		f, err := query.ParseFilter("dash0.issue.end_time is_not_set")
		if err != nil {
			return nil, fmt.Errorf("failed to build active filter: %w", err)
		}
		filters = append(filters, f)
	}

	return filters, nil
}

func labelStringValue(labels []issueLabel, key string) string {
	for _, l := range labels {
		if l.Key == key && l.Value.StringValue != nil {
			return *l.Value.StringValue
		}
	}
	return ""
}

func formatStart(s string) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return s
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

func sprintStatus(status string, width int) string {
	if colorpkg.NoColor {
		return fmt.Sprintf("%-*s", width, status)
	}
	o := colorpkg.StdoutOutput()
	padded := fmt.Sprintf("%-*s", width, status)
	switch strings.ToLower(status) {
	case "critical":
		return o.String(padded).Foreground(o.Color("1")).String() // red
	case "degraded":
		return o.String(padded).Foreground(o.Color("3")).String() // yellow
	case "resolved":
		return o.String(padded).Foreground(o.Color("2")).String() // green
	default:
		return padded
	}
}

func printTable(issues []issue, skipHeader bool) error {
	if len(issues) == 0 {
		fmt.Println("No failed checks found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if !skipHeader {
		fmt.Fprintln(w, "CHECK RULE\tPRIORITY\tSTATUS\tSTARTED\tSUMMARY")
	}
	for _, iss := range issues {
		priority := labelStringValue(iss.Labels, "priority")
		started := formatStart(iss.Start)
		status := sprintStatus(iss.InstanceStatus, 10)
		summary := iss.Summary
		if len(summary) > 60 {
			summary = summary[:57] + "..."
		}
		ruleName := iss.CheckRule.Name
		if len(ruleName) > 45 {
			ruleName = ruleName[:42] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", ruleName, priority, status, started, summary)
	}
	return w.Flush()
}

func printCSV(issues []issue, skipHeader bool) error {
	w := csv.NewWriter(os.Stdout)
	if !skipHeader {
		if err := w.Write([]string{"check_rule", "priority", "status", "started", "summary", "id"}); err != nil {
			return err
		}
	}
	for _, iss := range issues {
		priority := labelStringValue(iss.Labels, "priority")
		if err := w.Write([]string{
			iss.CheckRule.Name,
			priority,
			iss.InstanceStatus,
			formatStart(iss.Start),
			iss.Summary,
			iss.ID,
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func printJSON(raw []byte) error {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		_, err = os.Stdout.Write(raw)
		return err
	}
	_, err := buf.WriteTo(os.Stdout)
	return err
}
