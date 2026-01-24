package dashboards

import (
	"context"
	"fmt"

	dash0 "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/client"
	res "github.com/dash0hq/dash0-cli/internal/resource"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var flags res.FileInputFlags

	cmd := &cobra.Command{
		Use:   "create -f <file>",
		Short: "Create a dashboard from a file",
		Long:  `Create a new dashboard from a YAML or JSON definition file`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd.Context(), &flags)
		},
	}

	res.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runCreate(ctx context.Context, flags *res.FileInputFlags) error {
	// Read dashboard definition from file
	var dashboard dash0.DashboardDefinition
	if err := res.ReadDefinitionFile(flags.File, &dashboard); err != nil {
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

	result, err := apiClient.CreateDashboard(ctx, &dashboard, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err)
	}

	fmt.Printf("Dashboard %q created successfully\n", result.Metadata.Name)
	return nil
}
