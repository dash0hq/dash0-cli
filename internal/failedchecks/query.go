package failedchecks

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/client"
	colorpkg "github.com/dash0hq/dash0-cli/internal/color"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/dash0hq/dash0-cli/internal/query"
	"github.com/spf13/cobra"
)

const failedChecksAssetType = "failed checks"

type queryFlags struct {
	ApiUrl     string
	AuthToken  string
	Dataset    string
	Output     string
	Status     []string
	Active     bool
	Filter     []string
	From       string
	To         string
	Limit      int
	SkipHeader bool
	Column     []string
}

type queryFormat string

const (
	queryFormatTable queryFormat = "table"
	queryFormatJSON  queryFormat = "json"
	queryFormatCSV   queryFormat = "csv"
)

// failedCheckDefaultColumns defines the default columns for failed-checks query output.
var failedCheckDefaultColumns = []query.ColumnDef{
	{Key: "dash0.issue.check_rule_name", Aliases: []string{"check rule", "rule"}, Header: "CHECK RULE", Width: 45},
	{Key: "dash0.issue.status", Aliases: []string{"status"}, Header: "STATUS", Width: 10, ColorFn: colorpkg.SprintIssueStatus},
	{Key: "dash0.issue.start_time", Aliases: []string{"started", "start", "start time"}, Header: "STARTED", Width: 20},
	{Key: "dash0.issue.summary", Aliases: []string{"summary"}, Header: "SUMMARY", Width: 0},
}

// failedCheckKnownColumns extends failedCheckDefaultColumns with additional alias
// entries for fields that are not shown by default but should be resolvable by alias.
var failedCheckKnownColumns = append(failedCheckDefaultColumns,
	query.ColumnDef{Key: "dash0.issue.id", Aliases: []string{"id"}, Header: "ID", Width: 36},
	query.ColumnDef{Key: "dash0.issue.identifier", Aliases: []string{"identifier"}, Header: "IDENTIFIER", Width: 20},
	query.ColumnDef{Key: "dash0.issue.description", Aliases: []string{"description"}, Header: "DESCRIPTION", Width: 0},
	query.ColumnDef{Key: "dash0.issue.end_time", Aliases: []string{"ended", "end", "end time"}, Header: "ENDED", Width: 20},
	query.ColumnDef{Key: "dash0.issue.check_rule_id", Aliases: []string{"check rule id", "rule id"}, Header: "CHECK RULE ID", Width: 36},
)

