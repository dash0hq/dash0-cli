package telemetrytransformationrules

import (
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
		Use: "update [origin-or-id] -f <file>", Short: "Replace an existing telemetry transformation rule",
		Long: `Replace a rule from YAML or JSON. Use '-f -' for stdin.
The target defaults to the file's origin, then ID. An explicit argument must
match the file's identifiers when present. Unknown targets are not created.

PUT replaces the complete rule: omitting examples removes stored examples.
Export with get before editing. A supplied updated-at token is preserved;
otherwise the preflight GET supplies it. Stale versions fail instead of overwriting.` + internal.CONFIG_HINT,
		Example: `  # Replace the rule identified by its file
  dash0 telemetry-transformation-rules update -f rule.yaml

  # Preview a replacement without writing
  dash0 telemetry-transformation-rules update <origin-or-id> -f rule.yaml --dry-run`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := asset.ReadRawInput(flags.File, os.Stdin)
			if err != nil {
				return err
			}
			rule, err := asset.ParseTelemetryTransformationRule(raw)
			if err != nil {
				return err
			}
			key, err := asset.ResolveUpdateKey(args, asset.UpdateKey{Noun: assetType, UsesOrigin: true, Origin: asset.GetTelemetryTransformationRuleOrigin(rule), ID: dash0api.GetTelemetryTransformationRuleID(rule)})
			if err != nil {
				return err
			}
			apiClient, err := client.NewClientFromContext(cmd.Context(), flags.ApiUrl, flags.AuthToken)
			if err != nil {
				return err
			}
			dataset := client.ResolveDataset(cmd.Context(), flags.Dataset)
			ectx := client.ErrorContext{AssetType: assetType, AssetID: key}
			before, err := apiClient.GetTelemetryTransformationRule(cmd.Context(), key, dataset)
			if err != nil {
				return client.HandleAPIError(err, ectx)
			}
			asset.PrepareTelemetryTransformationRule(rule, before, dataset)
			if flags.DryRun {
				return asset.PrintDiff(os.Stdout, displayKind, dash0api.GetTelemetryTransformationRuleName(rule), before, rule)
			}
			result, err := apiClient.UpdateTelemetryTransformationRule(cmd.Context(), key, rule, dataset)
			if err != nil {
				return client.HandleAPIError(err, ectx)
			}
			return asset.PrintDiff(os.Stdout, displayKind, dash0api.GetTelemetryTransformationRuleName(result), before, result)
		},
	}
	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}
