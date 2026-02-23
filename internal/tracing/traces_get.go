package tracing

import (
	"context"
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

type getFlags struct {
	ApiUrl          string
	AuthToken       string
	Dataset         string
	Output          string
	From            string
	To              string
	SkipHeader      bool
	FollowSpanLinks string
	Column          []string
}

// getFormat represents the output format for trace queries.
type getFormat string

const (
	getFormatTable getFormat = "table"
	getFormatJSON  getFormat = "json"
	getFormatCSV   getFormat = "csv"

	tracesAssetType = "traces"

	maxFollowedTraces = 20
)

// traceDefaultColumns defines the default columns for traces get output.
var traceDefaultColumns = []query.ColumnDef{
	{Key: "otel.span.start_time", Aliases: []string{"timestamp", "start time", "time"}, Header: "TIMESTAMP", Width: 28},
	{Key: "otel.span.duration", Aliases: []string{"duration"}, Header: "DURATION", Width: 10},
	{Key: "otel.trace.id", Aliases: []string{"trace id"}, Header: "TRACE ID", Width: 32},
	{Key: "otel.span.id", Aliases: []string{"span id"}, Header: "SPAN ID", Width: 16},
	{Key: "otel.parent.id", Aliases: []string{"parent id"}, Header: "PARENT ID", Width: 16},
	{Key: "otel.span.name", Aliases: []string{"span name", "name"}, Header: "SPAN NAME", Width: 42, ColorFn: nil},
	{Key: "otel.span.status.code", Aliases: []string{"status", "status code"}, Header: "STATUS", Width: 8, ColorFn: colorpkg.SprintSpanStatus},
	{Key: "service.name", Aliases: []string{"service name", "service"}, Header: "SERVICE NAME", Width: 30},
	{Key: "otel.span.links", Aliases: []string{"span links", "links"}, Header: "SPAN LINKS", Width: 0},
}

// traceKnownColumns extends traceDefaultColumns with additional alias entries
// for fields that are not shown by default but should be resolvable by alias.
var traceKnownColumns = append(traceDefaultColumns,
	query.ColumnDef{Key: "otel.flags", Aliases: []string{"flags"}, Header: "FLAGS", Width: 10},
)

func parseGetFormat(s string) (getFormat, error) {
	switch strings.ToLower(s) {
	case "table", "":
		return getFormatTable, nil
	case "json":
		return getFormatJSON, nil
	case "csv":
		return getFormatCSV, nil
	default:
		return "", fmt.Errorf("unknown output format: %s (valid formats: table, json, csv)", s)
	}
}

func newGetCmd() *cobra.Command {
	flags := &getFlags{}

	cmd := &cobra.Command{
		Use:   "get <trace-id>",
		Short: "[experimental] Get all spans in a trace",
		Long:  `Retrieve all spans belonging to a trace from Dash0.` + internal.CONFIG_HINT,
		Example: `  # Get all spans in a trace
  dash0 --experimental traces get <trace-id>

  # Get a trace with a specific time range
  dash0 --experimental traces get <trace-id> --from now-2h

  # Follow span links to related traces
  dash0 --experimental traces get <trace-id> --follow-span-links

  # Follow span links with a custom lookback period
  dash0 --experimental traces get <trace-id> --follow-span-links 2h

  # Output as JSON (OTLP/JSON format)
  dash0 --experimental traces get <trace-id> -o json

  # Show only specific columns
  dash0 --experimental traces get <trace-id> \
      --column timestamp --column duration \
      --column "span name" --column status

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
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runGet(cmd, args[0], flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVar(&flags.Dataset, "dataset", "", "Dataset name")
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table, json (OTLP/JSON), csv (default: table)")
	cmd.Flags().StringVar(&flags.From, "from", "now-1h", "Start of time range (e.g. now-1h, 2024-01-25T10:00:00.000Z)")
	cmd.Flags().StringVar(&flags.To, "to", "now", "End of time range (e.g. now, 2024-01-25T11:00:00.000Z)")
	cmd.Flags().BoolVar(&flags.SkipHeader, "skip-header", false, "Omit the header row from table and CSV output")
	cmd.Flags().StringVar(&flags.FollowSpanLinks, "follow-span-links", "", "Follow span links to related traces; optional value sets the lookback period (default: 1h)")
	cmd.Flags().Lookup("follow-span-links").NoOptDefVal = "1h"
	cmd.Flags().StringArrayVar(&flags.Column, "column", nil, "Column to display (alias or attribute key; repeatable; table and CSV only)")

	return cmd
}

// traceGroup holds the spans for one trace ID.
type traceGroup struct {
	traceID       string
	resourceSpans []dash0api.ResourceSpans
}

func runGet(cmd *cobra.Command, traceID string, flags *getFlags) error {
	ctx := cmd.Context()

	if err := output.ValidateSkipHeader(flags.SkipHeader, flags.Output); err != nil {
		return err
	}

	if err := query.ValidateColumnFormat(flags.Column, flags.Output); err != nil {
		return err
	}

	format, err := parseGetFormat(flags.Output)
	if err != nil {
		return err
	}

	cols, err := resolveTraceColumns(flags.Column)
	if err != nil {
		return err
	}

	if len(traceID) != 32 {
		return fmt.Errorf("trace-id must be 32 hex characters, got %d", len(traceID))
	}
	if _, err := hex.DecodeString(traceID); err != nil {
		return fmt.Errorf("trace-id must be valid hex: %w", err)
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	timeRange := dash0api.TimeReferenceRange{
		From: query.NormalizeTimestamp(flags.From),
		To:   query.NormalizeTimestamp(flags.To),
	}

	allResourceSpans, err := fetchTraceSpans(ctx, apiClient, traceID, timeRange, dataset)
	if err != nil {
		return err
	}

	results := []traceGroup{{traceID: traceID, resourceSpans: allResourceSpans}}

	followLinks := cmd.Flags().Changed("follow-span-links")
	if followLinks {
		followRange := flags.FollowSpanLinks
		if followRange == "" {
			followRange = "1h"
		}
		followTimeRange := dash0api.TimeReferenceRange{
			From: "now-" + followRange,
			To:   "now",
		}

		seen := map[string]bool{traceID: true}
		queue := extractLinkedTraceIDs(allResourceSpans, seen)

		for len(queue) > 0 && len(results) < maxFollowedTraces {
			nextTraceID := queue[0]
			queue = queue[1:]

			linkedSpans, err := fetchTraceSpans(ctx, apiClient, nextTraceID, followTimeRange, dataset)
			if err != nil {
				return fmt.Errorf("failed to fetch linked trace %s: %w", nextTraceID, err)
			}
			results = append(results, traceGroup{traceID: nextTraceID, resourceSpans: linkedSpans})

			newLinks := extractLinkedTraceIDs(linkedSpans, seen)
			queue = append(queue, newLinks...)
		}
	}

	switch format {
	case getFormatTable:
		return renderTable(results, flags.SkipHeader, cols)
	case getFormatCSV:
		return renderCSV(results, flags.SkipHeader, cols)
	case getFormatJSON:
		return renderJSON(results)
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}

func resolveTraceColumns(columns []string) ([]query.ColumnDef, error) {
	if len(columns) == 0 {
		return traceDefaultColumns, nil
	}
	specs, err := query.ParseColumns(columns)
	if err != nil {
		return nil, err
	}
	return query.ResolveColumns(specs, traceKnownColumns), nil
}

func fetchTraceSpans(ctx context.Context, apiClient dash0api.Client, traceID string, timeRange dash0api.TimeReferenceRange, dataset *string) ([]dash0api.ResourceSpans, error) {
	filter := dash0api.FilterCriteria{
		{
			Key:      "otel.trace.id",
			Operator: dash0api.AttributeFilterOperatorIs,
		},
	}
	var val dash0api.AttributeFilter_Value
	if err := val.FromAttributeFilterStringValue(traceID); err != nil {
		return nil, fmt.Errorf("failed to build filter value: %w", err)
	}
	filter[0].Value = &val

	request := dash0api.GetSpansRequest{
		TimeRange:  timeRange,
		Dataset:    dataset,
		Filter:     &filter,
		Pagination: &dash0api.CursorPagination{Limit: dash0api.Int64(100)},
	}

	iter := apiClient.GetSpansIter(ctx, &request)
	var result []dash0api.ResourceSpans
	for iter.Next() {
		result = append(result, *iter.Current())
	}
	if err := iter.Err(); err != nil {
		return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: tracesAssetType})
	}
	return result, nil
}

// flatTraceSpan holds a flattened span for table/CSV rendering in a trace context.
type flatTraceSpan struct {
	timestamp     string
	duration      string
	spanID        string
	name          string
	kind          string
	statusCode    string
	statusMessage string
	scopeName     string
	scopeVersion  string
	parentID      string
	traceState    string
	flags         string
	spanLinks     string
	rawAttrs      []dash0api.KeyValue
}

// values returns a map of predefined column values. The trace ID is injected
// separately from the traceGroup context.
func (s flatTraceSpan) values(traceID string) map[string]string {
	return map[string]string{
		"otel.span.start_time":     s.timestamp,
		"otel.span.duration":       s.duration,
		"otel.trace.id":            traceID,
		"otel.span.id":             s.spanID,
		"otel.parent.id":           s.parentID,
		"otel.span.name":           s.name,
		"otel.span.kind":           s.kind,
		"otel.span.status.code":    s.statusCode,
		"otel.span.status.message": s.statusMessage,
		"otel.scope.name":          s.scopeName,
		"otel.scope.version":       s.scopeVersion,
		"otel.trace.state":         s.traceState,
		"otel.flags":               s.flags,
		"otel.span.links":          s.spanLinks,
	}
}

func flattenSpans(resourceSpans []dash0api.ResourceSpans) []flatTraceSpan {
	var spans []flatTraceSpan
	for _, rs := range resourceSpans {
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
				spans = append(spans, flatTraceSpan{
					timestamp:     query.FormatTimestamp(s.StartTimeUnixNano),
					duration:      FormatDuration(s.StartTimeUnixNano, s.EndTimeUnixNano),
					spanID:        hex.EncodeToString(s.SpanId),
					name:          s.Name,
					kind:          SpanKindString(s.Kind),
					statusCode:    SpanStatusString(s.Status.Code),
					statusMessage: otlp.DerefString(s.Status.Message),
					scopeName:     scopeName,
					scopeVersion:  scopeVersion,
					parentID:      parentID,
					traceState:    otlp.DerefString(s.TraceState),
					flags:         otlp.DerefInt64(s.Flags),
					spanLinks:     FormatSpanLinks(s.Links),
					rawAttrs:      otlp.MergeAttributes(rs.Resource.Attributes, scopeAttrs, s.Attributes),
				})
			}
		}
	}
	return spans
}

// buildTree walks spans depth-first from roots, assigning indentation depths.
func buildTree(spans []flatTraceSpan) []flatTraceSpan {
	if len(spans) == 0 {
		return spans
	}

	byID := make(map[string]*flatTraceSpan, len(spans))
	children := make(map[string][]string)
	spanIDs := make(map[string]bool, len(spans))

	for i := range spans {
		byID[spans[i].spanID] = &spans[i]
		spanIDs[spans[i].spanID] = true
	}

	for _, s := range spans {
		if s.parentID != "" && spanIDs[s.parentID] {
			children[s.parentID] = append(children[s.parentID], s.spanID)
		}
	}

	var roots []string
	for _, s := range spans {
		if s.parentID == "" || !spanIDs[s.parentID] {
			roots = append(roots, s.spanID)
		}
	}

	var result []flatTraceSpan
	visited := make(map[string]bool)
	var walk func(id string)
	walk = func(id string) {
		if visited[id] {
			return
		}
		visited[id] = true
		if s, ok := byID[id]; ok {
			result = append(result, *s)
		}
		for _, childID := range children[id] {
			walk(childID)
		}
	}
	for _, rootID := range roots {
		walk(rootID)
	}

	for _, s := range spans {
		if !visited[s.spanID] {
			result = append(result, s)
		}
	}

	return result
}

func renderTable(results []traceGroup, skipHeader bool, cols []query.ColumnDef) error {
	var rows []map[string]string

	for _, tr := range results {
		spans := flattenSpans(tr.resourceSpans)
		ordered := buildTree(spans)
		for _, s := range ordered {
			values := query.BuildValues(s.values(tr.traceID), cols, s.rawAttrs)
			rows = append(rows, values)
		}
	}
	if len(rows) == 0 {
		fmt.Println("No spans found for this trace.")
		return nil
	}
	query.RenderTable(os.Stdout, cols, rows, skipHeader)
	return nil
}

func renderCSV(results []traceGroup, skipHeader bool, cols []query.ColumnDef) error {
	w := csv.NewWriter(os.Stdout)
	if !skipHeader {
		if err := query.WriteCSVHeader(w, cols); err != nil {
			return err
		}
		w.Flush()
	}

	for _, tr := range results {
		spans := flattenSpans(tr.resourceSpans)
		ordered := buildTree(spans)
		for _, s := range ordered {
			values := query.BuildValues(s.values(tr.traceID), cols, s.rawAttrs)
			query.WriteCSVRow(w, cols, values)
			w.Flush()
		}
	}
	return nil
}

func renderJSON(results []traceGroup) error {
	var all []dash0api.ResourceSpans
	for _, tr := range results {
		all = append(all, tr.resourceSpans...)
	}
	if all == nil {
		all = []dash0api.ResourceSpans{}
	}
	wrapper := map[string]any{
		"resourceSpans": all,
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(wrapper)
}

func extractLinkedTraceIDs(resourceSpans []dash0api.ResourceSpans, seen map[string]bool) []string {
	var newTraceIDs []string
	for _, rs := range resourceSpans {
		for _, ss := range rs.ScopeSpans {
			for _, s := range ss.Spans {
				for _, link := range s.Links {
					tid := hex.EncodeToString(link.TraceId)
					if tid != "" && !seen[tid] {
						seen[tid] = true
						newTraceIDs = append(newTraceIDs, tid)
					}
				}
				if s.Dash0ForwardLinks != nil {
					for _, link := range *s.Dash0ForwardLinks {
						tid := hex.EncodeToString(link.TraceId)
						if tid != "" && !seen[tid] {
							seen[tid] = true
							newTraceIDs = append(newTraceIDs, tid)
						}
					}
				}
			}
		}
	}
	return newTraceIDs
}
