package checkrules

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var flags asset.FileInputFlags

	cmd := &cobra.Command{
		Use:   "update [id] -f <file>",
		Short: "Update a check rule from a file",
		Long: `Update an existing check rule from a YAML or JSON definition file. Use '-f -' to read from stdin.

Accepts both plain CheckRule definitions and PrometheusRule CRD files. When a PrometheusRule CRD is provided, the CRD must contain exactly one alerting rule.

If the ID argument is omitted, the ID is extracted from the file content.` + internal.CONFIG_HINT,
		Example: `  # Update a check rule from a file
  dash0 check-rules update <id> -f rule.yaml

  # Update using the ID from the file
  dash0 check-rules update -f rule.yaml

  # Update from a PrometheusRule CRD file
  dash0 check-rules update -f prometheus-rule.yaml

  # Export, edit, and update
  dash0 check-rules get <id> -o yaml > rule.yaml
  # edit rule.yaml
  dash0 check-rules update -f rule.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args, &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *asset.FileInputFlags) error {
	data, err := asset.ReadRawInput(flags.File, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read check rule definition: %w", err)
	}

	rules, err := dash0yaml.ParseAsPrometheusAlertRules(data)
	if err != nil {
		return err
	}

	if len(rules) > 1 {
		return fmt.Errorf("input contains %d check rules, but update requires exactly 1", len(rules))
	}
	rule := rules[0]

	id, err := resolveCheckRuleID(args, dash0api.GetCheckRuleID(rule))
	if err != nil {
		return err
	}

	return doUpdate(ctx, flags, id, rule)
}

func resolveCheckRuleID(args []string, fileID string) (string, error) {
	if len(args) == 1 {
		id := args[0]
		if fileID != "" && fileID != id {
			return "", fmt.Errorf("the ID argument %q does not match the ID in the file %q", id, fileID)
		}
		return id, nil
	}
	if fileID == "" {
		return "", fmt.Errorf("no check rule ID provided as argument, and the file does not contain an ID")
	}
	return fileID, nil
}

func doUpdate(ctx context.Context, flags *asset.FileInputFlags, id string, rule *dash0api.PrometheusAlertRule) error {
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	before, err := apiClient.GetCheckRule(ctx, id, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "check rule",
			AssetID:   id,
		})
	}

	if flags.DryRun {
		return asset.PrintDiff(os.Stdout, "Check rule", rule.Name, before, rule)
	}

	result, err := apiClient.UpdateCheckRule(ctx, id, rule, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "check rule",
			AssetID:   id,
			AssetName: rule.Name,
		})
	}

	return asset.PrintDiff(os.Stdout, "Check rule", result.Name, before, result)
}
