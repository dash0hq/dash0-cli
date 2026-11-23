package syntheticchecks

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
		Short: "Update a synthetic check from a file",
		Long: `Update an existing synthetic check from a YAML or JSON definition file. Use '-f -' to read from stdin.

If the ID argument is omitted, the file's dash0.com/id label is read first, then its dash0.com/origin label when the id is not found, as apply does.` + internal.CONFIG_HINT,
		Example: `  # Update a synthetic check from a file
  dash0 synthetic-checks update <id> -f check.yaml

  # Update using the ID from the file
  dash0 synthetic-checks update -f check.yaml

  # Export, edit, and update
  dash0 synthetic-checks get <id> -o yaml > check.yaml
  # edit check.yaml
  dash0 synthetic-checks update -f check.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args, &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *asset.FileInputFlags) error {
	var check dash0api.SyntheticCheckDefinition
	if err := asset.ReadDefinition(flags.File, &check, os.Stdin); err != nil {
		return fmt.Errorf("failed to read synthetic check definition: %w", err)
	}

	updateKey := asset.UpdateKey{Noun: "synthetic check", Upsert: asset.RoutedKeys(&check)}
	identity := asset.SyntheticCheckIdentity(&check, nil)
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
	get := func(ctx context.Context, key string) (*dash0api.SyntheticCheckDefinition, error) {
		return apiClient.GetSyntheticCheck(ctx, key, dataset)
	}
	id, before, err := asset.RouteUpdate(ctx, args, updateKey, identity, get)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "synthetic check",
			AssetID:   id,
		})
	}
	// An update never sends the origin, and an id that is not the path's key
	// can only disagree with it.
	dash0api.StripSyntheticCheckServerFields(&check)
	asset.KeepStoredOrigin(&check, before.GetOrigin(), id)
	if id != identity.ID {
		dash0api.ClearSyntheticCheckID(&check)
	}

	if flags.DryRun {
		return asset.PrintUpdateOutcome(os.Stdout, asset.UpdateOutcome{
			DisplayKind:     "Synthetic check",
			Name:            dash0api.GetSyntheticCheckName(&check),
			ID:              id,
			Before:          before,
			After:           &check,
			BeforeDeletedAt: asset.SyntheticCheckDeletedAt(before),
			DryRun:          true,
		})
	}

	result, err := apiClient.UpdateSyntheticCheck(ctx, id, &check, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "synthetic check",
			AssetID:   id,
			AssetName: dash0api.GetSyntheticCheckName(&check),
		})
	}

	return asset.PrintUpdateOutcome(os.Stdout, asset.UpdateOutcome{
		DisplayKind:     "Synthetic check",
		Name:            dash0api.GetSyntheticCheckName(result),
		ID:              id,
		Before:          before,
		After:           result,
		BeforeDeletedAt: asset.SyntheticCheckDeletedAt(before),
		AfterDeletedAt:  asset.SyntheticCheckDeletedAt(result),
	})
}
