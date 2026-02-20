package dashboards

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
		Short: "Update a dashboard from a file",
		Long: `Update an existing dashboard from a YAML or JSON definition file. Use '-f -' to read from stdin.` + internal.CONFIG_HINT,
		Example: `  # Update a dashboard from a file
  dash0 dashboards update <id> -f dashboard.yaml

  # Export, edit, and update
  dash0 dashboards get <id> -o yaml > dashboard.yaml
  # edit dashboard.yaml
  dash0 dashboards update <id> -f dashboard.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args[0], &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, id string, flags *asset.FileInputFlags) error {
	var dashboard dash0api.DashboardDefinition
	if err := asset.ReadDefinition(flags.File, &dashboard, os.Stdin); err != nil {
		return fmt.Errorf("failed to read dashboard definition: %w", err)
	}

	// Set origin to dash0-cli (using Id field since Origin is deprecated)
	if dashboard.Metadata.Dash0Extensions == nil {
		dashboard.Metadata.Dash0Extensions = &dash0api.DashboardMetadataExtensions{}
	}
	origin := internal.DEFAULT_ORIGIN
	dashboard.Metadata.Dash0Extensions.Id = &origin

	if flags.DryRun {
		fmt.Println("Dry run: dashboard definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, err := apiClient.UpdateDashboard(ctx, id, &dashboard, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "dashboard",
			AssetID:   id,
			AssetName: asset.ExtractDashboardDisplayName(&dashboard),
		})
	}

	fmt.Printf("Dashboard %q updated successfully\n", result.Metadata.Name)
	return nil
}
