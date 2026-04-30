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
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	var flags asset.GetFlags

	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "[experimental] Get a spam filter by ID",
		Long:  `Retrieve a spam filter definition by its ID.` + internal.CONFIG_HINT,
		Example: `  # Show spam filter summary
  dash0 --experimental spam-filters get <id>

  # Export as YAML (suitable for re-applying)
  dash0 --experimental spam-filters get <id> -o yaml > filter.yaml

  # Export as JSON
  dash0 --experimental spam-filters get <id> -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
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

	filter, err := apiClient.GetSpamFilter(ctx, id, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "spam filter",
			AssetID:   id,
		})
	}

	dash0api.SetSpamFilterIDIfAbsent(filter, id)

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout)

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return formatter.Print(filter)
	default:
		fmt.Printf("Kind: %s\n", filter.Kind)
		fmt.Printf("Name: %s\n", dash0api.GetSpamFilterName(filter))
		fmt.Printf("Dataset: %s\n", dash0api.GetSpamFilterDataset(filter))
		origin := ""
		if filter.Metadata.Labels != nil && filter.Metadata.Labels.Dash0Comorigin != nil {
			origin = *filter.Metadata.Labels.Dash0Comorigin
		}
		fmt.Printf("Origin: %s\n", origin)
		fmt.Printf("Contexts: %v\n", filter.Spec.Contexts)
		fmt.Printf("Filter criteria: %d\n", len(filter.Spec.Filter))
		return nil
	}
}
