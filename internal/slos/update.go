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

func newUpdateCmd() *cobra.Command {
	var flags asset.FileInputFlags

	cmd := &cobra.Command{
		Use:   "update [id] -f <file>",
		Short: "Update an SLO from a file",
		Long: `Update an existing SLO from a YAML or JSON definition file in OpenSLO v1 format. Use '-f -' to read from stdin.

If the ID argument is omitted, the ID is extracted from the file content.` + internal.CONFIG_HINT,
		Example: `  # Update an SLO from a file
  dash0 slos update <id> -f slo.yaml

  # Update using the ID from the file
  dash0 slos update -f slo.yaml

  # Export, edit, and update
  dash0 slos get <id> -o yaml > slo.yaml
  # edit slo.yaml
  dash0 slos update -f slo.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args, &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *asset.FileInputFlags) error {
	var slo dash0api.SloDefinition
	if err := asset.ReadDefinition(flags.File, &slo, os.Stdin); err != nil {
		return fmt.Errorf("failed to read SLO definition: %w", err)
	}

	var id string
	fileID := dash0api.GetSLOID(&slo)
	if len(args) == 1 {
		id = args[0]
		if fileID != "" && fileID != id {
			return fmt.Errorf("the ID argument %q does not match the ID in the file %q", id, fileID)
		}
	} else {
		id = fileID
		if id == "" {
			return fmt.Errorf("no SLO ID provided as argument, and the file does not contain an ID")
		}
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	before, err := apiClient.GetSLO(ctx, id, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "SLO",
			AssetID:   id,
		})
	}

	if flags.DryRun {
		return asset.PrintDiff(os.Stdout, "SLO", slo.Metadata.Name, before, &slo)
	}

	result, err := apiClient.UpdateSLO(ctx, id, &slo, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "SLO",
			AssetID:   id,
			AssetName: dash0api.GetSLOName(&slo),
		})
	}

	return asset.PrintDiff(os.Stdout, "SLO", dash0api.GetSLOName(result), before, result)
}
