package logging

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/client"
	colorpkg "github.com/dash0hq/dash0-cli/internal/color"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/dash0hq/dash0-cli/internal/otlp"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/dash0hq/dash0-cli/internal/query"
	"github.com/spf13/cobra"
)

type queryFlags struct {
	ApiUrl     string
	AuthToken  string
	Dataset    string
	Output     string
	From       string
	To         string
	Filter     []string
	Limit      int
	SkipHeader bool
	Column     []string
}

// queryFormat represents the output format for log queries.
type queryFormat string

const (
	queryFormatTable queryFormat = "table"
	queryFormatJSON  queryFormat = "json"
	queryFormatCSV   queryFormat = "csv"

	logRecordsAssetType = "log records"
)

// logDefaultColumns defines the default columns for log query output.
var logDefaultColumns = []query.ColumnDef{
	{Key: "otel.log.time", Aliases: []string{"timestamp", "time"}, Header: "TIMESTAMP", Width: 28},
	{Key: "otel.log.severity.range", Aliases: []string{"severity"}, Header: "SEVERITY", Width: 10, ColorFn: colorpkg.SprintSeverity},
	{Key: "otel.log.body", Aliases: []string{"body"}, Header: "BODY", Width: 0},
}

// logKnownColumns extends logDefaultColumns with additional alias entries for
// fields that are not shown by default but should be resolvable by alias.
var logKnownColumns = append(logDefaultColumns,
	query.ColumnDef{Key: "otel.trace.id", Aliases: []string{"trace id"}, Header: "TRACE ID", Width: 32},
	query.ColumnDef{Key: "otel.span.id", Aliases: []string{"span id"}, Header: "SPAN ID", Width: 16},
	query.ColumnDef{Key: "otel.flags", Aliases: []string{"flags"}, Header: "FLAGS", Width: 10},
)

func parseQueryFormat(s string) (queryFormat, error) {
	switch strings.ToLower(s) {
	case "table", "":
		return queryFormatTable, nil
	case "json":
		return queryFormatJSON, nil
	case "csv":
		return queryFormatCSV, nil
	default:
		return "", fmt.Errorf("unknown output format: %s (valid formats: table, json, csv)", s)
	}
}

