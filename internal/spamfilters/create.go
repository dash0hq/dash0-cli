package spamfilters

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

func newCreateCmd() *cobra.Command {
	var flags asset.FileInputFlags

	cmd := &cobra.Command{
		Use:     "create -f <file>",
		Aliases: []string{"add"},
		Short:   "[experimental] Create a spam filter from a file",
		Long: `Create a new spam filter from a YAML or JSON definition file. Use '-f -' to
read from stdin.` + internal.CONFIG_HINT,
		Example: `  # Create from a YAML file
  dash0 --experimental spam-filters create -f filter.yaml

  # Create from stdin
  cat filter.yaml | dash0 --experimental spam-filters create -f -

  # Validate without creating
  dash0 --experimental spam-filters create -f filter.yaml --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runCreate(cmd.Context(), &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runCreate(ctx context.Context, flags *asset.FileInputFlags) error {
	raw, err := asset.ReadRawInput(flags.File, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read spam filter definition: %w", err)
	}

	apiVersion, err := detectAPIVersion(raw)
	if err != nil {
		return err
	}

	if flags.DryRun {
		fmt.Printf("Dry run: spam filter definition is valid (apiVersion: %s)\n", apiVersion)
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	switch apiVersion {
	case string(dash0api.V1alpha1):
		filter, err := decodeV1Alpha1(raw)
		if err != nil {
			return err
		}
		result, importErr := asset.ImportSpamFilter(ctx, apiClient, filter, dataset)
		if importErr != nil {
			return client.HandleAPIError(importErr, client.ErrorContext{
				AssetType: "spam filter",
				AssetName: dash0api.GetSpamFilterName(filter),
			})
		}
		printImportResult(result)
		return nil
	case string(dash0api.V1alpha2):
		filter, err := decodeV1Alpha2(raw)
		if err != nil {
			return err
		}
		result, importErr := asset.ImportSpamFilterV1Alpha2(ctx, apiClient, filter, dataset)
		if importErr != nil {
			return client.HandleAPIError(importErr, client.ErrorContext{
				AssetType: "spam filter",
				AssetName: filter.Metadata.Name,
			})
		}
		printImportResult(result)
		return nil
	default:
		// Unreachable: detectAPIVersion only returns supported values or an error.
		return fmt.Errorf("unsupported spam filter apiVersion %q (supported: %s)", apiVersion, joinQuoted(supportedAPIVersions))
	}
}

func printImportResult(result asset.ImportResult) {
	if result.ID != "" {
		fmt.Printf("Spam filter %q %s (id: %s).\n", result.Name, result.Action, result.ID)
	} else {
		fmt.Printf("Spam filter %q %s.\n", result.Name, result.Action)
	}
}
