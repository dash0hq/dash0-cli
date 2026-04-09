package checkrules

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
		Use:   "get <id>",
		Short: "Get a check rule by ID",
		Long: `Retrieve a check rule definition by its ID.` + internal.CONFIG_HINT,
		Example: `  # Show check rule summary
  dash0 check-rules get <id>

  # Export as YAML (suitable for re-applying)
  dash0 check-rules get <id> -o yaml > rule.yaml

  # Export as JSON
  dash0 check-rules get <id> -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd.Context(), args[0], &flags)
		},
	}

	asset.RegisterGetFlags(cmd, &flags)
	return cmd
}

func runGet(ctx context.Context, id string, flags *asset.GetFlags) error {
	apiUrl := client.ResolveApiUrl(ctx, flags.ApiUrl)
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	rule, err := apiClient.GetCheckRule(ctx, id, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "check rule",
			AssetID:   id,
		})
	}

	dash0api.SetCheckRuleIDIfAbsent(rule, id)

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout)

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return formatter.Print(rule)
	default:
		fmt.Printf("Name: %s\n", dash0api.GetCheckRuleName(rule))
		fmt.Printf("Dataset: %s\n", dash0api.GetCheckRuleDataset(rule))
		fmt.Printf("Expression: %s\n", rule.Expression)
		if rule.Enabled != nil {
			fmt.Printf("Enabled: %t\n", *rule.Enabled)
		}
		if rule.Annotations != nil && rule.Annotations.Description != nil {
			fmt.Printf("Description: %s\n", *rule.Annotations.Description)
		}
		if deeplinkURL := asset.DeeplinkURL(apiUrl, "checkrule", id); deeplinkURL != "" {
			fmt.Printf("URL: %s\n", deeplinkURL)
		}
		return nil
	}
}
