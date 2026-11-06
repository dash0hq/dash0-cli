package telemetrytransformationrules

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

func newListCmd() *cobra.Command {
	var flags asset.ListFlags
	cmd := &cobra.Command{
		Use: "list", Aliases: []string{"ls"},
		Short: "List telemetry transformation rules",
		Long:  "List rules in a dataset without their stored examples.\nUse get before editing or backing up a complete rule." + internal.CONFIG_HINT,
		Example: `  # List every rule in a dataset
  dash0 telemetry-transformation-rules list --dataset production --all

  # Print summaries as JSON
  dash0 telemetry-transformation-rules list -o json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runList(cmd.Context(), &flags) },
	}
	asset.RegisterListFlags(cmd, &flags)
	return cmd
}

func runList(ctx context.Context, flags *asset.ListFlags) error {
	if err := output.ValidateSkipHeader(flags.SkipHeader, flags.Output); err != nil {
		return err
	}
	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}
	items, err := apiClient.ListTelemetryTransformationRules(ctx, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType})
	}
	if !flags.All && flags.Limit > 0 && len(items) > flags.Limit {
		items = items[:flags.Limit]
	}
	data := make([]interface{}, len(items))
	for i, rule := range items {
		data[i] = rule
	}
	formatter := output.NewFormatter(format, os.Stdout, output.WithSkipHeader(flags.SkipHeader))
	switch format {
	case output.FormatJSON:
		return formatter.PrintJSON(data)
	case output.FormatYAML:
		return formatter.PrintMultiDocYAML(data)
	}
	if len(items) == 0 {
		fmt.Println("No telemetry transformation rules found.")
		return nil
	}
	columns := []output.Column{
		{Header: internal.HEADER_NAME, Width: 40, Value: func(item interface{}) string {
			return dash0api.GetTelemetryTransformationRuleName(item.(*dash0api.TelemetryTransformationRuleDefinition))
		}},
		{Header: internal.HEADER_ID, Width: 36, Value: func(item interface{}) string {
			return dash0api.GetTelemetryTransformationRuleID(item.(*dash0api.TelemetryTransformationRuleDefinition))
		}},
		{Header: "SIGNAL", Width: 10, Value: func(item interface{}) string {
			return string(item.(*dash0api.TelemetryTransformationRuleDefinition).Spec.Signal)
		}},
		{Header: internal.HEADER_ENABLED, Width: 8, Value: func(item interface{}) string {
			return strconv.FormatBool(item.(*dash0api.TelemetryTransformationRuleDefinition).Spec.Enabled)
		}},
	}
	if format == output.FormatWide || format == output.FormatCSV {
		columns = append(columns,
			output.Column{Header: internal.HEADER_DATASET, Width: 15, Value: func(item interface{}) string {
				return dash0api.GetTelemetryTransformationRuleDataset(item.(*dash0api.TelemetryTransformationRuleDefinition))
			}},
			output.Column{Header: internal.HEADER_ORIGIN, Width: 30, Value: func(item interface{}) string {
				return asset.GetTelemetryTransformationRuleOrigin(item.(*dash0api.TelemetryTransformationRuleDefinition))
			}},
		)
	}
	if format == output.FormatCSV {
		return formatter.PrintCSV(columns, data)
	}
	return formatter.PrintTable(columns, data)
}
