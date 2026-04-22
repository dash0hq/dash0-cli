package recordingrules

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var flags asset.FileInputFlags

	cmd := &cobra.Command{
		Use:   "update [id] -f <file>",
		Short: "Update a recording rule from a file",
		Long: `Update an existing recording rule from a YAML or JSON definition file in PrometheusRule CRD format. Use '-f -' to read from stdin.

If the ID argument is omitted, the ID is extracted from the file content.` + internal.CONFIG_HINT,
		Example: `  # Update a recording rule from a file
  dash0 recording-rules update <id> -f rule.yaml

  # Update using the ID from the file
  dash0 recording-rules update -f rule.yaml

  # Export, edit, and update
  dash0 recording-rules get <id> -o yaml > rule.yaml
  # edit rule.yaml
  dash0 recording-rules update -f rule.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args, &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *asset.FileInputFlags) error {
	var rule dash0api.RecordingRule
	if err := asset.ReadDefinition(flags.File, &rule, os.Stdin); err != nil {
		return fmt.Errorf("failed to read recording rule definition: %w", err)
	}

	var id string
	fileID := dash0api.GetRecordingRuleID(&rule)
	if len(args) == 1 {
		id = args[0]
		if fileID != "" && fileID != id {
			return fmt.Errorf("the ID argument %q does not match the ID in the file %q", id, fileID)
		}
	} else {
		id = fileID
		if id == "" {
			return fmt.Errorf("no recording rule ID provided as argument, and the file does not contain an ID")
		}
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	before, err := apiClient.GetRecordingRule(ctx, id, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "recording rule",
			AssetID:   id,
		})
	}

	asset.CarryRecordingRuleVersion(before, &rule)

	if flags.DryRun {
		return asset.PrintDiff(os.Stdout, "Recording rule", dash0api.GetRecordingRuleName(&rule), before, &rule)
	}

	result, err := apiClient.UpdateRecordingRule(ctx, id, &rule, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "recording rule",
			AssetID:   id,
			AssetName: dash0api.GetRecordingRuleName(&rule),
		})
	}

	return asset.PrintDiff(os.Stdout, "Recording rule", dash0api.GetRecordingRuleName(result), before, result)
}
