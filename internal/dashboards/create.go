package dashboards

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
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
		Long:    `Create a new dashboard from a YAML or JSON definition file. Use '-f -' to read from stdin.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd.Context(), &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runCreate(ctx context.Context, flags *asset.FileInputFlags) error {
	var dashboard dash0api.DashboardDefinition
	if err := asset.ReadDefinition(flags.File, &dashboard, os.Stdin); err != nil {
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

	result, importErr := asset.ImportDashboard(ctx, apiClient, &dashboard, client.DatasetPtr(flags.Dataset))
	if importErr != nil {
		return client.HandleAPIError(importErr, client.ErrorContext{
			AssetType: "dashboard",
			AssetName: asset.ExtractDashboardDisplayName(&dashboard),
		})
	}

	fmt.Printf("Dashboard %q %s successfully\n", result.Name, result.Action)
	return nil
}
