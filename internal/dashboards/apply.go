package dashboards

import (
	"context"
	"fmt"

	dash0 "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/client"
	res "github.com/dash0hq/dash0-cli/internal/resource"
	"github.com/spf13/cobra"
)

func newApplyCmd() *cobra.Command {
	var flags res.FileInputFlags

	cmd := &cobra.Command{
		Use:   "apply -f <file>",
		Short: "Apply a dashboard definition (create or update)",
		Long: `Apply a dashboard definition from a YAML or JSON file.
If the dashboard exists, it will be updated. If it doesn't exist, it will be created.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApply(cmd.Context(), &flags)
		},
	}

	res.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runApply(ctx context.Context, flags *res.FileInputFlags) error {
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

	// Use Import for upsert semantics
	result, err := apiClient.ImportDashboard(ctx, &dashboard, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err)
	}

	fmt.Printf("Dashboard %q applied successfully\n", result.Metadata.Name)
	return nil
}
