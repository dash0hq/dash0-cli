package dashboards

import (
	"context"
	"fmt"
	"os"

	dash0 "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/client"
	res "github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var flags res.FileInputFlags

	cmd := &cobra.Command{
		Use:   "update <id> -f <file>",
		Short: "Update a dashboard from a file",
		Long:  `Update an existing dashboard from a YAML or JSON definition file. Use '-f -' to read from stdin.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args[0], &flags)
		},
	}

	res.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, id string, flags *res.FileInputFlags) error {
	var dashboard dash0.DashboardDefinition
	if err := res.ReadDefinition(flags.File, &dashboard, os.Stdin); err != nil {
		return fmt.Errorf("failed to read dashboard definition: %w", err)
	}

	// Set origin to dash0-cli (using Id field since Origin is deprecated)
	if dashboard.Metadata.Dash0Extensions == nil {
		dashboard.Metadata.Dash0Extensions = &dash0.DashboardMetadataExtensions{}
	}
	origin := "dash0-cli"
	dashboard.Metadata.Dash0Extensions.Id = &origin

	if flags.DryRun {
		fmt.Println("Dry run: dashboard definition is valid")
		return nil
	}

	apiClient, err := client.NewClient(flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, err := apiClient.UpdateDashboard(ctx, id, &dashboard, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err)
	}

	fmt.Printf("Dashboard %q updated successfully\n", result.Metadata.Name)
	return nil
}
