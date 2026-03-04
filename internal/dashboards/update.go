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
		Use:   "update [id] -f <file>",
		Short: "Update a dashboard from a file",
		Long: `Update an existing dashboard from a YAML or JSON definition file. Use '-f -' to read from stdin.

If the ID argument is omitted, the ID is extracted from the file content.` + internal.CONFIG_HINT,
		Example: `  # Update a dashboard from a file
  dash0 dashboards update <id> -f dashboard.yaml

  # Update using the ID from the file
  dash0 dashboards update -f dashboard.yaml

  # Export, edit, and update
  dash0 dashboards get <id> -o yaml > dashboard.yaml
  # edit dashboard.yaml
  dash0 dashboards update -f dashboard.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args, &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *asset.FileInputFlags) error {
	var dashboard dash0api.DashboardDefinition
	if err := asset.ReadDefinition(flags.File, &dashboard, os.Stdin); err != nil {
		return fmt.Errorf("failed to read dashboard definition: %w", err)
	}

	var id string
	fileID := asset.ExtractDashboardID(&dashboard)
	if len(args) == 1 {
		id = args[0]
		if fileID != "" && fileID != id {
			return fmt.Errorf("the ID argument %q does not match the ID in the file %q", id, fileID)
		}
	} else {
		id = fileID
		if id == "" {
			return fmt.Errorf("no dashboard ID provided as argument, and the file does not contain an ID")
		}
	}

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
