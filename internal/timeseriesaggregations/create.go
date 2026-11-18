package timeseriesaggregations

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

func newCreateCmd() *cobra.Command {
	var flags asset.FileInputFlags

	cmd := &cobra.Command{
		Use:     "create -f <file>",
		Aliases: []string{"add"},
		Short:   "Create a time series aggregation from a file",
		Long: `Create a time series aggregation from a YAML or JSON definition file. Use
'-f -' to read from stdin.

The CLI reads dash0.com/id first and updates the aggregation at the id when it
exists. Otherwise it writes by dash0.com/origin, or by an origin it infers from
the kind, metadata.name, and dataset, so repeated runs of the same file update
the same aggregation instead of creating duplicates.

Origins are unique per organization, while each aggregation belongs to exactly
one dataset. A document with an explicit origin therefore cannot be applied to
two datasets: use a distinct origin per dataset.` + internal.CONFIG_HINT,
		Example: `  # Create from a YAML file
  dash0 tsa create -f aggregation.yaml

  # Create from stdin
  cat aggregation.yaml | dash0 tsa create -f -

  # Validate without creating
  dash0 tsa create -f aggregation.yaml --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd.Context(), &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runCreate(ctx context.Context, flags *asset.FileInputFlags) error {
	raw, err := asset.ReadRawInput(flags.File, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read time series aggregation definition: %w", err)
	}

	aggregation, err := decode(raw)
	if err != nil {
		return err
	}

	if flags.DryRun {
		fmt.Println("Dry run: time series aggregation definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, err := asset.ImportTimeSeriesAggregation(ctx, apiClient, aggregation, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		if asset.IsTimeSeriesAggregationWrongDataset(err) {
			return err
		}
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: assetType,
			AssetName: dash0api.GetTimeSeriesAggregationName(aggregation),
		})
	}

	if result.ID != "" {
		fmt.Printf("%s %q %s (id: %s)\n", displayKind, result.Name, result.Action, result.ID)
	} else {
		fmt.Printf("%s %q %s\n", displayKind, result.Name, result.Action)
	}
	return nil
}
