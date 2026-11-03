package awsmetricscrapeconfigs

import (
	"context"
	"fmt"

	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

type deleteFlags struct {
	connectionFlags
	Force bool
}

func newDeleteCmd() *cobra.Command {
	flags := &deleteFlags{}

	cmd := &cobra.Command{
		Use:     "delete <integration-id>",
		Aliases: []string{"remove"},
		Short:   "[experimental] Clear the scrape config of an AWS integration",
		Long: `Clear the CloudWatch metric scrape config of an AWS integration. Dash0 stops
scraping CloudWatch metrics for the integration. The integration itself is
kept. Use --force to skip the confirmation prompt.` + internal.CONFIG_HINT,
		Example: `  # Clear with confirmation prompt
  dash0 --experimental aws-metric-scrape-configs delete <integration-id>

  # Clear without confirmation (for scripts and automation)
  dash0 --experimental aws-metric-scrape-configs delete <integration-id> --force`,
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

	return cmd
}

func runDelete(ctx context.Context, integrationID string, flags *deleteFlags) error {
	confirmed, err := confirmation.ConfirmDestructiveOperation(
		ctx,
		fmt.Sprintf("Are you sure you want to clear the AWS metric scrape config of integration %q? Dash0 stops scraping its CloudWatch metrics. [y/N]: ", integrationID),
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

	if err := apiClient.DeleteAwsMetricScrapeConfig(ctx, integrationID); err != nil {
		// A 404 means the integration is gone, and its config with it.
		ectx := errorContext(integrationID)
		if client.IsAlreadyDeleted(err, flags.Force, ectx) {
			return nil
		}
		return client.HandleAPIError(err, ectx)
	}

	fmt.Printf("%s of integration %q cleared\n", displayKind, integrationID)
	return nil
}
