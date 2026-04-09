package checkrules

import (
	"context"
	"fmt"
	"os"

	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var flags asset.FileInputFlags

	cmd := &cobra.Command{
		Use:     "create -f <file>",
		Aliases: []string{"add"},
		Short:   "Create a check rule from a file",
		Long: `Create a new check rule from a YAML or JSON definition file. Use '-f -' to read from stdin.

Accepts both plain CheckRule definitions and PrometheusRule CRD files. When a PrometheusRule CRD is provided, each alerting rule in the CRD is created as a separate check rule (recording rules are skipped).` + internal.CONFIG_HINT,
		Example: `  # Create from a YAML file
  dash0 check-rules create -f rule.yaml

  # Create from a PrometheusRule CRD file
  dash0 check-rules create -f prometheus-rules.yaml

  # Create from stdin
  cat rule.yaml | dash0 check-rules create -f -

  # Validate without creating
  dash0 check-rules create -f rule.yaml --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd.Context(), &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runCreate(ctx context.Context, flags *asset.FileInputFlags) error {
	data, err := asset.ReadRawInput(flags.File, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read check rule definition: %w", err)
	}

	rules, err := dash0yaml.ParseAsPrometheusAlertRules(data)
	if err != nil {
		return err
	}

	if flags.DryRun {
		fmt.Println("Dry run: check rule definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)
	for _, rule := range rules {
		result, importErr := asset.ImportCheckRule(ctx, apiClient, rule, dataset)
		if importErr != nil {
			return client.HandleAPIError(importErr, client.ErrorContext{
				AssetType: "check rule",
				AssetName: rule.Name,
			})
		}
		fmt.Printf("Check rule %q %s\n", result.Name, result.Action)
	}
	return nil
}
