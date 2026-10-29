package awsmetricscrapeconfigs

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
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
		Short: "[experimental] Replace the scrape config of an AWS integration from a file",
		Long: `Replace the CloudWatch metric scrape config of an AWS integration with the
config in a YAML or JSON file. Use '-f -' to read from stdin.

The whole config is replaced: a namespace missing from spec.namespaces is no
longer scraped, and 'spec.namespaces: []' stops all scraping.

When the positional argument is omitted, the integration id is taken from the
document's 'dash0.com/id' label. When it is given, it must match that label.

The output is a unified diff of the before and after states.` + internal.CONFIG_HINT,
		Example: `  # Replace the config, taking the integration id from the file
  dash0 --experimental aws-metric-scrape-configs update -f scrape-config.yaml

  # Replace the config of an explicit integration
  dash0 --experimental aws-metric-scrape-configs update <integration-id> -f scrape-config.yaml

  # Preview the diff without applying
  dash0 --experimental aws-metric-scrape-configs update -f scrape-config.yaml --dry-run`,
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
		return fmt.Errorf("failed to read AWS metric scrape config definition: %w", err)
	}

	cfg, warnings, err := asset.ParseAwsMetricScrapeConfig(raw)
	if err != nil {
		return err
	}

	integrationID, err := asset.ResolveUpdateKey(args, asset.UpdateKey{
		Noun: "AWS integration",
		ID:   dash0api.GetAwsMetricScrapeConfigID(cfg),
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
		before, err := apiClient.GetAwsMetricScrapeConfig(ctx, integrationID)
		if err != nil {
			return client.HandleAPIError(err, errorContext(integrationID))
		}
		if err := requireConfig(before, integrationID); err != nil {
			return err
		}
		return asset.PrintDiff(os.Stdout, displayKind, dash0api.GetAwsMetricScrapeConfigName(before), before, cfg)
	}

	result, err := asset.ImportAwsMetricScrapeConfig(ctx, apiClient, cfg, integrationID)
	if err != nil {
		return client.HandleAPIError(err, errorContext(integrationID))
	}
	if result.Before == nil {
		fmt.Printf("%s %q %s (integration id: %s)\n", displayKind, result.Name, result.Action, integrationID)
		return nil
	}
	return asset.PrintDiff(os.Stdout, displayKind, result.Name, result.Before, result.After)
}
