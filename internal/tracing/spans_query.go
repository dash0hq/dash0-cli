package tracing

import (
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
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

// queryFormat represents the output format for span queries.
type queryFormat string

const (
	queryFormatTable queryFormat = "table"
	queryFormatJSON  queryFormat = "json"
	queryFormatCSV   queryFormat = "csv"

	spansAssetType = "spans"
)

// spanQueryDefaultColumns defines the default columns for span query output.
var spanQueryDefaultColumns = []query.ColumnDef{
	{Key: "otel.span.start_time", Aliases: []string{"timestamp", "start time", "time"}, Header: "TIMESTAMP", Width: 28},
	{Key: "otel.span.duration", Aliases: []string{"duration"}, Header: "DURATION", Width: 10},
	{Key: "otel.span.name", Aliases: []string{"span name", "name"}, Header: "SPAN NAME", Width: 30},
	{Key: "otel.span.status.code", Aliases: []string{"status", "status code"}, Header: "STATUS", Width: 8, ColorFn: colorpkg.SprintSpanStatus},
	{Key: "service.name", Aliases: []string{"service name", "service"}, Header: "SERVICE NAME", Width: 30},
	{Key: "otel.parent.id", Aliases: []string{"parent id"}, Header: "PARENT ID", Width: 16},
	{Key: "otel.trace.id", Aliases: []string{"trace id"}, Header: "TRACE ID", Width: 32},
	{Key: "otel.span.id", Aliases: []string{"span id"}, Header: "SPAN ID", Width: 16},
	{Key: "otel.span.links", Aliases: []string{"span links", "links"}, Header: "SPAN LINKS", Width: 0},
}

// spanKnownColumns extends spanQueryDefaultColumns with additional alias entries
// for fields that are not shown by default but should be resolvable by alias.
var spanKnownColumns = append(spanQueryDefaultColumns,
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
		Short: "[experimental] Query spans from Dash0",
		Long:  `Query spans from Dash0 and display them in various formats.` + internal.CONFIG_HINT,
		Example: `  # Query recent spans (last 15 minutes, up to 50 records)
  dash0 --experimental spans query

  # Query spans from the last hour
  dash0 --experimental spans query --from now-1h

  # Filter by service name
  dash0 --experimental spans query --filter "service.name is my-service"

  # Filter by span status
  dash0 --experimental spans query --filter "otel.span.status.code is ERROR"

  # Combine multiple filters
  dash0 --experimental spans query \
      --filter "service.name is my-service" \
      --filter "otel.span.status.code is ERROR" \
      --from now-1h --limit 100

  # Output as CSV for further processing
  dash0 --experimental spans query -o csv

  # Output as JSON (OTLP/JSON format)
  dash0 --experimental spans query -o json --limit 10

  # Show only specific columns
  dash0 --experimental spans query \
      --column timestamp --column duration \
      --column "span name" --column status

  # Include an arbitrary attribute column
  dash0 --experimental spans query \
      --column timestamp --column duration \
      --column "span name" --column http.request.method

  Column aliases (case-insensitive):
    timestamp, start time, time  → otel.span.start_time
    duration                     → otel.span.duration
    span name, name              → otel.span.name
    status, status code          → otel.span.status.code
    service name, service        → service.name
    parent id                    → otel.parent.id
    trace id                     → otel.trace.id
    span id                      → otel.span.id
    span links, links            → otel.span.links
    flags                        → otel.flags

  Built-in OTLP fields (always available without attributes):
    otel.span.name
    otel.span.start_time
    otel.span.duration
    otel.span.kind
    otel.span.status.code
    otel.span.status.message
    otel.trace.id
    otel.span.id
    otel.parent.id
    otel.trace.state
    otel.flags
    otel.span.links
    otel.scope.name
    otel.scope.version

  Any OTLP attribute key can also be used as a column.

  NOTE: span events are currently not supported.`,
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
	cmd.Flags().IntVar(&flags.Limit, "limit", 50, "Maximum number of spans to return")
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

	cols, err := resolveSpanQueryColumns(flags.Column)
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

	const defaultPageSize int64 = 100
	pageSize := defaultPageSize
	if totalLimit > 0 && totalLimit < pageSize {
		pageSize = totalLimit
	}

	request := dash0api.GetSpansRequest{
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

	iter := apiClient.GetSpansIter(ctx, &request)

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

func resolveSpanQueryColumns(columns []string) ([]query.ColumnDef, error) {
	if len(columns) == 0 {
		return spanQueryDefaultColumns, nil
	}
	specs, err := query.ParseColumns(columns)
	if err != nil {
		return nil, err
	}
	return query.ResolveColumns(specs, spanKnownColumns), nil
}

// flatSpanRecord holds a flattened span for table/CSV rendering.
type flatSpanRecord struct {
	timestamp     string
	duration      string
	name          string
	kind          string
	statusCode    string
	statusMessage string
	scopeName     string
	scopeVersion  string
	traceID       string
	spanID        string
	parentID      string
	traceState    string
	flags         string
	spanLinks     string
	rawAttrs      []dash0api.KeyValue
}

// values returns a map of predefined column values.
func (r flatSpanRecord) values() map[string]string {
	return map[string]string{
		"otel.span.start_time":     r.timestamp,
		"otel.span.duration":       r.duration,
		"otel.span.name":           r.name,
		"otel.span.kind":           r.kind,
		"otel.span.status.code":    r.statusCode,
		"otel.span.status.message": r.statusMessage,
		"otel.scope.name":          r.scopeName,
		"otel.scope.version":       r.scopeVersion,
		"otel.parent.id":           r.parentID,
		"otel.trace.id":            r.traceID,
		"otel.span.id":             r.spanID,
		"otel.trace.state":         r.traceState,
		"otel.flags":               r.flags,
		"otel.span.links":          r.spanLinks,
	}
}

// iterateSpans drives the iterator, flattens each ResourceSpans into flat
// records, and calls emit for each one. It stops after totalLimit records
// (0 means unlimited).
func iterateSpans(iter *dash0api.Iter[dash0api.ResourceSpans], totalLimit int64, emit func(flatSpanRecord)) (int64, error) {
	var total int64
	for iter.Next() {
		rs := iter.Current()
		for _, ss := range rs.ScopeSpans {
			var scopeAttrs []dash0api.KeyValue
			var scopeName, scopeVersion string
			if ss.Scope != nil {
				scopeAttrs = ss.Scope.Attributes
				scopeName = otlp.DerefString(ss.Scope.Name)
				scopeVersion = otlp.DerefString(ss.Scope.Version)
			}
			for _, s := range ss.Spans {
				var parentID string
				if s.ParentSpanId != nil {
					parentID = hex.EncodeToString(*s.ParentSpanId)
				}
				emit(flatSpanRecord{
					timestamp:     query.FormatTimestamp(s.StartTimeUnixNano),
					duration:      FormatDuration(s.StartTimeUnixNano, s.EndTimeUnixNano),
					name:          s.Name,
					kind:          SpanKindString(s.Kind),
					statusCode:    SpanStatusString(s.Status.Code),
					statusMessage: otlp.DerefString(s.Status.Message),
					scopeName:     scopeName,
					scopeVersion:  scopeVersion,
					traceID:       hex.EncodeToString(s.TraceId),
					spanID:        hex.EncodeToString(s.SpanId),
					parentID:      parentID,
					traceState:    otlp.DerefString(s.TraceState),
					flags:         otlp.DerefInt64(s.Flags),
					spanLinks:     FormatSpanLinks(s.Links),
					rawAttrs:      otlp.MergeAttributes(rs.Resource.Attributes, scopeAttrs, s.Attributes),
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

// countSpans counts the total number of spans in a slice of ResourceSpans.
func countSpans(resourceSpans []dash0api.ResourceSpans) int64 {
	var count int64
	for _, rs := range resourceSpans {
		for _, ss := range rs.ScopeSpans {
			count += int64(len(ss.Spans))
		}
	}
	return count
}

func streamTable(iter *dash0api.Iter[dash0api.ResourceSpans], totalLimit int64, skipHeader bool, cols []query.ColumnDef) error {
	var rows []map[string]string

	total, err := iterateSpans(iter, totalLimit, func(r flatSpanRecord) {
		values := query.BuildValues(r.values(), cols, r.rawAttrs)
		rows = append(rows, values)
	})
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: spansAssetType})
	}
	if total == 0 {
		fmt.Println("No spans found.")
		return nil
	}
	query.RenderTable(os.Stdout, cols, rows, skipHeader)
	return nil
}

func streamCSV(iter *dash0api.Iter[dash0api.ResourceSpans], totalLimit int64, skipHeader bool, cols []query.ColumnDef) error {
	w := csv.NewWriter(os.Stdout)
	if !skipHeader {
		if err := query.WriteCSVHeader(w, cols); err != nil {
			return err
		}
		w.Flush()
	}

	_, err := iterateSpans(iter, totalLimit, func(r flatSpanRecord) {
		values := query.BuildValues(r.values(), cols, r.rawAttrs)
		query.WriteCSVRow(w, cols, values)
		w.Flush()
	})
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: spansAssetType})
	}
	return nil
}

func collectAndRenderJSON(iter *dash0api.Iter[dash0api.ResourceSpans], totalLimit int64) error {
	var allResourceSpans []dash0api.ResourceSpans
	var totalSpans int64

	for iter.Next() {
		rs := iter.Current()
		allResourceSpans = append(allResourceSpans, *rs)

		if totalLimit > 0 {
			totalSpans += countSpans([]dash0api.ResourceSpans{*rs})
			if totalSpans >= totalLimit {
				break
			}
		}
	}
	if err := iter.Err(); err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: spansAssetType})
	}

	if allResourceSpans == nil {
		allResourceSpans = []dash0api.ResourceSpans{}
	}
	wrapper := map[string]any{
		"resourceSpans": allResourceSpans,
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(wrapper)
}
