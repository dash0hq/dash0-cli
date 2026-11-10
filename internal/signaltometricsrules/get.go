package signaltometricsrules

import (
	"context"
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
		Use:   "get <origin-or-id>",
		Short: "Get a signal-to-metrics rule by origin or ID",
		Long: `Retrieve a signal-to-metrics rule definition by its 'dash0.com/origin' label
or its ID.` + internal.CONFIG_HINT,
		Example: `  # Show the rule summary
  dash0 s2m get <origin-or-id>

  # Export as YAML (suitable for re-applying)
  dash0 s2m get <origin-or-id> -o yaml > rule.yaml

  # Export as JSON
  dash0 s2m get <origin-or-id> -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd.Context(), args[0], &flags)
		},
	}

	asset.RegisterGetFlags(cmd, &flags)
	return cmd
}

func runGet(ctx context.Context, originOrID string, flags *asset.GetFlags) error {
	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	rule, err := apiClient.GetSignalToMetrics(ctx, originOrID, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: assetType,
			AssetID:   originOrID,
		})
	}

	// A 200 whose body the client cannot decode as JSON yields a nil rule and a
	// nil error, and printSummary reads rule.Spec directly.
	if rule == nil {
		return fmt.Errorf("the API returned no signal-to-metrics rule for %q", originOrID)
	}

	formatter := output.NewFormatter(format, os.Stdout)

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return formatter.Print(rule)
	default:
		printSummary(rule)
		return nil
	}
}

func printSummary(rule *dash0api.SignalToMetricsDefinition) {
	fmt.Printf("Kind: %s\n", displayKind)
	fmt.Printf("Name: %s\n", dash0api.GetSignalToMetricsName(rule))
	fmt.Printf("ID: %s\n", dash0api.GetSignalToMetricsID(rule))
	fmt.Printf("Dataset: %s\n", dash0api.GetSignalToMetricsDataset(rule))
	if deletedAt := asset.SignalToMetricsRuleDeletedAt(rule); deletedAt != nil {
		fmt.Printf("Deleted: %s\n", deletedAt.UTC().Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("Origin: %s\n", origin(rule))
	fmt.Printf("Signal: %s\n", signal(rule))
	fmt.Printf("Metric: %s\n", metricName(rule))
	if rule.Spec.Output.Type != nil {
		fmt.Printf("Metric type: %s\n", string(*rule.Spec.Output.Type))
	}
	fmt.Printf("Interval: %s\n", string(rule.Spec.Output.Interval))
	fmt.Printf("Enabled: %t\n", rule.Spec.Enabled)
	if rule.Spec.TargetDatasetMode != nil {
		fmt.Printf("Target dataset mode: %s\n", string(*rule.Spec.TargetDatasetMode))
	}
	if rule.Spec.AlternativeDatasetId != nil {
		fmt.Printf("Alternative dataset: %s\n", *rule.Spec.AlternativeDatasetId)
	}
	if rule.Metadata.Annotations != nil {
		if createdAt := rule.Metadata.Annotations.Dash0ComcreatedAt; createdAt != nil {
			fmt.Printf("Created: %s\n", createdAt.Format("2006-01-02 15:04:05"))
		}
		if updatedAt := rule.Metadata.Annotations.Dash0ComupdatedAt; updatedAt != nil {
			fmt.Printf("Updated: %s\n", updatedAt.Format("2006-01-02 15:04:05"))
		}
	}
}
