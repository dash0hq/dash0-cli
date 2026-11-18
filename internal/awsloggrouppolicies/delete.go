package awsloggrouppolicies

import (
	"context"
	"fmt"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

type deleteFlags struct {
	connectionFlags
	Force            bool
	RemoveForwarding bool
}

func newDeleteCmd() *cobra.Command {
	flags := &deleteFlags{}

	cmd := &cobra.Command{
		Use:     "delete <integration-id>",
		Aliases: []string{"remove"},
		Short:   "[experimental] Delete the log-group policy of an AWS integration",
		Long: `Delete the log-group instrumentation policy of an AWS integration. The
integration itself is kept.

By default the log groups the policy forwards keep forwarding. With
--remove-forwarding, Dash0 removes their subscription filters instead. Log
groups selected by hand keep forwarding either way.

Use --force to skip the confirmation prompt.` + internal.CONFIG_HINT,
		Example: `  # Delete, keep forwarding the matched log groups
  dash0 --experimental aws-log-group-policies delete <integration-id>

  # Delete and stop forwarding the matched log groups
  dash0 --experimental aws-log-group-policies delete <integration-id> --remove-forwarding

  # Delete without confirmation (for scripts and automation)
  dash0 --experimental aws-log-group-policies delete <integration-id> --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runDelete(cmd.Context(), args[0], flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().BoolVar(&flags.Force, "force", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&flags.RemoveForwarding, "remove-forwarding", false, "Also remove the subscription filters of the log groups the policy forwards")

	return cmd
}

func runDelete(ctx context.Context, integrationID string, flags *deleteFlags) error {
	disposition := dash0api.AwsLogGroupPolicyDispositionKeep
	prompt := fmt.Sprintf("Are you sure you want to delete the AWS log-group policy of integration %q? Its log groups keep forwarding. [y/N]: ", integrationID)
	if flags.RemoveForwarding {
		disposition = dash0api.AwsLogGroupPolicyDispositionRemove
		prompt = fmt.Sprintf("Are you sure you want to delete the AWS log-group policy of integration %q and remove forwarding from every log group it matched? [y/N]: ", integrationID)
	}
	confirmed, err := confirmation.ConfirmDestructiveOperation(ctx, prompt, flags.Force)
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

	if _, err := apiClient.DeleteAwsLogGroupInstrumentationPolicy(ctx, integrationID, disposition, nil); err != nil {
		ectx := errorContext(integrationID)
		if client.IsAlreadyDeleted(err, flags.Force, ectx) {
			return nil
		}
		return client.HandleAPIError(err, ectx)
	}

	if flags.RemoveForwarding {
		fmt.Printf("%s of integration %q deleted; forwarding is being removed from its log groups\n", displayKind, integrationID)
	} else {
		fmt.Printf("%s of integration %q deleted; its log groups keep forwarding\n", displayKind, integrationID)
	}
	return nil
}
