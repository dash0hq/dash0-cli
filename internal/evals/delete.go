package evals

import (
	"context"
	"fmt"

	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	var flags asset.DeleteFlags

	cmd := &cobra.Command{
		Use:     "delete <id-or-origin>",
		Aliases: []string{"remove"},
		Short:   "[experimental] Delete an eval",
		Long: `Delete a generative AI evaluation (eval) by its ID or its dash0.com/origin label. Use --force to skip the confirmation prompt.
Applying the same origin again restores a deleted eval.` + internal.CONFIG_HINT,
		Example: `  # Delete with confirmation prompt
  dash0 --experimental evals delete <id-or-origin>

  # Delete without confirmation (for scripts and automation)
  dash0 --experimental evals delete <id-or-origin> --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runDelete(cmd.Context(), args[0], &flags)
		},
	}

	asset.RegisterDeleteFlags(cmd, &flags)
	return cmd
}

func runDelete(ctx context.Context, id string, flags *asset.DeleteFlags) error {
	confirmed, err := confirmation.ConfirmDestructiveOperation(
		ctx,
		fmt.Sprintf("Are you sure you want to delete eval %q? [y/N]: ", id),
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

	err = apiClient.DeleteGenerativeAiEvaluation(ctx, id, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		ectx := client.ErrorContext{AssetType: assetType, AssetID: id}
		if client.IsAlreadyDeleted(err, flags.Force, ectx) {
			return nil
		}
		return client.HandleAPIError(err, ectx)
	}

	fmt.Printf("%s %q deleted\n", assetType, id)
	return nil
}