func newQueryCmd() *cobra.Command {
	flags := &queryFlags{}

	cmd := &cobra.Command{
		Use:     "query",
		Aliases: []string{"list", "ls"},
		Short:   "Query failed check instances",
		Long:    `Query failed check instances from Dash0 alerting within a time range.` + internal.CONFIG_HINT,
		Example: `  # List all currently active (unresolved) issues
  dash0 failed-checks query --active

  # Filter by status (critical, degraded, healthy, inactive, pending)
  dash0 failed-checks query --status critical,degraded

  # Filter by an arbitrary issue label (priority, owner, …)
  dash0 failed-checks query --filter "priority is_one_of p1 p2" --active

  # Issues from the last hour (active and resolved)
  dash0 failed-checks query --filter "priority is p1" --from now-1h

  # Use JSON filter criteria copied from the Dash0 UI
  dash0 failed-checks query \
      --filter '[{"key":"priority","operator":"is","value":"p1"}]'

  # Output as JSON
  dash0 failed-checks query --active -o json

  # Output as CSV
  dash0 failed-checks query --status critical --active -o csv

  # Show only specific columns
  dash0 failed-checks query \
      --column "check rule" --column status --column summary

  # Include an issue label (priority, owner, …) as its own column
  dash0 failed-checks query \
      --column "check rule" --column priority --column status

  Column aliases (case-insensitive):
    check rule, rule        → dash0.issue.check_rule_name
    status                  → dash0.issue.status
    started, start          → dash0.issue.start_time
    ended, end              → dash0.issue.end_time
    summary                 → dash0.issue.summary
    description             → dash0.issue.description
    id                      → dash0.issue.id
    identifier              → dash0.issue.identifier
    check rule id, rule id  → dash0.issue.check_rule_id

  Issue label keys (such as priority or owner) can be used directly as column names.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runQuery(cmd, flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVar(&flags.Dataset, "dataset", "", "Dataset name")
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table, json, csv (default: table)")
	cmd.Flags().StringSliceVar(&flags.Status, "status", nil, "Filter by instance status: comma-separated list of critical, degraded, healthy, inactive, pending")
	cmd.Flags().BoolVar(&flags.Active, "active", false, "Only show currently active (unresolved) issues")
	cmd.Flags().StringArrayVar(&flags.Filter, "filter", nil, "Filter expression as 'key [operator] value', or a JSON array/object from the Dash0 UI (repeatable)")
	cmd.Flags().StringVar(&flags.From, "from", "now-15m", "Start of time range (e.g. now-1h, 2024-01-25T10:00:00.000Z)")
	cmd.Flags().StringVar(&flags.To, "to", "now", "End of time range (e.g. now, 2024-01-25T11:00:00.000Z)")
	cmd.Flags().IntVarP(&flags.Limit, "limit", "l", 50, "Maximum number of failed checks to return")
	cmd.Flags().BoolVar(&flags.SkipHeader, "skip-header", false, "Omit the header row from table and CSV output")
	cmd.Flags().StringArrayVar(&flags.Column, "column", nil, "Column to display (alias or attribute key; repeatable; table and CSV only)")

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
	ctx := cmd.Context()

	if err := output.ValidateSkipHeader(flags.SkipHeader, flags.Output); err != nil {
		return err
	}

	if err := query.ValidateColumnFormat(flags.Column, flags.Output); err != nil {
		return err
	}

	format, err := parseQueryFormat(flags.Output)
	if err != nil {
		return err
	}

	cols, err := resolveColumns(flags.Column)
	if err != nil {
		return err
	}

	filters, err := buildFilters(flags)
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	totalLimit := int64(flags.Limit)

	const jsonMaxLimit int64 = 100
	if format == queryFormatJSON && totalLimit > jsonMaxLimit {
		return fmt.Errorf("json output is limited to %d records; use --limit %d or lower, or choose a different output format", jsonMaxLimit, jsonMaxLimit)
	}

	const defaultPageSize int64 = 100
	pageSize := defaultPageSize
	if totalLimit > 0 && totalLimit < pageSize {
		pageSize = totalLimit
	}

	timeRange := dash0api.TimeReferenceRange{
		From: query.NormalizeTimestamp(flags.From),
		To:   query.NormalizeTimestamp(flags.To),
	}

	ordering := dash0api.OrderingCriteria{
		{Key: "dash0.issue.start_time", Direction: "descending"},
	}

	request := dash0api.GetFailedChecksRequest{
		TimeRange:  timeRange,
		Dataset:    dataset,
		Filter:     filters,
		Ordering:   &ordering,
		Pagination: &dash0api.CursorPagination{Limit: dash0api.Int64(pageSize)},
	}

	apiUrl := client.ResolveApiUrl(ctx, flags.ApiUrl)
	deeplinkFilters := dash0api.FiltersToDeeplinkFilters(filters)
	explorerURL := dash0api.FailedChecksExplorerURL(apiUrl, deeplinkFilters, flags.From, flags.To, dataset)

	iter := apiClient.GetFailedChecksIter(ctx, &request)

	switch format {
	case queryFormatTable:
		return streamTable(iter, totalLimit, flags.SkipHeader, cols, explorerURL)
	case queryFormatCSV:
		return streamCSV(iter, totalLimit, flags.SkipHeader, cols)
	case queryFormatJSON:
		return collectAndRenderJSON(iter, totalLimit)
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}

func resolveColumns(columns []string) ([]query.ColumnDef, error) {
	if len(columns) == 0 {
		return failedCheckDefaultColumns, nil
	}
	specs, err := query.ParseColumns(columns)
	if err != nil {
		return nil, err
	}
	return query.ResolveColumns(specs, failedCheckKnownColumns), nil
}

// buildFilters combines --filter, --status, and --active into a FilterCriteria.
func buildFilters(flags *queryFlags) (*dash0api.FilterCriteria, error) {
	filters, err := query.ParseFilters(flags.Filter)
	if err != nil {
		return nil, err
	}

	var criteria dash0api.FilterCriteria
	if filters != nil {
		criteria = append(criteria, *filters...)
	}

	if len(flags.Status) > 0 {
		f, err := query.ParseFilter("dash0.issue.status is_one_of " + strings.Join(flags.Status, " "))
		if err != nil {
			return nil, fmt.Errorf("invalid --status value: %w", err)
		}
		criteria = append(criteria, f)
	}

	if flags.Active {
		f, err := query.ParseFilter("dash0.issue.end_time is_not_set")
		if err != nil {
			return nil, fmt.Errorf("failed to build active filter: %w", err)
		}
		criteria = append(criteria, f)
	}

	if len(criteria) == 0 {
		return nil, nil
	}
	return &criteria, nil
}

// flatIssue holds a flattened issue for table/CSV rendering.
type flatIssue struct {
	id            string
	identifier    string
	checkRuleID   string
	checkRuleName string
	status        string
	summary       string
	description   string
	start         string
	end           string
	rawLabels     []dash0api.KeyValue
}

// values returns a map of predefined column values.
func (f flatIssue) values() map[string]string {
	return map[string]string{
		"dash0.issue.id":              f.id,
		"dash0.issue.identifier":      f.identifier,
		"dash0.issue.check_rule_id":   f.checkRuleID,
		"dash0.issue.check_rule_name": f.checkRuleName,
		"dash0.issue.status":          f.status,
		"dash0.issue.summary":         f.summary,
		"dash0.issue.description":     f.description,
		"dash0.issue.start_time":      f.start,
		"dash0.issue.end_time":        f.end,
	}
}

// flattenIssue converts an API Issue into a flatIssue ready for column rendering.
func flattenIssue(iss *dash0api.Issue) flatIssue {
	return flatIssue{
		id:            iss.Id,
		identifier:    iss.IssueIdentifier,
		checkRuleID:   iss.CheckRule.Id,
		checkRuleName: iss.CheckRule.Name,
		status:        string(iss.InstanceStatus),
		summary:       iss.Summary,
		description:   iss.Description,
		start:         formatIssueTime(iss.Start),
		end:           formatIssueEnd(iss.End),
		rawLabels:     iss.Labels,
	}
}

// formatIssueTime parses an RFC 3339 timestamp and re-renders it in compact UTC form.
func formatIssueTime(s string) string {
	if s == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return s
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

// formatIssueEnd treats the zero timestamp as "still active" and renders an empty string.
func formatIssueEnd(s string) string {
	if s == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return s
	}
	if t.IsZero() || t.Unix() == 0 {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

// iterateIssues drives the iterator, calling emit for each issue.
// It stops after totalLimit issues (0 means unlimited).
func iterateIssues(iter *dash0api.Iter[dash0api.Issue], totalLimit int64, emit func(flatIssue)) (int64, error) {
	var total int64
	for iter.Next() {
		emit(flattenIssue(iter.Current()))
		total++
		if totalLimit > 0 && total >= totalLimit {
			return total, iter.Err()
		}
	}
	return total, iter.Err()
}

func streamTable(iter *dash0api.Iter[dash0api.Issue], totalLimit int64, skipHeader bool, cols []query.ColumnDef, explorerURL string) error {
	var rows []map[string]string

	total, err := iterateIssues(iter, totalLimit, func(f flatIssue) {
		values := query.BuildValues(f.values(), cols, f.rawLabels)
		rows = append(rows, values)
	})
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: failedChecksAssetType})
	}
	if total == 0 {
		fmt.Println("No failed checks found.")
	} else {
		query.RenderTable(os.Stdout, cols, rows, skipHeader)
	}
	if explorerURL != "" {
		fmt.Printf("\nOpen this query in Dash0:\n    %s\n", explorerURL)
	}
	return nil
}

func streamCSV(iter *dash0api.Iter[dash0api.Issue], totalLimit int64, skipHeader bool, cols []query.ColumnDef) error {
	w := csv.NewWriter(os.Stdout)
	if !skipHeader {
		if err := query.WriteCSVHeader(w, cols); err != nil {
			return err
		}
		w.Flush()
	}

	_, err := iterateIssues(iter, totalLimit, func(f flatIssue) {
		values := query.BuildValues(f.values(), cols, f.rawLabels)
		if err := query.WriteCSVRow(w, cols, values); err != nil {
			return
		}
		w.Flush()
	})
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: failedChecksAssetType})
	}
	return nil
}

func collectAndRenderJSON(iter *dash0api.Iter[dash0api.Issue], totalLimit int64) error {
	var issues []dash0api.Issue
	var total int64
	for iter.Next() {
		issues = append(issues, *iter.Current())
		total++
		if totalLimit > 0 && total >= totalLimit {
			break
		}
	}
	if err := iter.Err(); err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: failedChecksAssetType})
	}

	if issues == nil {
		issues = []dash0api.Issue{}
	}
	wrapper := map[string]any{
		"issues": issues,
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(wrapper)
}
