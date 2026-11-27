package views

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

func newUpdateCmd() *cobra.Command {
	var flags asset.FileInputFlags

	cmd := &cobra.Command{
		Use:   "update [id] -f <file>",
		Short: "Update a view from a file",
		Long: `Update an existing view from a YAML or JSON definition file. Use '-f -' to read from stdin.

If the ID argument is omitted, the file's dash0.com/id label is read first, then its dash0.com/origin label when the id is not found, as apply does.` + internal.CONFIG_HINT,
		Example: `  # Update a view from a file
  dash0 views update <id> -f view.yaml

  # Update using the ID from the file
  dash0 views update -f view.yaml

  # Export, edit, and update
  dash0 views get <id> -o yaml > view.yaml
  # edit view.yaml
  dash0 views update -f view.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args, &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *asset.FileInputFlags) error {
	var view dash0api.ViewDefinition
	if err := asset.ReadDefinitionForWrite(flags.File, &view, os.Stdin, "view"); err != nil {
		return fmt.Errorf("failed to read view definition: %w", err)
	}

	updateKey := asset.UpdateKey{Noun: "view", Upsert: asset.RoutedKeys(&view)}
	identity := asset.ViewIdentity(&view, nil)
	if err := asset.CheckRouteUpdate(args, updateKey, identity); err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	// The PUT below creates the asset when nothing matches the key, so this
	// read is what makes update fail on a missing one. It reads the file's id
	// first, then its origin, as apply does.
	get := func(ctx context.Context, key string) (*dash0api.ViewDefinition, error) {
		return apiClient.GetView(ctx, key, dataset)
	}
	id, before, err := asset.RouteUpdate(ctx, args, updateKey, identity, get)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "view",
			AssetID:   id,
		})
	}
	// An update never sends the origin, and an id that is not the path's key
	// can only disagree with it.
	dash0api.StripViewServerFields(&view)
	asset.SetViewOrigin(&view, asset.UpdateOrigin(before.GetOrigin(), id))
	if id != identity.ID {
		dash0api.ClearViewID(&view)
	}

	if flags.DryRun {
		return asset.PrintUpdateOutcome(os.Stdout, asset.UpdateOutcome{
			DisplayKind:     "View",
			Name:            dash0api.GetViewName(&view),
			ID:              id,
			Before:          before,
			After:           &view,
			BeforeDeletedAt: asset.ViewDeletedAt(before),
			DryRun:          true,
		})
	}

	result, err := apiClient.UpdateView(ctx, id, &view, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "view",
			AssetID:   id,
			AssetName: dash0api.GetViewName(&view),
		})
	}

	return asset.PrintUpdateOutcome(os.Stdout, asset.UpdateOutcome{
		DisplayKind:     "View",
		Name:            dash0api.GetViewName(result),
		ID:              id,
		Before:          before,
		After:           result,
		BeforeDeletedAt: asset.ViewDeletedAt(before),
		AfterDeletedAt:  asset.ViewDeletedAt(result),
	})
}
