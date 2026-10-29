package authtokens

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

const deleteNote = "Anything that uses the token loses access at once, and the deletion cannot be undone."

func newDeleteCmd() *cobra.Command {
	flags := &deleteFlags{}

	cmd := &cobra.Command{
		Use:     "delete <id>",
		Aliases: []string{"remove"},
		Short:   "[experimental] Delete an auth token",
		Long: `Delete an auth token by its ID or origin. Use --force to skip the confirmation prompt.

` + deleteNote + `
The last auth token of an organization cannot be deleted.` + internal.CONFIG_HINT,
		Example: `  # Delete with confirmation prompt
  dash0 --experimental auth-tokens delete <id>

  # Delete without confirmation (for scripts and automation)
  dash0 --experimental auth-tokens delete <id> --force`,
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

	if err := confirmation.RequireForceInAgentMode(flags.Force, "dash0 auth-tokens delete", fmt.Sprintf(
		"re-run with `dash0 -X auth-tokens delete %s --force` to confirm. %s", originOrID, deleteNote,
	)); err != nil {
		return err
	}

	confirmed, err := confirmation.ConfirmDestructiveOperation(
		ctx,
		fmt.Sprintf("Are you sure you want to delete auth token %q? %s [y/N]: ", originOrID, deleteNote),
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

	if err := apiClient.DeleteAuthToken(ctx, originOrID); err != nil {
		ectx := client.ErrorContext{AssetType: assetType, AssetID: originOrID}
		if client.IsAlreadyDeleted(err, flags.Force, ectx) {
			return nil
		}
		return client.HandleAPIError(err, ectx)
	}

	fmt.Printf("Auth token %q deleted\n", originOrID)
	return nil
}
