package spamfilters

import (
	"context"
	"fmt"
	"os"
	"strings"

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
		Long: `Retrieve a spam filter definition by its ID. The returned definition is in
whichever apiVersion (v1alpha1 or v1alpha2) the server has stored.` + internal.CONFIG_HINT,
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

	setIDIfAbsent(filter, id)

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout)

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return formatter.Print(filter)
	default:
		fmt.Printf("Kind: %s\n", objectKind(filter))
		fmt.Printf("API Version: %s\n", objectAPIVersion(filter))
		fmt.Printf("Name: %s\n", objectName(filter))
		fmt.Printf("Dataset: %s\n", objectDataset(filter))
		fmt.Printf("Origin: %s\n", objectOrigin(filter))
		switch v := filter.(type) {
		case *dash0api.SpamFilter:
			fmt.Printf("Contexts: [%s]\n", strings.Join(contextStrings(v.Spec.Contexts), ", "))
		case *dash0api.SpamFilterV1Alpha2:
			fmt.Printf("Context: %s\n", string(v.Spec.Context))
		}
		const filtersLabel = "Filters: "
		fmt.Printf("%s%s\n", filtersLabel, renderFiltersBlock(filter, len(filtersLabel)))
		return nil
	}
}

// setIDIfAbsent backfills the dash0.com/id label so the printed/exported
// definition is self-contained even when the server response omitted it.
// The api-client only ships a v1alpha1 setter, so we handle v1alpha2 inline.
func setIDIfAbsent(filter dash0api.SpamFilterObject, id string) {
	switch v := filter.(type) {
	case *dash0api.SpamFilter:
		dash0api.SetSpamFilterIDIfAbsent(v, id)
	case *dash0api.SpamFilterV1Alpha2:
		if v.Metadata.Labels == nil {
			v.Metadata.Labels = &dash0api.SpamFilterLabels{}
		}
		if v.Metadata.Labels.Dash0Comid == nil {
			value := id
			v.Metadata.Labels.Dash0Comid = &value
		}
	}
}

func contextStrings(values []dash0api.TelemetryFilterContext) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}
