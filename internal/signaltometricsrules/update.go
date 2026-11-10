package signaltometricsrules

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
		Use:   "update [origin-or-id] -f <file>",
		Short: "Update a signal-to-metrics rule from a file",
		Long: `Update an existing signal-to-metrics rule from a YAML or JSON definition
file. Use '-f -' to read from stdin.

When the positional argument is omitted, the target is taken from the
document's 'dash0.com/origin' label, or else its 'dash0.com/id' label. When it
is given, it must match one of them. The rule must already exist: unlike
'create' and 'apply', this command never creates one.

The output is a unified diff of the before and after states.` + internal.CONFIG_HINT,
		Example: `  # Update using the origin or ID from the file
  dash0 s2m update -f rule.yaml

  # Update by explicit origin or ID, which must match the file
  dash0 s2m update <origin-or-id> -f rule.yaml

  # Preview the diff without applying
  dash0 s2m update -f rule.yaml --dry-run

  # Export, edit, and update
  dash0 s2m get <origin-or-id> -o yaml > rule.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args, &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *asset.FileInputFlags) error {
	raw, err := asset.ReadRawInput(flags.File, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read signal-to-metrics rule definition: %w", err)
	}

	rule, err := decode(raw)
	if err != nil {
		return err
	}

	key, err := asset.ResolveUpdateKey(args, asset.UpdateKey{
		Noun:       assetType,
		UsesOrigin: true,
		Origin:     origin(rule),
		ID:         dash0api.GetSignalToMetricsID(rule),
	})
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	// The PUT below creates a rule when nothing matches the key, so this GET is
	// what makes update fail on a missing rule instead of creating one.
	before, err := apiClient.GetSignalToMetrics(ctx, key, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: assetType,
			AssetID:   key,
		})
	}

	if flags.DryRun {
		return asset.PrintUpdateOutcome(os.Stdout, asset.UpdateOutcome{
			DisplayKind:     displayKind,
			Name:            dash0api.GetSignalToMetricsName(rule),
			ID:              key,
			Before:          before,
			After:           rule,
			BeforeDeletedAt: asset.SignalToMetricsRuleDeletedAt(before),
			DryRun:          true,
		})
	}

	// The server takes the target from the URL, and the dataset from the
	// --dataset query parameter, matching the import path. When the key is the
	// origin, an id in the body can only disagree with the rule the path
	// selects, so it is not sent either.
	if key != dash0api.GetSignalToMetricsID(rule) {
		dash0api.ClearSignalToMetricsID(rule)
	}
	dash0api.StripSignalToMetricsServerFields(rule)

	result, err := apiClient.UpdateSignalToMetrics(ctx, key, rule, dataset)
	if err != nil {
		if asset.IsSignalToMetricsRuleWrongDataset(err) {
			return asset.WrapSignalToMetricsRuleWrongDataset(err, key)
		}
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: assetType,
			AssetID:   key,
			AssetName: dash0api.GetSignalToMetricsName(rule),
		})
	}

	return asset.PrintUpdateOutcome(os.Stdout, asset.UpdateOutcome{
		DisplayKind:     displayKind,
		Name:            dash0api.GetSignalToMetricsName(result),
		ID:              key,
		Before:          before,
		After:           result,
		BeforeDeletedAt: asset.SignalToMetricsRuleDeletedAt(before),
		AfterDeletedAt:  asset.SignalToMetricsRuleDeletedAt(result),
	})
}
