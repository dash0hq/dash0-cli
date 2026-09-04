package timeseriesaggregations

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	var flags asset.GetFlags

	cmd := &cobra.Command{
		Use:   "get <origin-or-id>",
		Short: "Get a time series aggregation by origin or ID",
		Long: `Retrieve a time series aggregation definition by its 'dash0.com/origin' label
or its ID.` + internal.CONFIG_HINT,
		Example: `  # Show the aggregation summary
  dash0 tsa get <origin-or-id>

  # Export as YAML (suitable for re-applying)
  dash0 tsa get <origin-or-id> -o yaml > aggregation.yaml

  # Export as JSON
  dash0 tsa get <origin-or-id> -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd.Context(), args[0], &flags)
		},
	}

	asset.RegisterGetFlags(cmd, &flags)
	return cmd
}

func runGet(ctx context.Context, originOrID string, flags *asset.GetFlags) error {
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	aggregation, err := apiClient.GetTimeSeriesAggregation(ctx, originOrID, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		if asset.IsTimeSeriesAggregationWrongDataset(err) {
			return asset.WrapTimeSeriesAggregationWrongDataset(err, originOrID)
		}
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: assetType,
			AssetID:   originOrID,
		})
	}

	dash0api.SetTimeSeriesAggregationIDIfAbsent(aggregation, originOrID)

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout)

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return formatter.Print(aggregation)
	default:
		printSummary(aggregation)
		return nil
	}
}

func printSummary(aggregation *dash0api.TimeSeriesAggregationDefinition) {
	fmt.Printf("Kind: %s\n", displayKind)
	fmt.Printf("Name: %s\n", dash0api.GetTimeSeriesAggregationName(aggregation))
	fmt.Printf("ID: %s\n", dash0api.GetTimeSeriesAggregationID(aggregation))
	fmt.Printf("Dataset: %s\n", dash0api.GetTimeSeriesAggregationDataset(aggregation))
	fmt.Printf("Origin: %s\n", origin(aggregation))
	fmt.Printf("Interval: %s\n", interval(aggregation))
	fmt.Printf("Enabled: %t\n", aggregation.Spec.Enabled)
	if aggregation.Spec.Sample.Delay != nil {
		fmt.Printf("Delay: %s\n", string(*aggregation.Spec.Sample.Delay))
	}
	if aggregation.Spec.Sample.StaleAfter != nil {
		fmt.Printf("Stale after: %s\n", string(*aggregation.Spec.Sample.StaleAfter))
	}
	if aggregation.Spec.Priority != nil {
		fmt.Printf("Priority: %d\n", *aggregation.Spec.Priority)
	}
	if aggregation.Metadata.Annotations != nil {
		if createdAt := aggregation.Metadata.Annotations.Dash0ComcreatedAt; createdAt != nil {
			fmt.Printf("Created: %s\n", createdAt.Format("2006-01-02 15:04:05"))
		}
		if updatedAt := aggregation.Metadata.Annotations.Dash0ComupdatedAt; updatedAt != nil {
			fmt.Printf("Updated: %s\n", updatedAt.Format("2006-01-02 15:04:05"))
		}
	}
}
