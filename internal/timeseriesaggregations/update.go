package timeseriesaggregations

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var flags asset.FileInputFlags

	cmd := &cobra.Command{
		Use:   "update [origin] -f <file>",
		Short: "Update a time series aggregation from a file",
		Long: `Update an existing time series aggregation from a YAML or JSON definition
file. Use '-f -' to read from stdin.

When the positional argument is omitted, the target is taken from the
document's 'dash0.com/origin' label. When it is given, it must match that
label. An id is accepted only when the document also carries that id, which
is the case for a definition exported with 'tsa get -o yaml'; a hand-written
document normally carries only the origin, because ids are server-assigned.

The output is a unified diff of the before and after states.` + internal.CONFIG_HINT,
		Example: `  # Update using the origin from the file
  dash0 tsa update -f aggregation.yaml

  # Update by explicit origin, which must match the file's origin label
  dash0 tsa update <origin> -f aggregation.yaml

  # Preview the diff without applying
  dash0 tsa update -f aggregation.yaml --dry-run

  # Export, edit, and update
  dash0 tsa get <origin-or-id> -o yaml > aggregation.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args, &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *asset.FileInputFlags) error {
	raw, err := asset.ReadRawInput(flags.File, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read time series aggregation definition: %w", err)
	}

	aggregation, err := decode(raw)
	if err != nil {
		return err
	}

	key, err := asset.ResolveUpdateKey(args, asset.UpdateKey{
		Noun:       assetType,
		UsesOrigin: true,
		Origin:     origin(aggregation),
		ID:         dash0api.GetTimeSeriesAggregationID(aggregation),
	})
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	before, err := apiClient.GetTimeSeriesAggregation(ctx, key, dataset)
	if err != nil {
		if asset.IsTimeSeriesAggregationWrongDataset(err) {
			return asset.WrapTimeSeriesAggregationWrongDataset(err, key)
		}
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: assetType,
			AssetID:   key,
		})
	}

	if flags.DryRun {
		return asset.PrintDiff(os.Stdout, displayKind, dash0api.GetTimeSeriesAggregationName(aggregation), before, aggregation)
	}

	// The origin label is stripped from the outbound body, matching the import
	// path: the server takes the origin from the URL and ignores the body's id.
	dash0api.StripTimeSeriesAggregationServerFields(aggregation)

	result, err := apiClient.UpdateTimeSeriesAggregation(ctx, key, aggregation, dataset)
	if err != nil {
		if asset.IsTimeSeriesAggregationWrongDataset(err) {
			return asset.WrapTimeSeriesAggregationWrongDataset(err, key)
		}
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: assetType,
			AssetID:   key,
			AssetName: dash0api.GetTimeSeriesAggregationName(aggregation),
		})
	}

	return asset.PrintDiff(os.Stdout, displayKind, dash0api.GetTimeSeriesAggregationName(result), before, result)
}
