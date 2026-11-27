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

Accepts Dashboard and EphemeralDashboard definitions and PersesDashboard CRD files. When a PersesDashboard CRD is provided, it is converted to a Dash0 dashboard.

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

	dashboard, inferredName, err := asset.ParseDashboard(data)
	if err != nil {
		return err
	}
	if err := asset.ValidateEphemeralDashboard(dashboard); err != nil {
		return err
	}

	if err := asset.ValidateDashboard(dashboard); err != nil {
		return err
	}
	key := asset.UpdateKey{Noun: "dashboard", Upsert: asset.DashboardKeys(dashboard)}
	identity := asset.DashboardIdentity(dashboard, inferredName, nil)
	if err := asset.CheckRouteUpdate(args, key, identity); err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)
	asset.ClearDashboardDeletedAt(dashboard)

	get := func(ctx context.Context, key string) (*dash0api.DashboardDefinition, error) {
		return apiClient.GetDashboard(ctx, key, dataset)
	}
	id, before, err := asset.RouteUpdate(ctx, args, key, identity, get)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "dashboard",
			AssetID:   id,
		})
	}
	if err := asset.CheckDashboardKind(id, before, dashboard); err != nil {
		return err
	}

	displayName := dash0api.GetDashboardName(dashboard)
	if displayName == "" {
		displayName = dashboard.Metadata.Name
	}

	if flags.DryRun {
		return asset.PrintUpdateOutcome(os.Stdout, asset.UpdateOutcome{
			DisplayKind:     asset.DashboardDisplayKind(dashboard),
			Name:            displayName,
			ID:              id,
			Before:          before,
			After:           dashboard,
			BeforeDeletedAt: asset.DashboardDeletedAt(before),
			DryRun:          true,
		})
	}

	dash0api.ClearDashboardID(dashboard)
	asset.ClearUpdateOrigin(dashboard)
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
	return asset.PrintUpdateOutcome(os.Stdout, asset.UpdateOutcome{
		DisplayKind:     asset.DashboardDisplayKind(result),
		Name:            resultDisplayName,
		ID:              id,
		Before:          before,
		After:           result,
		BeforeDeletedAt: asset.DashboardDeletedAt(before),
		AfterDeletedAt:  asset.DashboardDeletedAt(result),
	})
}
