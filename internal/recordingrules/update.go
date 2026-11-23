package recordingrules

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
		Short: "Update a recording rule from a file",
		Long: `Update an existing recording rule from a YAML or JSON definition file in PrometheusRule CRD format. Use '-f -' to read from stdin.

If the ID argument is omitted, the file's dash0.com/id label is read first, then its dash0.com/origin label when the id is not found, as apply does.` + internal.CONFIG_HINT,
		Example: `  # Update a recording rule from a file
  dash0 recording-rules update <id> -f rule.yaml

  # Update using the ID from the file
  dash0 recording-rules update -f rule.yaml

  # Export, edit, and update
  dash0 recording-rules get <id> -o yaml > rule.yaml
  # edit rule.yaml
  dash0 recording-rules update -f rule.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args, &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *asset.FileInputFlags) error {
	var rule dash0api.RecordingRule
	if err := asset.ReadDefinition(flags.File, &rule, os.Stdin); err != nil {
		return fmt.Errorf("failed to read recording rule definition: %w", err)
	}

	updateKey := asset.UpdateKey{Noun: "recording rule", Upsert: asset.RoutedKeys(&rule)}
	identity := asset.RecordingRuleIdentity(&rule, nil)
	if err := asset.CheckRouteUpdate(args, updateKey, identity); err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)
	asset.ClearRecordingRuleDeletedAt(&rule)

	// The PUT below creates the asset when nothing matches the key, so this
	// read is what makes update fail on a missing one. It reads the file's id
	// first, then its origin, as apply does.
	get := func(ctx context.Context, key string) (*dash0api.RecordingRule, error) {
		return apiClient.GetRecordingRule(ctx, key, dataset)
	}
	id, before, err := asset.RouteUpdate(ctx, args, updateKey, identity, get)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "recording rule",
			AssetID:   id,
		})
	}
	// An update never sends the origin, and an id that is not the path's key
	// can only disagree with it.
	dash0api.StripRecordingRuleServerFields(&rule)
	asset.KeepStoredOrigin(&rule, before.GetOrigin(), id)
	asset.CarryRecordingRuleVersion(before, &rule)

	if flags.DryRun {
		return asset.PrintUpdateOutcome(os.Stdout, asset.UpdateOutcome{
			DisplayKind:     "Recording rule",
			Name:            dash0api.GetRecordingRuleName(&rule),
			ID:              id,
			Before:          before,
			After:           &rule,
			BeforeDeletedAt: asset.RecordingRuleDeletedAt(before),
			DryRun:          true,
		})
	}

	result, err := apiClient.UpdateRecordingRule(ctx, id, &rule, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "recording rule",
			AssetID:   id,
			AssetName: dash0api.GetRecordingRuleName(&rule),
		})
	}

	return asset.PrintUpdateOutcome(os.Stdout, asset.UpdateOutcome{
		DisplayKind:     "Recording rule",
		Name:            dash0api.GetRecordingRuleName(result),
		ID:              id,
		Before:          before,
		After:           result,
		BeforeDeletedAt: asset.RecordingRuleDeletedAt(before),
		AfterDeletedAt:  asset.RecordingRuleDeletedAt(result),
	})
}
