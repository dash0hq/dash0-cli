package recordingrules

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
		Short: "Get a recording rule by ID",
		Long:  `Retrieve a recording rule definition by its ID.` + internal.CONFIG_HINT,
		Example: `  # Show recording rule summary
  dash0 recording-rules get <id>

  # Export as YAML (suitable for re-applying)
  dash0 recording-rules get <id> -o yaml > rule.yaml

  # Export as JSON
  dash0 recording-rules get <id> -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd.Context(), args[0], &flags)
		},
	}

	asset.RegisterGetFlags(cmd, &flags)
	return cmd
}

func runGet(ctx context.Context, id string, flags *asset.GetFlags) error {
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	rule, err := apiClient.GetRecordingRule(ctx, id, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "recording rule",
			AssetID:   id,
		})
	}

	dash0api.SetRecordingRuleIDIfAbsent(rule, id)

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout)

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return formatter.Print(rule)
	default:
		fmt.Printf("Kind: %s\n", rule.Kind)
		fmt.Printf("Name: %s\n", dash0api.GetRecordingRuleName(rule))
		fmt.Printf("Dataset: %s\n", dash0api.GetRecordingRuleDataset(rule))
		origin := ""
		if rule.Metadata.Labels != nil {
			origin = (*rule.Metadata.Labels)["dash0.com/origin"]
		}
		fmt.Printf("Origin: %s\n", origin)
		groupCount := len(rule.Spec.Groups)
		ruleCount := 0
		for _, g := range rule.Spec.Groups {
			ruleCount += len(g.Rules)
		}
		fmt.Printf("Groups: %d\n", groupCount)
		fmt.Printf("Rules: %d\n", ruleCount)
		return nil
	}
}
