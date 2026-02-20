package views

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
		Short: "Update a view from a file",
		Long: `Update an existing view from a YAML or JSON definition file. Use '-f -' to read from stdin.` + internal.CONFIG_HINT,
		Example: `  # Update a view from a file
  dash0 views update <id> -f view.yaml

  # Export, edit, and update
  dash0 views get <id> -o yaml > view.yaml
  # edit view.yaml
  dash0 views update <id> -f view.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args[0], &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, id string, flags *asset.FileInputFlags) error {
	var view dash0api.ViewDefinition
	if err := asset.ReadDefinition(flags.File, &view, os.Stdin); err != nil {
		return fmt.Errorf("failed to read view definition: %w", err)
	}

	// Set origin to dash0-cli
	if view.Metadata.Labels == nil {
		view.Metadata.Labels = &dash0api.ViewLabels{}
	}
	origin := internal.DEFAULT_ORIGIN
	view.Metadata.Labels.Dash0Comorigin = &origin

	if flags.DryRun {
		fmt.Println("Dry run: view definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, err := apiClient.UpdateView(ctx, id, &view, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "view",
			AssetID:   id,
			AssetName: view.Metadata.Name,
		})
	}

	fmt.Printf("View %q updated successfully\n", result.Metadata.Name)
	return nil
}
