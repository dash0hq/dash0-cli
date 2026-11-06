package telemetrytransformationrules

import (
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	var flags asset.GetFlags
	cmd := &cobra.Command{
		Use: "get <origin-or-id>", Short: "Get a complete telemetry transformation rule",
		Long: "Retrieve a rule by origin or ID, including all stored examples." + internal.CONFIG_HINT,
		Example: `  # Export a complete rule for editing
  dash0 telemetry-transformation-rules get <origin-or-id> -o yaml > rule.yaml

  # Show a rule summary
  dash0 telemetry-transformation-rules get <origin-or-id>`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := output.ParseFormat(flags.Output)
			if err != nil {
				return err
			}
			apiClient, err := client.NewClientFromContext(cmd.Context(), flags.ApiUrl, flags.AuthToken)
			if err != nil {
				return err
			}
			rule, err := apiClient.GetTelemetryTransformationRule(cmd.Context(), args[0], client.ResolveDataset(cmd.Context(), flags.Dataset))
			if err != nil {
				return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType, AssetID: args[0]})
			}
			if format == output.FormatJSON || format == output.FormatYAML {
				return output.NewFormatter(format, os.Stdout).Print(rule)
			}
			fmt.Printf("Kind: %s\nName: %s\nID: %s\nDataset: %s\nOrigin: %s\nSignal: %s\nContext: %s\nEnabled: %t\n",
				displayKind, dash0api.GetTelemetryTransformationRuleName(rule), dash0api.GetTelemetryTransformationRuleID(rule),
				dash0api.GetTelemetryTransformationRuleDataset(rule), asset.GetTelemetryTransformationRuleOrigin(rule), rule.Spec.Signal, rule.Spec.Context, rule.Spec.Enabled)
			priority := 200
			if rule.Spec.Priority != nil {
				priority = *rule.Spec.Priority
			}
			fmt.Printf("Priority: %d\nTransformations: %d\n", priority, len(rule.Spec.Transformations))
			return nil
		},
	}
	asset.RegisterGetFlags(cmd, &flags)
	return cmd
}
