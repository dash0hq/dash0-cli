package datasets

import (
	"fmt"

	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

type deleteFlags struct {
	ApiUrl    string
	AuthToken string
	Force     bool
}

func newDeleteCmd() *cobra.Command {
	flags := &deleteFlags{}

	cmd := &cobra.Command{
		Use:     "delete <id>",
		Aliases: []string{"remove"},
		Short:   "[experimental] Delete a dataset",
		Long: `Delete a dataset by its ID (slug) or origin. Use --force to skip the confirmation prompt.

Dash0 keeps a deleted dataset for 30 days.
Creating a dataset with the same slug in that time restores it.
After that, the dataset and its telemetry are permanently purged.
The default dataset cannot be deleted.` + internal.CONFIG_HINT,
		Example: `  # Delete with confirmation prompt
  dash0 --experimental datasets delete <id>

  # Delete without confirmation (for scripts and automation)
  dash0 --experimental datasets delete <id> --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runDelete(cmd, args[0], flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().BoolVar(&flags.Force, "force", false, "Skip confirmation prompt")

	return cmd
}

func runDelete(cmd *cobra.Command, originOrID string, flags *deleteFlags) error {
	ctx := cmd.Context()

	confirmed, err := confirmation.ConfirmDestructiveOperation(
		ctx,
		fmt.Sprintf("Are you sure you want to delete dataset %q? Dash0 keeps it for 30 days, so creating a dataset with the same slug restores it. After that, the dataset and its telemetry are permanently purged. [y/N]: ", originOrID),
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

	if err := apiClient.DeleteDataset(ctx, originOrID); err != nil {
		ectx := client.ErrorContext{AssetType: "dataset", AssetID: originOrID}
		if client.IsAlreadyDeleted(err, flags.Force, ectx) {
			return nil
		}
		return client.HandleAPIError(err, ectx)
	}

	fmt.Printf("Dataset %q deleted\n", originOrID)
	return nil
}
