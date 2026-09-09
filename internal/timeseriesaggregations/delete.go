package timeseriesaggregations

import (
	"context"
	"fmt"

	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	var flags asset.DeleteFlags

	cmd := &cobra.Command{
		Use:     "delete <origin-or-id>",
		Aliases: []string{"remove"},
		Short:   "Delete a time series aggregation",
		Long: `Delete a time series aggregation by its 'dash0.com/origin' label or its ID.
Use --force to skip the confirmation prompt.` + internal.CONFIG_HINT,
		Example: `  # Delete with confirmation prompt
  dash0 tsa delete <origin-or-id>

  # Delete without confirmation (for scripts and automation)
  dash0 tsa delete <origin-or-id> --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd.Context(), args[0], &flags)
		},
	}

	asset.RegisterDeleteFlags(cmd, &flags)
	return cmd
}

func runDelete(ctx context.Context, originOrID string, flags *asset.DeleteFlags) error {
	confirmed, err := confirmation.ConfirmDestructiveOperation(
		ctx,
		fmt.Sprintf("Are you sure you want to delete time series aggregation %q? [y/N]: ", originOrID),
		flags.Force,
	)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Println("Deletion cancelled")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)
	ectx := client.ErrorContext{AssetType: assetType, AssetID: originOrID}

	// The time series aggregation API returns 204 for an aggregation that does
	// not exist, so deleting a typo'd origin reports success. That matches
	// every other asset type in this CLI — the dashboard, check-rule, view,
	// and spam-filter delete endpoints all return 2xx for a missing asset too
	// — so no client-side existence check is done here. IsAlreadyDeleted stays
	// wired per docs/cli-naming-conventions.md so the behavior is right if the
	// API ever starts returning 404.
	if err := apiClient.DeleteTimeSeriesAggregation(ctx, originOrID, dataset); err != nil {
		// A cross-dataset collision means the aggregation exists and belongs
		// to another dataset. It arrives as a 400, so IsAlreadyDeleted would
		// not swallow it anyway, but handling it first replaces a bare
		// "invalid request" with an explanation.
		if asset.IsTimeSeriesAggregationWrongDataset(err) {
			return asset.WrapTimeSeriesAggregationWrongDataset(err, originOrID)
		}
		if client.IsAlreadyDeleted(err, flags.Force, ectx) {
			return nil
		}
		return client.HandleAPIError(err, ectx)
	}

	fmt.Printf("%s %q deleted\n", displayKind, originOrID)
	return nil
}
