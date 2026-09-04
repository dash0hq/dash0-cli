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
		Use:   "update [origin-or-id] -f <file>",
		Short: "Update a time series aggregation from a file",
		Long: `Update an existing time series aggregation from a YAML or JSON definition
file. Use '-f -' to read from stdin.

When the positional argument is omitted, the target is taken from the
document's 'dash0.com/origin' label. When both are given, the argument must
match the document's origin or its id.

The output is a unified diff of the before and after states.` + internal.CONFIG_HINT,
		Example: `  # Update using the origin from the file
  dash0 tsa update -f aggregation.yaml

  # Update by explicit origin or id
  dash0 tsa update <origin-or-id> -f aggregation.yaml

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

	key, err := resolveUpdateKey(args, origin(aggregation), dash0api.GetTimeSeriesAggregationID(aggregation))
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

// resolveUpdateKey returns the value to pass as the originOrId URL parameter.
//
// Origin wins over id, matching the import path's upsert key. When the caller
// passes a positional argument it must match one of the two, so a copy-paste
// mistake fails before any API call rather than updating a different
// aggregation than the file describes.
func resolveUpdateKey(args []string, fileOrigin, fileID string) (string, error) {
	if len(args) == 0 {
		if fileOrigin != "" {
			return fileOrigin, nil
		}
		if fileID != "" {
			return fileID, nil
		}
		return "", fmt.Errorf(
			"no time series aggregation origin or id given: pass one as an argument, or set %s in the file",
			asset.TimeSeriesAggregationOriginLabel,
		)
	}

	arg := args[0]
	if fileOrigin == "" && fileID == "" {
		return arg, nil
	}
	if arg == fileOrigin || arg == fileID {
		return arg, nil
	}
	return "", fmt.Errorf(
		"argument %q matches neither the file's origin (%q) nor its id (%q)",
		arg, fileOrigin, fileID,
	)
}
