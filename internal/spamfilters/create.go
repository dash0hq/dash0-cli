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
		Long:    `Create a new spam filter from a YAML or JSON definition file. Use '-f -' to read from stdin.` + internal.CONFIG_HINT,
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
	var filter dash0api.SpamFilter
	if err := asset.ReadDefinition(flags.File, &filter, os.Stdin); err != nil {
		return fmt.Errorf("failed to read spam filter definition: %w", err)
	}

	if flags.DryRun {
		fmt.Println("Dry run: spam filter definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, importErr := asset.ImportSpamFilter(ctx, apiClient, &filter, client.ResolveDataset(ctx, flags.Dataset))
	if importErr != nil {
		return client.HandleAPIError(importErr, client.ErrorContext{
			AssetType: "spam filter",
			AssetName: dash0api.GetSpamFilterName(&filter),
		})
	}

	if result.ID != "" {
		fmt.Printf("Spam filter %q %s (id: %s).\n", result.Name, result.Action, result.ID)
	} else {
		fmt.Printf("Spam filter %q %s.\n", result.Name, result.Action)
	}
	return nil
}
