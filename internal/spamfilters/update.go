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

func newUpdateCmd() *cobra.Command {
	var flags asset.FileInputFlags

	cmd := &cobra.Command{
		Use:   "update [id] -f <file>",
		Short: "[experimental] Update a spam filter from a file",
		Long: `Update an existing spam filter from a YAML or JSON definition file. Use '-f -' to read from stdin.

If the ID argument is omitted, the ID is extracted from the file content.` + internal.CONFIG_HINT,
		Example: `  # Update a spam filter from a file
  dash0 --experimental spam-filters update <id> -f filter.yaml

  # Update using the ID from the file
  dash0 --experimental spam-filters update -f filter.yaml

  # Export, edit, and update
  dash0 --experimental spam-filters get <id> -o yaml > filter.yaml
  # edit filter.yaml
  dash0 --experimental spam-filters update -f filter.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runUpdate(cmd.Context(), args, &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *asset.FileInputFlags) error {
	var filter dash0api.SpamFilter
	if err := asset.ReadDefinition(flags.File, &filter, os.Stdin); err != nil {
		return fmt.Errorf("failed to read spam filter definition: %w", err)
	}

	var id string
	fileID := dash0api.GetSpamFilterID(&filter)
	if len(args) == 1 {
		id = args[0]
		if fileID != "" && fileID != id {
			return fmt.Errorf("the ID argument %q does not match the ID in the file %q", id, fileID)
		}
	} else {
		id = fileID
		if id == "" {
			return fmt.Errorf("no spam filter ID provided as argument, and the file does not contain an ID")
		}
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	before, err := apiClient.GetSpamFilter(ctx, id, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "spam filter",
			AssetID:   id,
		})
	}

	if flags.DryRun {
		return asset.PrintDiff(os.Stdout, "Spam filter", dash0api.GetSpamFilterName(&filter), before, &filter)
	}

	result, err := apiClient.UpdateSpamFilter(ctx, id, &filter, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "spam filter",
			AssetID:   id,
			AssetName: dash0api.GetSpamFilterName(&filter),
		})
	}

	return asset.PrintDiff(os.Stdout, "Spam filter", dash0api.GetSpamFilterName(result), before, result)
}
