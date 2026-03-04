package syntheticchecks

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
		Short: "Update a synthetic check from a file",
		Long: `Update an existing synthetic check from a YAML or JSON definition file. Use '-f -' to read from stdin.

If the ID argument is omitted, the ID is extracted from the file content.` + internal.CONFIG_HINT,
		Example: `  # Update a synthetic check from a file
  dash0 synthetic-checks update <id> -f check.yaml

  # Update using the ID from the file
  dash0 synthetic-checks update -f check.yaml

  # Export, edit, and update
  dash0 synthetic-checks get <id> -o yaml > check.yaml
  # edit check.yaml
  dash0 synthetic-checks update -f check.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args, &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *asset.FileInputFlags) error {
	var check dash0api.SyntheticCheckDefinition
	if err := asset.ReadDefinition(flags.File, &check, os.Stdin); err != nil {
		return fmt.Errorf("failed to read synthetic check definition: %w", err)
	}

	var id string
	fileID := asset.ExtractSyntheticCheckID(&check)
	if len(args) == 1 {
		id = args[0]
		if fileID != "" && fileID != id {
			return fmt.Errorf("the ID argument %q does not match the ID in the file %q", id, fileID)
		}
	} else {
		id = fileID
		if id == "" {
			return fmt.Errorf("no synthetic check ID provided as argument, and the file does not contain an ID")
		}
	}

	if flags.DryRun {
		fmt.Println("Dry run: synthetic check definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, err := apiClient.UpdateSyntheticCheck(ctx, id, &check, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "synthetic check",
			AssetID:   id,
			AssetName: check.Metadata.Name,
		})
	}

	fmt.Printf("Synthetic check %q updated successfully\n", result.Metadata.Name)
	return nil
}
