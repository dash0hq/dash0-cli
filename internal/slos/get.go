package slos

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	var flags asset.GetFlags

	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get an SLO by ID",
		Long:  `Retrieve an SLO definition by its ID.` + internal.CONFIG_HINT,
		Example: `  # Show SLO summary
  dash0 slos get <id>

  # Export as YAML (suitable for re-applying)
  dash0 slos get <id> -o yaml > slo.yaml

  # Export as JSON
  dash0 slos get <id> -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd.Context(), args[0], &flags)
		},
	}

	asset.RegisterGetFlags(cmd, &flags)
	return cmd
}

func runGet(ctx context.Context, id string, flags *asset.GetFlags) error {
	apiUrl := client.ResolveApiUrl(ctx, flags.ApiUrl)
	dataset := client.ResolveDataset(ctx, flags.Dataset)
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	slo, err := apiClient.GetSLO(ctx, id, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "SLO",
			AssetID:   id,
		})
	}

	dash0api.SetSLOIDIfAbsent(slo, id)

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout)

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return formatter.Print(slo)
	default:
		fmt.Printf("Kind: %s\n", slo.Kind)
		fmt.Printf("Name: %s\n", dash0api.GetSLOName(slo))
		fmt.Printf("Dataset: %s\n", dash0api.GetSLODataset(slo))
		if slo.Spec.Service != nil && *slo.Spec.Service != "" {
			fmt.Printf("Service: %s\n", *slo.Spec.Service)
		}
		if slo.Spec.Description != nil && *slo.Spec.Description != "" {
			fmt.Printf("Description: %s\n", *slo.Spec.Description)
		}
		if deeplinkURL := dash0api.DeeplinkURL(apiUrl, dash0api.DeeplinkAssetTypeSLO, id, dataset); deeplinkURL != "" {
			fmt.Printf("URL: %s\n", deeplinkURL)
		}
		return nil
	}
}
