package dashboards

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
		Short: "Update a dashboard from a file",
		Long: `Update an existing dashboard from a YAML or JSON definition file. Use '-f -' to read from stdin.

Accepts both plain Dashboard definitions and PersesDashboard CRD files. When a PersesDashboard CRD is provided, it is converted to a Dash0 dashboard.

If the ID argument is omitted, the ID is extracted from the file content.` + internal.CONFIG_HINT,
		Example: `  # Update a dashboard from a file
  dash0 dashboards update <id> -f dashboard.yaml

  # Update using the ID from the file
  dash0 dashboards update -f dashboard.yaml

  # Update from a PersesDashboard CRD file
  dash0 dashboards update -f persesdashboard.yaml

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
	data, err := asset.ReadRawInput(flags.File, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read dashboard definition: %w", err)
	}

	dashboard, err := dash0yaml.ParseAsDashboard(data)
	if err != nil {
		return err
	}

	id, err := resolveDashboardID(args, dash0api.GetDashboardID(dashboard))
	if err != nil {
		return err
	}

	return doUpdate(ctx, flags, id, dashboard)
}

func resolveDashboardID(args []string, fileID string) (string, error) {
	if len(args) == 1 {
		id := args[0]
		if fileID != "" && fileID != id {
			return "", fmt.Errorf("the ID argument %q does not match the ID in the file %q", id, fileID)
		}
		return id, nil
	}
	if fileID == "" {
		return "", fmt.Errorf("no dashboard ID provided as argument, and the file does not contain an ID")
	}
	return fileID, nil
}

func doUpdate(ctx context.Context, flags *asset.FileInputFlags, id string, dashboard *dash0api.DashboardDefinition) error {
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	before, err := apiClient.GetDashboard(ctx, id, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "dashboard",
			AssetID:   id,
		})
	}

	displayName := dash0api.GetDashboardName(dashboard)
	if displayName == "" {
		displayName = dashboard.Metadata.Name
	}

	if flags.DryRun {
		return asset.PrintDiff(os.Stdout, "Dashboard", displayName, before, dashboard)
	}

	dash0api.ClearDashboardID(dashboard)
	result, err := apiClient.UpdateDashboard(ctx, id, dashboard, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "dashboard",
			AssetID:   id,
			AssetName: displayName,
		})
	}

	resultDisplayName := dash0api.GetDashboardName(result)
	if resultDisplayName == "" {
		resultDisplayName = result.Metadata.Name
	}
	return asset.PrintDiff(os.Stdout, "Dashboard", resultDisplayName, before, result)
}
