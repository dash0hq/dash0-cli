package views

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
		Short: "Apply a view definition (create or update)",
		Long: `Apply a view definition from a YAML or JSON file.
If the view exists, it will be updated. If it doesn't exist, it will be created.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApply(cmd.Context(), &flags)
		},
	}

	res.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runApply(ctx context.Context, flags *res.FileInputFlags) error {
	var view dash0.ViewDefinition
	if err := res.ReadDefinitionFile(flags.File, &view); err != nil {
		return fmt.Errorf("failed to read view definition: %w", err)
	}

	if flags.DryRun {
		fmt.Println("Dry run: view definition is valid")
		return nil
	}

	apiClient, err := client.NewClient(flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, err := apiClient.ImportView(ctx, &view, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err)
	}

	fmt.Printf("View %q applied successfully\n", result.Metadata.Name)
	return nil
}
