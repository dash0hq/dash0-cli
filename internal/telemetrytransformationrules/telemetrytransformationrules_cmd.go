package telemetrytransformationrules

import "github.com/spf13/cobra"

const (
	assetType   = "telemetry transformation rule"
	displayKind = "Telemetry transformation rule"
)

func NewTelemetryTransformationRulesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "telemetry-transformation-rules",
		Short: "Manage Dash0 telemetry transformation rules",
		Long:  "Create, list, get, replace, and delete dataset-scoped telemetry transformation rules.\nUse get to export a complete rule: list omits stored examples.",
		Example: `  # List rules in a dataset
  dash0 telemetry-transformation-rules list --dataset production

  # Export a complete rule, including its examples
  dash0 telemetry-transformation-rules get <origin-or-id> -o yaml`,
	}
	cmd.AddCommand(newListCmd(), newGetCmd(), newCreateCmd(), newUpdateCmd(), newDeleteCmd())
	return cmd
}
