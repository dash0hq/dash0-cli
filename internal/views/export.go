package views

import (
	"context"
	"fmt"

	dash0 "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/client"
	res "github.com/dash0hq/dash0-cli/internal/resource"
	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	var flags res.ExportFlags

	cmd := &cobra.Command{
		Use:   "export <id>",
		Short: "Export a view to a file",
		Long:  `Export a view definition to a YAML or JSON file`,
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

	view, err := apiClient.GetView(ctx, id, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err)
	}

	// Ensure the view ID is preserved for upsert semantics on apply
	if view.Metadata.Labels == nil {
		view.Metadata.Labels = &dash0.ViewLabels{}
	}
	if view.Metadata.Labels.Dash0Comid == nil {
		view.Metadata.Labels.Dash0Comid = &id
	}

	if flags.File != "" {
		if err := res.WriteDefinitionFile(flags.File, view); err != nil {
			return fmt.Errorf("failed to write view to file: %w", err)
		}
		fmt.Printf("View exported to %s\n", flags.File)
	} else {
		format := "yaml"
		if flags.Output == "json" {
			format = "json"
		}
		if err := res.WriteToStdout(format, view); err != nil {
			return fmt.Errorf("failed to write view: %w", err)
		}
	}

	return nil
}
