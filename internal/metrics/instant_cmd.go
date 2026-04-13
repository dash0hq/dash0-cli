package metrics

import (
	"fmt"
	"time"

	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/query"
	"github.com/spf13/cobra"
)

type instantFlags struct {
	apiURL     string
	authToken  string
	dataset    string
	output     string
	promql     string
	queryAlias string // deprecated --query alias
	filter     []string
	from       string
	timeAlias  string // deprecated --time alias
	skipHeader bool
	column     []string
}

func newInstantCmd() *cobra.Command {
	flags := &instantFlags{}

	cmd := &cobra.Command{
		Use:   "instant",
		Short: "Run an instant PromQL query",
		Long: `Run an instant PromQL query against the Dash0 API, returning a single` +
			` datapoint per time series.` + internal.CONFIG_HINT,
		Example: `  # Query the current request rate
  dash0 metrics instant --promql 'sum by (service_name) (rate(http_server_request_duration_seconds_count[5m]))'

  # Query with a specific dataset
  dash0 metrics instant --promql 'sum(rate(http_server_request_duration_seconds_count[5m]))' --dataset production

  # Query at a specific time
  dash0 metrics instant --promql 'sum by (service_name) (rate(http_server_request_duration_seconds_count[5m]))' --from 2024-01-25T10:00:00Z

  # Query with filters instead of PromQL
  dash0 metrics instant --filter 'service.name is my-service'

  # Output as CSV without header
  dash0 metrics instant --promql 'sum by (service_name) (rate(http_server_request_duration_seconds_count[5m]))' -o csv --skip-header

  # Select specific columns
  dash0 metrics instant --promql 'sum by (service_name) (rate(http_server_request_duration_seconds_count[5m]))' --column value --column service_name`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInstant(cmd, flags)
		},
	}

	// Primary flags
	cmd.Flags().StringVar(&flags.promql, "promql", "", "PromQL query expression")
	cmd.Flags().StringArrayVar(&flags.filter, "filter", nil, "Filter as 'key [operator] value', translated to PromQL label matchers (repeatable)")
	cmd.Flags().StringVar(&flags.from, "from", "", "Evaluation timestamp (default: now)")
	cmd.Flags().StringVar(&flags.dataset, "dataset", "", "Dataset to query")
	cmd.Flags().StringVar(&flags.apiURL, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.authToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVarP(&flags.output, "output", "o", "", "Output format: table, json, csv (default: table; json in agent mode)")
	cmd.Flags().BoolVar(&flags.skipHeader, "skip-header", false, "Omit the header row from table and CSV output")
	cmd.Flags().StringArrayVar(&flags.column, "column", nil, "Column to display (repeatable; table and CSV only)")

	// Deprecated aliases
	cmd.Flags().StringVar(&flags.queryAlias, "query", "", "PromQL query expression")
	cmd.Flags().StringVar(&flags.timeAlias, "time", "", "Evaluation timestamp")
	_ = cmd.Flags().MarkDeprecated("query", "use --promql instead")
	_ = cmd.Flags().MarkDeprecated("time", "use --from instead")

	return cmd
}

func runInstant(_ *cobra.Command, flags *instantFlags) error {
	// Resolve deprecated aliases.
	if flags.promql == "" && flags.queryAlias != "" {
		flags.promql = flags.queryAlias
	}
	if flags.from == "" && flags.timeAlias != "" {
		flags.from = flags.timeAlias
	}

	// Validate mutually exclusive --promql and --filter.
	if flags.promql != "" && len(flags.filter) > 0 {
		return fmt.Errorf("--promql and --filter are mutually exclusive; use one or the other")
	}
	if flags.promql == "" && len(flags.filter) == 0 {
		return fmt.Errorf("either --promql or --filter must be specified")
	}

	// Parse and validate output format.
	format, err := parseQueryFormat(flags.output)
	if err != nil {
		return err
	}

	// Validate --column with JSON output.
	if err := query.ValidateColumnFormat(flags.column, string(format)); err != nil {
		return err
	}

	// Resolve columns.
	cols, err := resolveMetricColumns(flags.column, format)
	if err != nil {
		return err
	}

	// Resolve configuration.
	cfg, err := profiles.ResolveConfiguration(flags.apiURL, flags.authToken)
	if err != nil {
		return err
	}
	apiURL := cfg.ApiUrl
	authToken := cfg.AuthToken

	dataset := flags.dataset
	if dataset == "" {
		dataset = cfg.Dataset
	}

	// Build PromQL from filters if needed.
	promql := flags.promql
	if len(flags.filter) > 0 {
		filters, err := query.ParseFilters(flags.filter)
		if err != nil {
			return fmt.Errorf("failed to parse filter: %w", err)
		}
		promql, err = filtersToPromQL(filters)
		if err != nil {
			return err
		}
	}

	// Normalize the evaluation timestamp.
	evalTime := query.NormalizeTimestamp(flags.from)

	// Execute the query.
	var datasetPtr *string
	if dataset != "" {
		datasetPtr = &dataset
	}
	response, err := runInstantQuery(apiURL, authToken, promql, evalTime, datasetPtr)
	if err != nil {
		return err
	}

	// Render output.
	customColumns := len(flags.column) > 0

	if !customColumns && format != queryFormatJSON {
		printAvailableColumnsHint(response)
	}

	switch format {
	case queryFormatJSON:
		return renderInstantJSON(response)
	case queryFormatCSV:
		renderInstantCSV(response, cols, flags.skipHeader)
	case queryFormatTable:
		if !customColumns {
			fmt.Println("Query:", promql)
			fmt.Printf("Time: %s\n\n", time.Now().Format(time.RFC3339))
		}
		renderInstantTable(response, cols, flags.skipHeader, customColumns)
	}

	return nil
}
