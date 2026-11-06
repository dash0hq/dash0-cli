package telemetrytransformationrules

import (
	"fmt"

	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	var flags asset.DeleteFlags
	cmd := &cobra.Command{
		Use: "delete <origin-or-id>", Aliases: []string{"remove"}, Short: "Delete a telemetry transformation rule",
		Long: "Delete a rule by origin or ID. Use --force to skip confirmation and accept an absent rule." + internal.CONFIG_HINT,
		Example: `  # Delete with confirmation
  dash0 telemetry-transformation-rules delete <origin-or-id>

  # Delete without prompting, including an already absent rule
  dash0 telemetry-transformation-rules delete <origin-or-id> --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			confirmed, err := confirmation.ConfirmDestructiveOperation(cmd.Context(), fmt.Sprintf("Delete telemetry transformation rule %q? [y/N]: ", args[0]), flags.Force)
			if err != nil {
				return err
			}
			if !confirmed {
				fmt.Println("Deletion cancelled")
				return nil
			}
			apiClient, err := client.NewClientFromContext(cmd.Context(), flags.ApiUrl, flags.AuthToken)
			if err != nil {
				return err
			}
			ectx := client.ErrorContext{AssetType: assetType, AssetID: args[0]}
			if err := apiClient.DeleteTelemetryTransformationRule(cmd.Context(), args[0], client.ResolveDataset(cmd.Context(), flags.Dataset)); err != nil {
				if client.IsAlreadyDeleted(err, flags.Force, ectx) {
					return nil
				}
				return client.HandleAPIError(err, ectx)
			}
			fmt.Printf("%s %q deleted\n", displayKind, args[0])
			return nil
		},
	}
	asset.RegisterDeleteFlags(cmd, &flags)
	return cmd
}
