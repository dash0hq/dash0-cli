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

	dashboard, err := dash0yaml.ParseAsDashboard(data)
	if err != nil {
		return err
	}

	if flags.DryRun {
		fmt.Println("Dry run: dashboard definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, importErr := asset.ImportDashboard(ctx, apiClient, dashboard, client.ResolveDataset(ctx, flags.Dataset))
	if importErr != nil {
		return client.HandleAPIError(importErr, client.ErrorContext{
			AssetType: "dashboard",
			AssetName: dash0api.GetDashboardName(dashboard),
		})
	}

	fmt.Printf("Dashboard %q %s\n", result.Name, result.Action)
	return nil
}
