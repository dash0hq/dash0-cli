package dashboards

import (
	"context"
	"fmt"
	"os"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

func newCreateCmd() *cobra.Command {
	var flags asset.FileInputFlags

	cmd := &cobra.Command{
		Use:     "create -f <file>",
		Aliases: []string{"add"},
		Short:   "Create a dashboard from a file",
		Long: `Create a new dashboard from a YAML or JSON definition file. Use '-f -' to read from stdin.

Accepts both plain Dashboard definitions and PersesDashboard CRD files. When a PersesDashboard CRD is provided, it is converted to a Dash0 dashboard.` + internal.CONFIG_HINT,
		Example: `  # Create from a YAML file
  dash0 dashboards create -f dashboard.yaml

  # Create from a PersesDashboard CRD file
  dash0 dashboards create -f persesdashboard.yaml

  # Create from stdin
  cat dashboard.yaml | dash0 dashboards create -f -

  # Validate without creating
  dash0 dashboards create -f dashboard.yaml --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd.Context(), &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runCreate(ctx context.Context, flags *asset.FileInputFlags) error {
	data, err := asset.ReadRawInput(flags.File, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read dashboard definition: %w", err)
	}

	kind := strings.ToLower(asset.DetectKind(data))
	if kind == "persesdashboard" {
		return createFromPersesDashboard(ctx, flags, data)
	}
	return createFromDashboard(ctx, flags, data)
}

func createFromDashboard(ctx context.Context, flags *asset.FileInputFlags, data []byte) error {
	var dashboard dash0api.DashboardDefinition
	if err := yaml.Unmarshal(data, &dashboard); err != nil {
		return fmt.Errorf("failed to read dashboard definition: %w", err)
	}

	if flags.DryRun {
		fmt.Println("Dry run: dashboard definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, importErr := asset.ImportDashboard(ctx, apiClient, &dashboard, client.ResolveDataset(ctx, flags.Dataset))
	if importErr != nil {
		return client.HandleAPIError(importErr, client.ErrorContext{
			AssetType: "dashboard",
			AssetName: asset.ExtractDashboardDisplayName(&dashboard),
		})
	}

	fmt.Printf("Dashboard %q %s\n", result.Name, result.Action)
	return nil
}

func createFromPersesDashboard(ctx context.Context, flags *asset.FileInputFlags, data []byte) error {
	var perses asset.PersesDashboard
	if err := yaml.Unmarshal(data, &perses); err != nil {
		return fmt.Errorf("failed to read PersesDashboard definition: %w", err)
	}

	if flags.DryRun {
		fmt.Println("Dry run: PersesDashboard definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, importErr := asset.ImportPersesDashboard(ctx, apiClient, &perses, client.ResolveDataset(ctx, flags.Dataset))
	if importErr != nil {
		return client.HandleAPIError(importErr, client.ErrorContext{
			AssetType: "dashboard",
			AssetName: asset.ExtractPersesDashboardName(&perses),
		})
	}

	fmt.Printf("Dashboard %q %s\n", result.Name, result.Action)
	return nil
}
