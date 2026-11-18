package awsloggrouppolicies

import (
	"context"
	"fmt"
	"os"

	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

type updateFlags struct {
	connectionFlags
	File   string
	DryRun bool
}

func newUpdateCmd() *cobra.Command {
	flags := &updateFlags{}

	cmd := &cobra.Command{
		Use:   "update [integration-id] -f <file>",
		Short: "[experimental] Create or replace the log-group policy of an AWS integration from a file",
		Long: `Create or replace the log-group instrumentation policy of an AWS integration
with the policy in a YAML or JSON file, and recreate a deleted one. Use '-f -'
to read from stdin.

The whole policy is replaced, and the file wins over any change made since it
was exported: the CLI does not send the dash0.com/version revision.

When the positional argument is omitted, the integration id is taken from the
document's 'dash0.com/id' label. When it is given, it must match that label.

The output is a unified diff of the before and after states.` + internal.CONFIG_HINT,
		Example: `  # Replace the policy, taking the integration id from the file
  dash0 --experimental aws-log-group-policies update -f policy.yaml

  # Replace the policy of an explicit integration
  dash0 --experimental aws-log-group-policies update <integration-id> -f policy.yaml

  # Preview the diff without applying
  dash0 --experimental aws-log-group-policies update -f policy.yaml --dry-run`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runUpdate(cmd.Context(), args, flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Path to YAML or JSON definition file (use '-' for stdin)")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Show the diff without applying it")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *updateFlags) error {
	raw, err := asset.ReadRawInput(flags.File, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read AWS log-group policy definition: %w", err)
	}

	policy, warnings, err := asset.ParseAwsLogGroupPolicy(raw)
	if err != nil {
		return err
	}

	integrationID, err := asset.ResolveUpdateKey(args, asset.UpdateKey{
		Noun:   "AWS integration",
		Upsert: policy,
	})
	if err != nil {
		return err
	}
	printWarnings(warnings)

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	if flags.DryRun {
		plan, err := asset.PlanAwsLogGroupPolicy(ctx, apiClient, policy, integrationID)
		if err != nil {
			return client.HandleAPIError(err, errorContext(integrationID))
		}
		if plan.Before == nil {
			fmt.Printf("%s of integration %q would be created\n", displayKind, integrationID)
			return nil
		}
		return asset.PrintDiff(os.Stdout, displayKind, integrationID, plan.Before, policy)
	}

	result, err := asset.ImportAwsLogGroupPolicy(ctx, apiClient, policy, integrationID)
	if err != nil {
		return client.HandleAPIError(err, errorContext(integrationID))
	}
	if result.Before == nil {
		fmt.Printf("%s of integration %q %s\n", displayKind, integrationID, result.Action)
		return nil
	}
	return asset.PrintDiff(os.Stdout, displayKind, integrationID, result.Before, result.After)
}
