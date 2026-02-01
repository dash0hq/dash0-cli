package views

import (
	"context"
	"fmt"
	"os"

	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	var flags asset.GetFlags

	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a view by ID",
		Long:  `Retrieve a view definition by its ID`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd.Context(), args[0], &flags)
		},
	}

	asset.RegisterGetFlags(cmd, &flags)
	return cmd
}

func runGet(ctx context.Context, id string, flags *asset.GetFlags) error {
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	view, err := apiClient.GetView(ctx, id, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "view",
			AssetID:   id,
		})
	}

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
		fmt.Printf("Name: %s\n", view.Metadata.Name)
		dataset := ""
		origin := ""
		if view.Metadata.Labels != nil {
			if view.Metadata.Labels.Dash0Comdataset != nil {
				dataset = *view.Metadata.Labels.Dash0Comdataset
			}
			if view.Metadata.Labels.Dash0Comorigin != nil {
				origin = *view.Metadata.Labels.Dash0Comorigin
			}
		}
		fmt.Printf("Dataset: %s\n", dataset)
		fmt.Printf("Origin: %s\n", origin)
		return nil
	}
}
