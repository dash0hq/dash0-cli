package views

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
		Short: "Get a view by ID",
		Long:  `Retrieve a view definition by its ID.` + internal.CONFIG_HINT,
		Example: `  # Show view summary
  dash0 views get <id>

  # Export as YAML (suitable for re-applying)
  dash0 views get <id> -o yaml > view.yaml

  # Export as JSON
  dash0 views get <id> -o json`,
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

	view, err := apiClient.GetView(ctx, id, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "view",
			AssetID:   id,
		})
	}

	dash0api.SetViewIDIfAbsent(view, id)

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout)

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return formatter.Print(view)
	default:
		fmt.Printf("Kind: %s\n", view.Kind)
		fmt.Printf("Name: %s\n", dash0api.GetViewName(view))
		viewDataset := ""
		origin := ""
		if view.Metadata.Labels != nil {
			if view.Metadata.Labels.Dash0Comdataset != nil {
				viewDataset = *view.Metadata.Labels.Dash0Comdataset
			}
			if view.Metadata.Labels.Dash0Comorigin != nil {
				origin = *view.Metadata.Labels.Dash0Comorigin
			}
		}
		fmt.Printf("Dataset: %s\n", viewDataset)
		fmt.Printf("Origin: %s\n", origin)
		if deeplinkURL := dash0api.ViewDeeplinkURL(apiUrl, view.Spec.Type, id, dataset); deeplinkURL != "" {
			fmt.Printf("URL: %s\n", deeplinkURL)
		}
		return nil
	}
}
