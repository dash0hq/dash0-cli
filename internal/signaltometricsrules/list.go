package signaltometricsrules

import (
	"context"
	"fmt"
	"os"
	"strconv"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/spf13/cobra"
)

type listFlags struct {
	asset.ListFlags
	OriginPrefix string
	Name         string
	Signal       string
	Enabled      bool
	// EnabledSet records whether --enabled was passed at all, since the flag
	// filters in both directions and its absence means "do not filter".
	EnabledSet bool
}

func newListCmd() *cobra.Command {
	var flags listFlags

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List signal-to-metrics rules",
		Long: `List the signal-to-metrics rules in the specified dataset.

The --origin-prefix, --name, --signal, and --enabled filters are applied by the
Dash0 API. --name matches the display name and the output metric name as a
case-insensitive substring, and the rule ID exactly.` + internal.CONFIG_HINT,
		Example: `  # List signal-to-metrics rules (default: up to 50)
  dash0 s2m list

  # List only the rules that turn spans into metrics
  dash0 s2m list --signal spans

  # List the disabled rules whose name or metric contains "checkout"
  dash0 s2m list --name checkout --enabled=false

  # List the rules managed by the Dash0 CLI
  dash0 s2m list --origin-prefix dash0-cli

  # Output as YAML for backup or version control
  dash0 s2m list -o yaml > rules.yaml

  # Output as CSV (pipe-friendly)
  dash0 s2m list -o csv`,
		// A boolean flag takes its value only after "=", so "--enabled false"
		// would otherwise filter for enabled rules and silently drop "false".
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			flags.EnabledSet = cmd.Flags().Changed("enabled")
			return runList(cmd.Context(), &flags)
		},
	}

	asset.RegisterListFlags(cmd, &flags.ListFlags)
	cmd.Flags().StringVar(&flags.OriginPrefix, "origin-prefix", "", "Only list rules whose origin starts with this prefix")
	cmd.Flags().StringVar(&flags.Name, "name", "", "Only list rules whose display name or metric name contains this text, or whose ID equals it")
	cmd.Flags().StringVar(&flags.Signal, "signal", "", "Only list rules for this signal: logs or spans")
	cmd.Flags().BoolVar(&flags.Enabled, "enabled", false, "Only list rules that are enabled (--enabled) or disabled (--enabled=false)")
	return cmd
}

// listOptions turns the filter flags into API list options.
func listOptions(flags *listFlags) ([]dash0api.ListOption, error) {
	var opts []dash0api.ListOption
	if flags.OriginPrefix != "" {
		opts = append(opts, dash0api.WithOriginPrefix(flags.OriginPrefix))
	}
	if flags.Name != "" {
		opts = append(opts, dash0api.WithSignalToMetricsName(flags.Name))
	}
	if flags.Signal != "" {
		s := dash0api.SignalToMetricsSignalType(flags.Signal)
		switch s {
		case dash0api.SignalToMetricsSignalTypeLogs, dash0api.SignalToMetricsSignalTypeSpans:
			opts = append(opts, dash0api.WithSignal(s))
		default:
			return nil, fmt.Errorf("invalid --signal %q: must be logs or spans", flags.Signal)
		}
	}
	if flags.EnabledSet {
		opts = append(opts, dash0api.WithEnabled(flags.Enabled))
	}
	return opts, nil
}

func runList(ctx context.Context, flags *listFlags) error {
	if err := output.ValidateSkipHeader(flags.SkipHeader, flags.Output); err != nil {
		return err
	}

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	opts, err := listOptions(flags)
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	items, err := apiClient.ListSignalToMetrics(ctx, client.ResolveDataset(ctx, flags.Dataset), opts...)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType})
	}

	if !flags.All && flags.Limit > 0 && len(items) > flags.Limit {
		items = items[:flags.Limit]
	}

	formatter := output.NewFormatter(format, os.Stdout, output.WithSkipHeader(flags.SkipHeader))

	switch format {
	case output.FormatJSON:
		return formatter.PrintJSON(toInterfaceSlice(items))
	case output.FormatYAML:
		return formatter.PrintMultiDocYAML(toInterfaceSlice(items))
	default:
		return printTable(formatter, items, format)
	}
}

func toInterfaceSlice(items []*dash0api.SignalToMetricsDefinition) []interface{} {
	result := make([]interface{}, len(items))
	for i, r := range items {
		result[i] = r
	}
	return result
}

// printTable renders the table, wide, and CSV formats.
//
// There is no URL column, because the API client ships no deeplink helper for
// this kind. Time series aggregations and spam filters are the same.
func printTable(f *output.Formatter, items []*dash0api.SignalToMetricsDefinition, format output.Format) error {
	columns := []output.Column{
		{Header: internal.HEADER_NAME, Width: 40, Value: func(item interface{}) string {
			return dash0api.GetSignalToMetricsName(item.(*dash0api.SignalToMetricsDefinition))
		}},
		{Header: internal.HEADER_ID, Width: 44, Value: func(item interface{}) string {
			return dash0api.GetSignalToMetricsID(item.(*dash0api.SignalToMetricsDefinition))
		}},
		{Header: internal.HEADER_SIGNAL, Width: 6, Value: func(item interface{}) string {
			return signal(item.(*dash0api.SignalToMetricsDefinition))
		}},
		{Header: internal.HEADER_METRIC, Width: 40, Value: func(item interface{}) string {
			return metricName(item.(*dash0api.SignalToMetricsDefinition))
		}},
		{Header: internal.HEADER_ENABLED, Width: 8, Value: func(item interface{}) string {
			return strconv.FormatBool(item.(*dash0api.SignalToMetricsDefinition).Spec.Enabled)
		}},
	}

	if format == output.FormatWide || format == output.FormatCSV {
		columns = append(columns,
			output.Column{Header: internal.HEADER_DATASET, Width: 15, Value: func(item interface{}) string {
				return dash0api.GetSignalToMetricsDataset(item.(*dash0api.SignalToMetricsDefinition))
			}},
			output.Column{Header: internal.HEADER_ORIGIN, Width: 30, Value: func(item interface{}) string {
				return origin(item.(*dash0api.SignalToMetricsDefinition))
			}},
		)
	}

	if len(items) == 0 {
		fmt.Println("No signal-to-metrics rules found.")
		return nil
	}

	data := toInterfaceSlice(items)
	if format == output.FormatCSV {
		return f.PrintCSV(columns, data)
	}
	return f.PrintTable(columns, data)
}
