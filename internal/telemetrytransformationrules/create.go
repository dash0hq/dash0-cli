package telemetrytransformationrules

import (
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
		Use: "create -f <file>", Aliases: []string{"add"}, Short: "Create a telemetry transformation rule",
		Long: "Create a rule from YAML or JSON. Use '-f -' for stdin.\nThis sends POST, not upsert: use apply to manage a rule by a fixed origin." + internal.CONFIG_HINT,
		Example: `  # Create a rule
  dash0 telemetry-transformation-rules create -f rule.yaml --dataset production

  # Validate without writing
  dash0 telemetry-transformation-rules create -f rule.yaml --dry-run

  # Read a rule from stdin
  cat rule.yaml | dash0 telemetry-transformation-rules create -f -`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := asset.ReadRawInput(flags.File, os.Stdin)
			if err != nil {
				return err
			}
			rule, err := asset.ParseTelemetryTransformationRule(raw)
			if err != nil {
				return err
			}
			if flags.DryRun {
				fmt.Println("Dry run: telemetry transformation rule definition is valid")
				return nil
			}
			apiClient, err := client.NewClientFromContext(cmd.Context(), flags.ApiUrl, flags.AuthToken)
			if err != nil {
				return err
			}
			dataset := client.ResolveDataset(cmd.Context(), flags.Dataset)
			asset.PrepareTelemetryTransformationRule(rule, nil, dataset)
			created, err := apiClient.CreateTelemetryTransformationRule(cmd.Context(), rule, dataset)
			if err != nil {
				return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType, AssetName: dash0api.GetTelemetryTransformationRuleName(rule)})
			}
			fmt.Printf("%s %q created (id: %s)\n", displayKind, dash0api.GetTelemetryTransformationRuleName(created), dash0api.GetTelemetryTransformationRuleID(created))
			return nil
		},
	}
	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}
