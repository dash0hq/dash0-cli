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
		Short: "Get an SLO by ID or origin",
		Long: `Retrieve an SLO definition by its ID or its dash0.com/origin label.` +
			internal.CONFIG_HINT,
		Example: `  # Show SLO summary
  dash0 slos get <id>

  # Look up by the dash0.com/origin label instead
  dash0 slos get <origin>

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

func runGet(ctx context.Context, originOrID string, flags *asset.GetFlags) error {
	apiUrl := client.ResolveApiUrl(ctx, flags.ApiUrl)
	dataset := client.ResolveDataset(ctx, flags.Dataset)
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	slo, err := apiClient.GetSLO(ctx, originOrID, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "SLO",
			AssetID:   originOrID,
		})
	}

	// The argument may be an origin or an id — `GET /api/slos/{originOrId}`
	// accepts either, and origin is the recommended key. Read the id back off
	// the response so the id label and the deep link are built from a real id;
	// a deep link built from an origin does not resolve. Fall back to the
	// argument only when the response carries no id at all.
	id := dash0api.GetSLOID(slo)
	if id == "" {
		id = originOrID
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
		// Origin is the SLO upsert key, so it is the field users script
		// against — print it whenever the SLO has one.
		if origin := sloOrigin(slo); origin != "" {
			fmt.Printf("Origin: %s\n", origin)
		}
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
