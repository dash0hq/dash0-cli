package slos

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var flags asset.FileInputFlags

	cmd := &cobra.Command{
		Use:     "create -f <file>",
		Aliases: []string{"add"},
		Short:   "Create an SLO from a file",
		Long:    `Create a new SLO from a YAML or JSON definition file in OpenSLO v1 format. Use '-f -' to read from stdin.` + internal.CONFIG_HINT,
		Example: `  # Create from a YAML file
  dash0 slos create -f slo.yaml

  # Create from stdin
  cat slo.yaml | dash0 slos create -f -

  # Validate without creating
  dash0 slos create -f slo.yaml --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd.Context(), &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runCreate(ctx context.Context, flags *asset.FileInputFlags) error {
	var slo dash0api.SloDefinition
	if err := asset.ReadDefinition(flags.File, &slo, os.Stdin); err != nil {
		return fmt.Errorf("failed to read SLO definition: %w", err)
	}

	if flags.DryRun {
		fmt.Println("Dry run: SLO definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, importErr := asset.ImportSLO(ctx, apiClient, &slo, client.ResolveDataset(ctx, flags.Dataset))
	if importErr != nil {
		return client.HandleAPIError(importErr, client.ErrorContext{
			AssetType: "SLO",
			AssetName: dash0api.GetSLOName(&slo),
		})
	}

	fmt.Printf("SLO %q %s\n", result.Name, result.Action)
	return nil
}
