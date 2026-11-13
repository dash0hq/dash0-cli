package samplingrules

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
		Short:   "Delete a sampling rule",
		Long: `Delete a sampling rule by its 'dash0.com/origin' label or its ID. Use --force
to skip the confirmation prompt.

The rule is deleted for good: applying the same document again creates a new
rule with a new ID. Deleting the last enabled rule in a dataset turns sampling
off, so Dash0 keeps every trace again.` + internal.CONFIG_HINT,
		Example: `  # Delete with confirmation prompt
  dash0 sampling-rules delete <origin-or-id>

  # Delete without confirmation (for scripts and automation)
  dash0 sampling-rules delete <origin-or-id> --force`,
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
		fmt.Sprintf("Are you sure you want to delete sampling rule %q? [y/N]: ", originOrID),
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
	if err := apiClient.DeleteSamplingRule(ctx, originOrID, client.ResolveDataset(ctx, flags.Dataset)); err != nil {
		// Not "already gone": the rule exists in another dataset.
		if asset.IsSamplingRuleWrongDataset(err) {
			return asset.WrapSamplingRuleWrongDataset(err, originOrID)
		}
		if client.IsAlreadyDeleted(err, flags.Force, ectx) {
			return nil
		}
		return client.HandleAPIError(err, ectx)
	}

	fmt.Printf("%s %q deleted\n", displayKind, originOrID)
	return nil
}
