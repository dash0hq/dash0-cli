package signaltometricsrules

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
		Short:   "Delete a signal-to-metrics rule",
		Long: `Delete a signal-to-metrics rule by its 'dash0.com/origin' label or its ID.
Use --force to skip the confirmation prompt.` + internal.CONFIG_HINT,
		Example: `  # Delete with confirmation prompt
  dash0 s2m delete <origin-or-id>

  # Delete without confirmation (for scripts and automation)
  dash0 s2m delete <origin-or-id> --force`,
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
		fmt.Sprintf("Are you sure you want to delete signal-to-metrics rule %q? [y/N]: ", originOrID),
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

	ectx := client.ErrorContext{AssetType: assetType, AssetID: originOrID}
	if err := apiClient.DeleteSignalToMetrics(ctx, originOrID, client.ResolveDataset(ctx, flags.Dataset)); err != nil {
		if client.IsAlreadyDeleted(err, flags.Force, ectx) {
			return nil
		}
		if asset.IsSignalToMetricsRuleWrongDataset(err) {
			return asset.WrapSignalToMetricsRuleWrongDataset(err, originOrID)
		}
		return client.HandleAPIError(err, ectx)
	}

	fmt.Printf("%s %q deleted\n", displayKind, originOrID)
	return nil
}