func newQueryCmd() *cobra.Command {
	flags := &queryFlags{}

	cmd := &cobra.Command{
		Use:   "query",
		Short: "[experimental] Query log records from Dash0",
		Long:  `Query log records from Dash0 and display them in various formats.` + internal.CONFIG_HINT,
		Example: `  # Query recent logs (last 15 minutes, up to 50 records)
  dash0 --experimental logs query

  # Query logs from the last hour
  dash0 --experimental logs query --from now-1h

  # Filter by service name
  dash0 --experimental logs query --filter "service.name is my-service"

  # Filter by severity
  dash0 --experimental logs query --filter "otel.log.severity.range is_one_of ERROR WARN"

  # Combine multiple filters
  dash0 --experimental logs query \
      --filter "service.name is my-service" \
      --filter "otel.log.severity.number gte 13" \
      --from now-1h --limit 100

  # Output as CSV for further processing
  dash0 --experimental logs query -o csv

  # Output as JSON (OTLP/JSON format)
  dash0 --experimental logs query -o json --limit 10

  # Output as CSV without the header row
  dash0 --experimental logs query -o csv --skip-header

  # Show only timestamp and body
  dash0 --experimental logs query --column time --column body

  # Include an arbitrary attribute column
  dash0 --experimental logs query \
      --column time --column severity \
      --column service.name --column body

  Column aliases (case-insensitive):
    time, timestamp  → otel.log.time
    severity         → otel.log.severity.range
    body             → otel.log.body
    trace id         → otel.trace.id
    span id          → otel.span.id
    flags            → otel.flags

  Built-in OTLP fields (always available without attributes):
    otel.log.time
    otel.log.body
    otel.log.severity.range
    otel.log.severity.number
    otel.log.severity.text
    otel.event.name
    otel.trace.id
    otel.span.id
    otel.flags
    otel.scope.name
    otel.scope.version

  Any OTLP attribute key can also be used as a column.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runQuery(cmd, flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVar(&flags.Dataset, "dataset", "", "Dataset name")
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table, json (OTLP/JSON), csv (default: table)")
	cmd.Flags().StringVar(&flags.From, "from", "now-15m", "Start of time range (e.g. now-1h, 2024-01-25T10:00:00.000Z)")
	cmd.Flags().StringVar(&flags.To, "to", "now", "End of time range (e.g. now, 2024-01-25T11:00:00.000Z)")
	cmd.Flags().StringArrayVar(&flags.Filter, "filter", nil, "Filter expression as 'key [operator] value' (repeatable)")
	cmd.Flags().IntVar(&flags.Limit, "limit", 50, "Maximum number of log records to return")
	cmd.Flags().BoolVar(&flags.SkipHeader, "skip-header", false, "Omit the header row from table and CSV output")
	cmd.Flags().StringArrayVar(&flags.Column, "column", nil, "Column to display (alias or attribute key; repeatable; table and CSV only)")

	return cmd
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

	cols, err := resolveLogColumns(flags.Column)
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	filters, err := query.ParseFilters(flags.Filter)
	if err != nil {
		return err
	}

	totalLimit := int64(flags.Limit)

	const jsonMaxLimit int64 = 100
	if format == queryFormatJSON && totalLimit > jsonMaxLimit {
		return fmt.Errorf("json output is limited to %d records; use --limit %d or lower, or choose a different output format", jsonMaxLimit, jsonMaxLimit)
	}

	// Use a reasonable page size for API requests, independent of the total limit.
	const defaultPageSize int64 = 100
	pageSize := defaultPageSize
	if totalLimit > 0 && totalLimit < pageSize {
		pageSize = totalLimit
	}

	request := dash0api.GetLogRecordsRequest{
		TimeRange: dash0api.TimeReferenceRange{
			From: query.NormalizeTimestamp(flags.From),
			To:   query.NormalizeTimestamp(flags.To),
		},
		Dataset: dataset,
		Filter:  filters,
		Pagination: &dash0api.CursorPagination{
			Limit: dash0api.Int64(pageSize),
		},
	}

	iter := apiClient.GetLogRecordsIter(ctx, &request)

	switch format {
	case queryFormatTable:
		return streamTable(iter, totalLimit, flags.SkipHeader, cols)
	case queryFormatCSV:
		return streamCSV(iter, totalLimit, flags.SkipHeader, cols)
	case queryFormatJSON:
		return collectAndRenderJSON(iter, totalLimit)
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}

func resolveLogColumns(columns []string) ([]query.ColumnDef, error) {
	if len(columns) == 0 {
		return logDefaultColumns, nil
	}
	specs, err := query.ParseColumns(columns)
	if err != nil {
		return nil, err
	}
	return query.ResolveColumns(specs, logKnownColumns), nil
}

// flatRecord holds a flattened log record for table/CSV rendering.
type flatRecord struct {
	timestamp      string
	severityRange  string
	severityNumber string
	severityText   string
	body           string
	traceID        string
	spanID         string
	flags          string
	eventName      string
	scopeName      string
	scopeVersion   string
	rawAttrs       []dash0api.KeyValue
}

// values returns a map of predefined column values.
func (r flatRecord) values() map[string]string {
	return map[string]string{
		"otel.log.time":            r.timestamp,
		"otel.log.severity.range":  r.severityRange,
		"otel.log.severity.number": r.severityNumber,
		"otel.log.severity.text":   r.severityText,
		"otel.log.body":            r.body,
		"otel.trace.id":            r.traceID,
		"otel.span.id":             r.spanID,
		"otel.flags":               r.flags,
		"otel.event.name":          r.eventName,
		"otel.scope.name":          r.scopeName,
		"otel.scope.version":       r.scopeVersion,
	}
}

// iterateRecords drives the iterator, flattens each ResourceLogs into flat
// records, and calls emit for each one. It stops after totalLimit records
// (0 means unlimited). Returns the total number of records emitted and any
// iterator error.
func iterateRecords(iter *dash0api.Iter[dash0api.ResourceLogs], totalLimit int64, emit func(flatRecord)) (int64, error) {
	var total int64
	for iter.Next() {
		rl := iter.Current()
		for _, sl := range rl.ScopeLogs {
			var scopeAttrs []dash0api.KeyValue
			var scopeName, scopeVersion string
			if sl.Scope != nil {
				scopeAttrs = sl.Scope.Attributes
				scopeName = otlp.DerefString(sl.Scope.Name)
				scopeVersion = otlp.DerefString(sl.Scope.Version)
			}
			for _, lr := range sl.LogRecords {
				emit(flatRecord{
					timestamp:      formatTimestamp(lr.TimeUnixNano),
					severityRange:  severityRange(lr.SeverityNumber),
					severityNumber: formatSeverityNumber(lr.SeverityNumber),
					severityText:   otlp.DerefString(lr.SeverityText),
					body:           extractBodyString(lr.Body),
					traceID:        otlp.DerefHexBytes(lr.TraceId),
					spanID:         otlp.DerefHexBytes(lr.SpanId),
					flags:          otlp.DerefInt64(lr.Flags),
					eventName:      otlp.DerefString(lr.EventName),
					scopeName:      scopeName,
					scopeVersion:   scopeVersion,
					rawAttrs:       otlp.MergeAttributes(rl.Resource.Attributes, scopeAttrs, lr.Attributes),
				})
				total++
				if totalLimit > 0 && total >= totalLimit {
					return total, iter.Err()
				}
			}
		}
	}
	return total, iter.Err()
}

// countRecords counts the total number of log records in a slice of ResourceLogs.
func countRecords(resourceLogs []dash0api.ResourceLogs) int64 {
	var count int64
	for _, rl := range resourceLogs {
		for _, sl := range rl.ScopeLogs {
			count += int64(len(sl.LogRecords))
		}
	}
	return count
}

func streamTable(iter *dash0api.Iter[dash0api.ResourceLogs], totalLimit int64, skipHeader bool, cols []query.ColumnDef) error {
	var rows []map[string]string

	total, err := iterateRecords(iter, totalLimit, func(r flatRecord) {
		values := query.BuildValues(r.values(), cols, r.rawAttrs)
		rows = append(rows, values)
	})
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: logRecordsAssetType})
	}
	if total == 0 {
		fmt.Println("No log records found.")
		return nil
	}
	query.RenderTable(os.Stdout, cols, rows, skipHeader)
	return nil
}

func streamCSV(iter *dash0api.Iter[dash0api.ResourceLogs], totalLimit int64, skipHeader bool, cols []query.ColumnDef) error {
	w := csv.NewWriter(os.Stdout)
	if !skipHeader {
		if err := query.WriteCSVHeader(w, cols); err != nil {
			return err
		}
		w.Flush()
	}

	_, err := iterateRecords(iter, totalLimit, func(r flatRecord) {
		values := query.BuildValues(r.values(), cols, r.rawAttrs)
		query.WriteCSVRow(w, cols, values)
		w.Flush()
	})
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: logRecordsAssetType})
	}
	return nil
}

func collectAndRenderJSON(iter *dash0api.Iter[dash0api.ResourceLogs], totalLimit int64) error {
	var allResourceLogs []dash0api.ResourceLogs
	var totalRecords int64

	for iter.Next() {
		rl := iter.Current()
		allResourceLogs = append(allResourceLogs, *rl)

		if totalLimit > 0 {
			totalRecords += countRecords([]dash0api.ResourceLogs{*rl})
			if totalRecords >= totalLimit {
				break
			}
		}
	}
	if err := iter.Err(); err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: logRecordsAssetType})
	}

	if allResourceLogs == nil {
		allResourceLogs = []dash0api.ResourceLogs{}
	}
	wrapper := map[string]any{
		"resourceLogs": allResourceLogs,
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(wrapper)
}

// extractBodyString extracts a string representation from an AnyValue.
func extractBodyString(body *dash0api.AnyValue) string {
	if body == nil {
		return ""
	}
	if body.StringValue != nil {
		return *body.StringValue
	}
	if body.IntValue != nil {
		return *body.IntValue
	}
	if body.DoubleValue != nil {
		return strconv.FormatFloat(*body.DoubleValue, 'f', -1, 64)
	}
	if body.BoolValue != nil {
		return strconv.FormatBool(*body.BoolValue)
	}
	return ""
}

// formatTimestamp delegates to the shared query.FormatTimestamp.
func formatTimestamp(nanoStr string) string {
	return query.FormatTimestamp(nanoStr)
}

// severityRange returns the OTel severity range derived from the severity number.
func severityRange(num *int32) string {
	if num != nil {
		return otlp.SeverityNumberToRange(*num)
	}
	return ""
}

// formatSeverityNumber returns the severity number as a string, or "" if nil.
func formatSeverityNumber(num *int32) string {
	if num != nil {
		return strconv.FormatInt(int64(*num), 10)
	}
	return ""
}
