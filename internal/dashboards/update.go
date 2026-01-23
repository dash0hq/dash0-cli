package dashboards

import (
	"context"
	"fmt"

	dash0 "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/client"
	res "github.com/dash0hq/dash0-cli/internal/resource"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var flags res.FileInputFlags

	cmd := &cobra.Command{
		Use:   "update <id> -f <file>",
		Short: "Update a dashboard from a file",
		Long:  `Update an existing dashboard from a YAML or JSON definition file`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args[0], &flags)
		},
	}

	res.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, id string, flags *res.FileInputFlags) error {
	// Read dashboard definition from file
	var dashboard dash0.DashboardDefinition
	if err := res.ReadDefinitionFile(flags.File, &dashboard); err != nil {
		return fmt.Errorf("failed to read dashboard definition: %w", err)
	}

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
