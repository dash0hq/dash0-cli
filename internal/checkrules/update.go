package checkrules

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
		Short: "Update a check rule from a file",
		Long: `Update an existing check rule from a YAML or JSON definition file. Use '-f -' to read from stdin.

If the ID argument is omitted, the ID is extracted from the file content.` + internal.CONFIG_HINT,
		Example: `  # Update a check rule from a file
  dash0 check-rules update <id> -f rule.yaml

  # Update using the ID from the file
  dash0 check-rules update -f rule.yaml

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
	var rule dash0api.PrometheusAlertRule
	if err := asset.ReadDefinition(flags.File, &rule, os.Stdin); err != nil {
		return fmt.Errorf("failed to read check rule definition: %w", err)
	}

	var id string
	fileID := asset.ExtractCheckRuleID(&rule)
	if len(args) == 1 {
		id = args[0]
		if fileID != "" && fileID != id {
			return fmt.Errorf("the ID argument %q does not match the ID in the file %q", id, fileID)
		}
	} else {
		id = fileID
		if id == "" {
			return fmt.Errorf("no check rule ID provided as argument, and the file does not contain an ID")
		}
	}

	if flags.DryRun {
		fmt.Println("Dry run: check rule definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, err := apiClient.UpdateCheckRule(ctx, id, &rule, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "check rule",
			AssetID:   id,
			AssetName: rule.Name,
		})
	}

	fmt.Printf("Check rule %q updated successfully\n", result.Name)
	return nil
}
