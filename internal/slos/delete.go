package slos

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
		Use:     "delete <id-or-origin>",
		Aliases: []string{"remove"},
		Short:   "Delete an SLO",
		Long:    `Delete an SLO by its ID or its dash0.com/origin label. Use --force to skip the confirmation prompt.` + internal.CONFIG_HINT,
		Example: `  # Delete with confirmation prompt
  dash0 slos delete <id-or-origin>

  # Delete without confirmation (for scripts and automation)
  dash0 slos delete <id-or-origin> --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd.Context(), args[0], &flags)
		},
	}

	asset.RegisterDeleteFlags(cmd, &flags)
	return cmd
}

func runDelete(ctx context.Context, id string, flags *asset.DeleteFlags) error {
	confirmed, err := confirmation.ConfirmDestructiveOperation(
		ctx,
		fmt.Sprintf("Are you sure you want to delete SLO %q? [y/N]: ", id),
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

	err = apiClient.DeleteSLO(ctx, id, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		ectx := client.ErrorContext{AssetType: "SLO", AssetID: id}
		if client.IsAlreadyDeleted(err, flags.Force, ectx) {
			return nil
		}
		return client.HandleAPIError(err, ectx)
	}

	fmt.Printf("SLO %q deleted\n", id)
	return nil
}
