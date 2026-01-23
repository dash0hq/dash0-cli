package dashboards

import (
	"context"
	"fmt"

	"github.com/dash0hq/dash0-cli/internal/client"
	res "github.com/dash0hq/dash0-cli/internal/resource"
	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	var flags res.ExportFlags

	cmd := &cobra.Command{
		Use:   "export <id>",
		Short: "Export a dashboard to a file",
		Long:  `Export a dashboard definition to a YAML or JSON file`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(cmd.Context(), args[0], &flags)
		},
	}

	res.RegisterExportFlags(cmd, &flags)
	return cmd
}

func runExport(ctx context.Context, id string, flags *res.ExportFlags) error {
	apiClient, err := client.NewClient(flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dashboard, err := apiClient.GetDashboard(ctx, id, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err)
	}

	// If file is specified, write to file; otherwise write to stdout
	if flags.File != "" {
		if err := res.WriteDefinitionFile(flags.File, dashboard); err != nil {
			return fmt.Errorf("failed to write dashboard to file: %w", err)
		}
		fmt.Printf("Dashboard exported to %s\n", flags.File)
	} else {
		// Default to YAML for stdout
		format := "yaml"
		if flags.Output == "json" {
			format = "json"
		}
		if err := res.WriteToStdout(format, dashboard); err != nil {
			return fmt.Errorf("failed to write dashboard: %w", err)
		}
	}

	return nil
}
