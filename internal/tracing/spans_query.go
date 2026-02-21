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
}

// queryFormat represents the output format for span queries.
type queryFormat string

const (
	queryFormatTable    queryFormat = "table"
	queryFormatJSON queryFormat = "json"
	queryFormatCSV      queryFormat = "csv"

	spansAssetType = "spans"
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
  dash0 --experimental spans query -o json --limit 10`,
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

	return cmd
}

func runQuery(cmd *cobra.Command, flags *queryFlags) error {
	ctx := cmd.Context()

	if err := output.ValidateSkipHeader(flags.SkipHeader, flags.Output); err != nil {
		return err
	}

	format, err := parseQueryFormat(flags.Output)
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
		return streamTable(iter, totalLimit, flags.SkipHeader)
	case queryFormatCSV:
		return streamCSV(iter, totalLimit, flags.SkipHeader)
	case queryFormatJSON:
		return collectAndRenderJSON(iter, totalLimit)
	default:
		return fmt.Errorf("unknown format: %s", format)
	}
}

// flatSpanRecord holds a flattened span for table/CSV rendering.
type flatSpanRecord struct {
	timestamp string
	duration  string
	name      string
	status    string
	service   string
	traceID   string
	parentID  string
	spanLinks string
}

// iterateSpans drives the iterator, flattens each ResourceSpans into flat
// records, and calls emit for each one. It stops after totalLimit records
// (0 means unlimited).
func iterateSpans(iter *dash0api.Iter[dash0api.ResourceSpans], totalLimit int64, emit func(flatSpanRecord)) (int64, error) {
	var total int64
	for iter.Next() {
		rs := iter.Current()
		serviceName := extractServiceName(rs.Resource)
		for _, ss := range rs.ScopeSpans {
			for _, s := range ss.Spans {
				var parentID string
				if s.ParentSpanId != nil {
					parentID = hex.EncodeToString(*s.ParentSpanId)
				}
				emit(flatSpanRecord{
					timestamp: query.FormatTimestamp(s.StartTimeUnixNano),
					duration:  FormatDuration(s.StartTimeUnixNano, s.EndTimeUnixNano),
					name:      s.Name,
					status:    SpanStatusString(s.Status.Code),
					service:   serviceName,
					traceID:   hex.EncodeToString(s.TraceId),
					parentID:  parentID,
					spanLinks: FormatSpanLinks(s.Links),
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

func streamTable(iter *dash0api.Iter[dash0api.ResourceSpans], totalLimit int64, skipHeader bool) error {
	headerPrinted := false

	total, err := iterateSpans(iter, totalLimit, func(r flatSpanRecord) {
		if !headerPrinted && !skipHeader {
			fmt.Fprintf(os.Stdout, "%-28s  %-10s  %-40s  %-8s  %-30s  %-16s  %-32s  %s\n",
				"TIMESTAMP", "DURATION", "SPAN NAME", "STATUS", "SERVICE NAME", "PARENT ID", "TRACE ID", "SPAN LINKS")
			headerPrinted = true
		}
		name := output.Truncate(r.name, 40)
		service := output.Truncate(r.service, 30)
		fmt.Fprintf(os.Stdout, "%-28s  %-10s  %-40s  %s  %-30s  %-16s  %-32s  %s\n",
			r.timestamp, r.duration, name, colorpkg.SprintSpanStatus(r.status), service, r.parentID, r.traceID, r.spanLinks)
	})
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: spansAssetType})
	}
	if total == 0 {
		fmt.Println("No spans found.")
	}
	return nil
}

func streamCSV(iter *dash0api.Iter[dash0api.ResourceSpans], totalLimit int64, skipHeader bool) error {
	w := csv.NewWriter(os.Stdout)
	if !skipHeader {
		if err := w.Write([]string{"otel.span.start_time", "otel.span.duration", "otel.span.name", "otel.span.status.code", "service.name", "otel.parent.id", "otel.trace.id", "otel.span.links"}); err != nil {
			return err
		}
		w.Flush()
	}

	_, err := iterateSpans(iter, totalLimit, func(r flatSpanRecord) {
		w.Write([]string{r.timestamp, r.duration, r.name, r.status, r.service, r.parentID, r.traceID, r.spanLinks})
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

// extractServiceName extracts the service name from resource attributes.
// If service.namespace is set, it returns "namespace/name".
func extractServiceName(resource dash0api.Resource) string {
	var name, namespace string
	for _, attr := range resource.Attributes {
		if attr.Key == "service.name" && attr.Value.StringValue != nil {
			name = *attr.Value.StringValue
		}
		if attr.Key == "service.namespace" && attr.Value.StringValue != nil {
			namespace = *attr.Value.StringValue
		}
	}
	if namespace != "" && name != "" {
		return namespace + "/" + name
	}
	return name
}
