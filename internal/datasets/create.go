package datasets

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

type createFlags struct {
	ApiUrl    string
	AuthToken string
	File      string
	DryRun    bool
}

func newCreateCmd() *cobra.Command {
	flags := &createFlags{}

	cmd := &cobra.Command{
		Use:     "create -f <file>",
		Aliases: []string{"add"},
		Short:   "[experimental] Create a dataset from a file",
		Long: `Create a dataset from a YAML or JSON definition file. Use '-f -' to read from stdin.

The dataset slug in metadata.labels["dash0.com/id"] is required and cannot be changed later.
If a dataset with that slug exists, it is updated.
If it was deleted in the last 30 days, it is restored.` + internal.CONFIG_HINT,
		Example: `  # Create from a YAML file
  dash0 --experimental datasets create -f dataset.yaml

  # Create from stdin
  cat dataset.yaml | dash0 --experimental datasets create -f -

  # Validate without creating
  dash0 --experimental datasets create -f dataset.yaml --dry-run`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runCreate(cmd.Context(), flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Path to YAML or JSON definition file (use '-' for stdin)")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Validate without creating/updating")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

func runCreate(ctx context.Context, flags *createFlags) error {
	var dataset dash0api.DatasetDefinition
	if err := asset.ReadDefinition(flags.File, &dataset, os.Stdin); err != nil {
		return fmt.Errorf("failed to read dataset definition: %w", err)
	}

	if dash0api.GetDatasetID(&dataset) == "" {
		return asset.ErrDatasetSlugRequired
	}

	if flags.DryRun {
		fmt.Println("Dry run: dataset definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	// Must run before ImportDataset, which strips the field.
	if warning := asset.TelemetryFiltersWarning(&dataset); warning != "" {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
	}

	result, err := asset.ImportDataset(ctx, apiClient, &dataset)
	if err != nil {
		err = client.HandleAPIError(err, client.ErrorContext{
			AssetType: "dataset",
			AssetID:   dash0api.GetDatasetID(&dataset),
			AssetName: dash0api.GetDatasetName(&dataset),
		})
		if result.After != nil {
			fmt.Printf("Dataset %q %s (id: %s)\n", result.Name, result.Action, result.ID)
			return asset.DatasetPermissionsNotApplied(err)
		}
		return err
	}

	fmt.Printf("Dataset %q %s (id: %s)\n", result.Name, result.Action, result.ID)
	return nil
}
