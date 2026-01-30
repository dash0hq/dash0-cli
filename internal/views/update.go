package views

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
		Short: "Update a view from a file",
		Long:  `Update an existing view from a YAML or JSON definition file. Use '-f -' to read from stdin.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args[0], &flags)
		},
	}

	res.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, id string, flags *res.FileInputFlags) error {
	var view dash0.ViewDefinition
	if err := res.ReadDefinition(flags.File, &view, os.Stdin); err != nil {
		return fmt.Errorf("failed to read view definition: %w", err)
	}

	// Set origin to dash0-cli
	if view.Metadata.Labels == nil {
		view.Metadata.Labels = &dash0.ViewLabels{}
	}
	origin := "dash0-cli"
	view.Metadata.Labels.Dash0Comorigin = &origin

	if flags.DryRun {
		fmt.Println("Dry run: view definition is valid")
		return nil
	}

	apiClient, err := client.NewClient(flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, err := apiClient.UpdateView(ctx, id, &view, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err)
	}

	fmt.Printf("View %q updated successfully\n", result.Metadata.Name)
	return nil
}
