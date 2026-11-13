package samplingrules

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
		Short: "Get a sampling rule by origin or ID",
		Long: `Retrieve a sampling rule definition by its 'dash0.com/origin' label or its
ID.` + internal.CONFIG_HINT,
		Example: `  # Show the rule summary
  dash0 sampling-rules get <origin-or-id>

  # Export as YAML (suitable for re-applying)
  dash0 sampling-rules get <origin-or-id> -o yaml > sampling-rule.yaml

  # Export as JSON
  dash0 sampling-rules get <origin-or-id> -o json`,
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

	rule, err := apiClient.GetSamplingRule(ctx, originOrID, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		if asset.IsSamplingRuleWrongDataset(err) {
			return asset.WrapSamplingRuleWrongDataset(err, originOrID)
		}
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: assetType,
			AssetID:   originOrID,
		})
	}

	// A 200 whose body the client cannot decode as JSON yields a nil rule and a
	// nil error, and printSummary reads rule.Spec directly.
	if rule == nil {
		return fmt.Errorf("the API returned no sampling rule for %q", originOrID)
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

// printSummary prints the rule's key fields. Sampling rules carry no created
// or updated timestamps, so there are no Created and Updated lines.
func printSummary(rule *dash0api.SamplingDefinition) {
	fmt.Printf("Kind: %s\n", displayKind)
	fmt.Printf("Name: %s\n", dash0api.GetSamplingRuleName(rule))
	fmt.Printf("ID: %s\n", dash0api.GetSamplingRuleID(rule))
	fmt.Printf("Dataset: %s\n", dash0api.GetSamplingRuleDataset(rule))
	fmt.Printf("Origin: %s\n", origin(rule))
	fmt.Printf("Enabled: %t\n", rule.Spec.Enabled)
	fmt.Printf("Conditions: %s\n", asset.DescribeSamplingRuleConditions(rule))
	if limit := rateLimit(rule); limit != "" {
		fmt.Printf("Rate limit: %s traces per minute\n", limit)
	}
}
