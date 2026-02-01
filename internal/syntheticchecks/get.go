package syntheticchecks

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
		Short: "Get a synthetic check by ID",
		Long:  `Retrieve a synthetic check definition by its ID`,
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

	check, err := apiClient.GetSyntheticCheck(ctx, id, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "synthetic check",
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
		return formatter.Print(check)
	default:
		fmt.Printf("Kind: %s\n", check.Kind)
		fmt.Printf("Name: %s\n", check.Metadata.Name)
		dataset := ""
		origin := ""
		if check.Metadata.Labels != nil {
			if check.Metadata.Labels.Dash0Comdataset != nil {
				dataset = *check.Metadata.Labels.Dash0Comdataset
			}
			if check.Metadata.Labels.Dash0Comorigin != nil {
				origin = *check.Metadata.Labels.Dash0Comorigin
			}
		}
		fmt.Printf("Dataset: %s\n", dataset)
		fmt.Printf("Origin: %s\n", origin)
		if check.Metadata.Description != nil {
			fmt.Printf("Description: %s\n", *check.Metadata.Description)
		}
		return nil
	}
}
