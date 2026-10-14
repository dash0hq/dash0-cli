package datasets

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

type updateFlags struct {
	ApiUrl    string
	AuthToken string
	File      string
	DryRun    bool
}

func newUpdateCmd() *cobra.Command {
	flags := &updateFlags{}

	cmd := &cobra.Command{
		Use:   "update [id] -f <file>",
		Short: "[experimental] Update a dataset from a file",
		Long: `Update an existing dataset from a YAML or JSON definition file. Use '-f -' to read from stdin.

If the ID argument is omitted, the slug is extracted from the file content.
The dataset must exist; use create to make a new one.` + internal.CONFIG_HINT,
		Example: `  # Update a dataset from a file
  dash0 --experimental datasets update <id> -f dataset.yaml

  # Update using the slug from the file
  dash0 --experimental datasets update -f dataset.yaml

  # Preview the changes
  dash0 --experimental datasets update -f dataset.yaml --dry-run

  # Export, edit, and update
  dash0 --experimental datasets get <id> -o yaml > dataset.yaml
  # edit dataset.yaml
  dash0 --experimental datasets update -f dataset.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runUpdate(cmd.Context(), args, flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Path to YAML or JSON definition file (use '-' for stdin)")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Show the diff without updating")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *updateFlags) error {
	var dataset dash0api.DatasetDefinition
	if err := asset.ReadDefinition(flags.File, &dataset, os.Stdin); err != nil {
		return fmt.Errorf("failed to read dataset definition: %w", err)
	}

	if err := asset.ValidateDatasetAPIVersion(&dataset); err != nil {
		return err
	}

	id, err := asset.ResolveUpdateKey(args, asset.UpdateKey{
		Noun: "dataset",
		ID:   dash0api.GetDatasetID(&dataset),
	})
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	// The server upserts on PUT for most tokens, so a mistyped id would create a dataset.
	// The GET turns that into a not-found error.
	before, err := apiClient.GetDataset(ctx, id)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: "dataset", AssetID: id})
	}

	dash0api.StripDatasetServerFields(&dataset)

	if flags.DryRun {
		// A spec field left out of the file keeps its stored value, so the preview
		// merges the file onto the stored dataset instead of showing omissions as removals.
		preview, err := previewUpdate(before, &dataset)
		if err != nil {
			return err
		}
		return asset.PrintDiff(os.Stdout, "Dataset", dataset.Metadata.Name, before, preview)
	}

	result, err := apiClient.UpdateDataset(ctx, id, &dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "dataset",
			AssetID:   id,
			AssetName: dash0api.GetDatasetName(&dataset),
		})
	}

	return asset.PrintDiff(os.Stdout, "Dataset", dash0api.GetDatasetName(result), before, result)
}

// previewUpdate returns the dataset as it would look after update: the file's values win,
// and anything the file leaves out keeps its stored value.
func previewUpdate(stored, update *dash0api.DatasetDefinition) (*dash0api.DatasetDefinition, error) {
	base, err := toMap(stored)
	if err != nil {
		return nil, err
	}
	over, err := toMap(update)
	if err != nil {
		return nil, err
	}
	var merged dash0api.DatasetDefinition
	data, err := json.Marshal(mergeMaps(base, over))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &merged); err != nil {
		return nil, fmt.Errorf("failed to build the update preview: %w", err)
	}
	return &merged, nil
}

func toMap(d *dash0api.DatasetDefinition) (map[string]any, error) {
	data, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// mergeMaps overlays over onto base: nested objects merge, everything else is replaced.
func mergeMaps(base, over map[string]any) map[string]any {
	for k, v := range over {
		if vm, ok := v.(map[string]any); ok {
			if bm, ok := base[k].(map[string]any); ok {
				base[k] = mergeMaps(bm, vm)
				continue
			}
		}
		base[k] = v
	}
	return base
}
