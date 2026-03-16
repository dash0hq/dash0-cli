package recordingrulegroups

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
		Short: "Update a recording rule group from a file",
		Long: `Update an existing recording rule group from a YAML or JSON definition file. Use '-f -' to read from stdin.

If the ID argument is omitted, the ID is extracted from the file content.` + internal.CONFIG_HINT,
		Example: `  # Update a recording rule group from a file
  dash0 recording-rule-groups update <id> -f group.yaml

  # Update using the ID from the file
  dash0 recording-rule-groups update -f group.yaml

  # Export, edit, and update
  dash0 recording-rule-groups get <id> -o yaml > group.yaml
  # edit group.yaml
  dash0 recording-rule-groups update -f group.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args, &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *asset.FileInputFlags) error {
	var group dash0api.RecordingRuleGroupDefinition
	if err := asset.ReadDefinition(flags.File, &group, os.Stdin); err != nil {
		return fmt.Errorf("failed to read recording rule group definition: %w", err)
	}

	var id string
	fileID := asset.ExtractRecordingRuleGroupID(&group)
	if len(args) == 1 {
		id = args[0]
		if fileID != "" && fileID != id {
			return fmt.Errorf("the ID argument %q does not match the ID in the file %q", id, fileID)
		}
	} else {
		id = fileID
		if id == "" {
			return fmt.Errorf("no recording rule group ID provided as argument, and the file does not contain an ID")
		}
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	before, err := apiClient.GetRecordingRuleGroup(ctx, id, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "recording rule group",
			AssetID:   id,
		})
	}

	if flags.DryRun {
		return asset.PrintDiff(os.Stdout, "Recording rule group", group.Metadata.Name, before, &group)
	}

	result, err := apiClient.UpdateRecordingRuleGroup(ctx, id, &group)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "recording rule group",
			AssetID:   id,
			AssetName: asset.ExtractRecordingRuleGroupName(&group),
		})
	}

	return asset.PrintDiff(os.Stdout, "Recording rule group", asset.ExtractRecordingRuleGroupName(result), before, result)
}
