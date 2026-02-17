package logs

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/dash0hq/dash0-cli/internal/query"
	"github.com/spf13/cobra"
)

type queryFlags struct {
	ApiUrl    string
	AuthToken string
	Dataset   string
	Output    string
	From      string
	To        string
	Filter    []string
	Limit     int
}

// queryFormat represents the output format for log queries.
type queryFormat string

const (
	queryFormatTable    queryFormat = "table"
	queryFormatOtlpJSON queryFormat = "otlp-json"
	queryFormatCSV      queryFormat = "csv"

	logRecordsAssetType = "log records"
)

func parseQueryFormat(s string) (queryFormat, error) {
	switch strings.ToLower(s) {
	case "table", "":
		return queryFormatTable, nil
	case "otlp-json":
		return queryFormatOtlpJSON, nil
	case "csv":
		return queryFormatCSV, nil
	default:
		return "", fmt.Errorf("unknown output format: %s (valid formats: table, otlp-json, csv)", s)
	}
}

func newQueryCmd() *cobra.Command {
	flags := &queryFlags{}

	cmd := &cobra.Command{
		Use:   "query",
		Short: "[experimental] Query log records from Dash0",
		Long:  `Query log records from Dash0 and display them in various formats.`,
		Args:  cobra.NoArgs,
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
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table, otlp-json, csv (default: table)")
	cmd.Flags().StringVar(&flags.From, "from", "now-15m", "Start of time range (e.g. now-1h, 2024-01-25T10:00:00.000Z)")
	cmd.Flags().StringVar(&flags.To, "to", "now", "End of time range (e.g. now, 2024-01-25T11:00:00.000Z)")
	cmd.Flags().StringArrayVar(&flags.Filter, "filter", nil, "Filter expression as 'key [operator] value' (repeatable)")
	cmd.Flags().IntVar(&flags.Limit, "limit", 50, "Maximum number of log records to return")

	return cmd
}

func runQuery(cmd *cobra.Command, flags *queryFlags) error {
	ctx := cmd.Context()

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

	const otlpJSONMaxLimit int64 = 100
	if format == queryFormatOtlpJSON && totalLimit > otlpJSONMaxLimit {
		return fmt.Errorf("otlp-json output is limited to %d records; use --limit %d or lower, or choose a different output format", otlpJSONMaxLimit, otlpJSONMaxLimit)
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
		return streamTable(iter, totalLimit)
	case queryFormatCSV:
		return streamCSV(iter, totalLimit)
	case queryFormatOtlpJSON:
		return collectAndRenderOtlpJSON(iter, totalLimit)
	default:
		return fmt.Errorf("unknown format: %s", format)
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
			for _, lr := range sl.LogRecords {
				emit(flatRecord{
					timestamp: formatTimestamp(lr.TimeUnixNano),
					severity:  severityName(lr.SeverityNumber, lr.SeverityText),
					body:      extractBodyString(lr.Body),
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

// flatRecord holds a flattened log record for table/CSV rendering.
type flatRecord struct {
	timestamp string
	severity  string
	body      string
}

func streamTable(iter *dash0api.Iter[dash0api.ResourceLogs], totalLimit int64) error {
	headerPrinted := false

	total, err := iterateRecords(iter, totalLimit, func(r flatRecord) {
		if !headerPrinted {
			fmt.Fprintf(os.Stdout, "%-28s  %-10s  %s\n", "TIMESTAMP", "SEVERITY", "BODY")
			headerPrinted = true
		}
		fmt.Fprintf(os.Stdout, "%-28s  %-10s  %s\n", r.timestamp, r.severity, r.body)
	})
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: logRecordsAssetType})
	}
	if total == 0 {
		fmt.Println("No log records found.")
	}
	return nil
}

func streamCSV(iter *dash0api.Iter[dash0api.ResourceLogs], totalLimit int64) error {
	w := csv.NewWriter(os.Stdout)
	if err := w.Write([]string{"timestamp", "severity", "body"}); err != nil {
		return err
	}
	w.Flush()

	_, err := iterateRecords(iter, totalLimit, func(r flatRecord) {
		w.Write([]string{r.timestamp, r.severity, r.body})
		w.Flush()
	})
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: logRecordsAssetType})
	}
	return nil
}

func collectAndRenderOtlpJSON(iter *dash0api.Iter[dash0api.ResourceLogs], totalLimit int64) error {
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

// formatTimestamp converts a nanosecond Unix timestamp string to a human-readable format.
func formatTimestamp(nanoStr string) string {
	nanos, err := strconv.ParseInt(nanoStr, 10, 64)
	if err != nil {
		return nanoStr
	}
	t := time.Unix(0, nanos).UTC()
	return t.Format("2006-01-02T15:04:05.000Z")
}

// severityName returns a human-readable severity name.
func severityName(num *int32, text *string) string {
	if text != nil && *text != "" {
		return *text
	}
	if num != nil {
		return severityNumberToName(*num)
	}
	return ""
}

// severityNumberToName maps OTel severity numbers to names.
func severityNumberToName(n int32) string {
	switch {
	case n == 0:
		return "UNSPECIFIED"
	case n >= 1 && n <= 4:
		return "TRACE"
	case n >= 5 && n <= 8:
		return "DEBUG"
	case n >= 9 && n <= 12:
		return "INFO"
	case n >= 13 && n <= 16:
		return "WARN"
	case n >= 17 && n <= 20:
		return "ERROR"
	case n >= 21 && n <= 24:
		return "FATAL"
	default:
		return fmt.Sprintf("SEVERITY_%d", n)
	}
}
