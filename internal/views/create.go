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

func newCreateCmd() *cobra.Command {
	var flags res.FileInputFlags

	cmd := &cobra.Command{
		Use:     "create -f <file>",
		Aliases: []string{"add"},
		Short:   "Create a view from a file",
		Long:    `Create a new view from a YAML or JSON definition file. Use '-f -' to read from stdin.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd.Context(), &flags)
		},
	}

	res.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runCreate(ctx context.Context, flags *res.FileInputFlags) error {
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

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, err := apiClient.CreateView(ctx, &view, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "view",
			AssetName: view.Metadata.Name,
		})
	}

	fmt.Printf("View %q created successfully\n", result.Metadata.Name)
	return nil
}
