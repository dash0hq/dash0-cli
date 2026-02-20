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
		Use:   "update <id> -f <file>",
		Short: "Update a synthetic check from a file",
		Long: `Update an existing synthetic check from a YAML or JSON definition file. Use '-f -' to read from stdin.` + internal.CONFIG_HINT,
		Example: `  # Update a synthetic check from a file
  dash0 synthetic-checks update <id> -f check.yaml

  # Export, edit, and update
  dash0 synthetic-checks get <id> -o yaml > check.yaml
  # edit check.yaml
  dash0 synthetic-checks update <id> -f check.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args[0], &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, id string, flags *asset.FileInputFlags) error {
	var check dash0api.SyntheticCheckDefinition
	if err := asset.ReadDefinition(flags.File, &check, os.Stdin); err != nil {
		return fmt.Errorf("failed to read synthetic check definition: %w", err)
	}

	// Set origin to dash0-cli
	if check.Metadata.Labels == nil {
		check.Metadata.Labels = &dash0api.SyntheticCheckLabels{}
	}
	origin := internal.DEFAULT_ORIGIN
	check.Metadata.Labels.Dash0Comorigin = &origin

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
