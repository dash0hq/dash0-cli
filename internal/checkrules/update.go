package checkrules

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
		Use:   "update [id] -f <file>",
		Short: "Update a check rule from a file",
		Long: `Update an existing check rule from a YAML or JSON definition file. Use '-f -' to read from stdin.

Accepts both plain CheckRule definitions and PrometheusRule CRD files. When a PrometheusRule CRD is provided, the CRD must contain exactly one alerting rule.

If the ID argument is omitted, the file's id is read first, then its dash0.com/origin label when the id is not found, as apply does.` + internal.CONFIG_HINT,
		Example: `  # Update a check rule from a file
  dash0 check-rules update <id> -f rule.yaml

  # Update using the ID from the file
  dash0 check-rules update -f rule.yaml

  # Update from a PrometheusRule CRD file
  dash0 check-rules update -f prometheus-rule.yaml

  # Export, edit, and update
  dash0 check-rules get <id> -o yaml > rule.yaml
  # edit rule.yaml
  dash0 check-rules update -f rule.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args, &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *asset.FileInputFlags) error {
	data, err := asset.ReadRawInputForWrite(flags.File, os.Stdin, "checkrule")
	if err != nil {
		return fmt.Errorf("failed to read check rule definition: %w", err)
	}

	rules, err := asset.ParseCheckRules(data)
	if err != nil {
		return err
	}

	if len(rules) > 1 {
		return fmt.Errorf("input contains %d check rules, but update requires exactly 1", len(rules))
	}
	rule := rules[0]

	updateKey := asset.UpdateKey{Noun: "check rule", Upsert: rule.UpdateKeys(), OriginLabel: `labels["dash0.com/origin"]`}
	identity := rule.Identity(nil)
	if err := asset.CheckRouteUpdate(args, updateKey, identity); err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	dataset := client.ResolveDataset(ctx, flags.Dataset)
	asset.ClearCheckRuleDeletedAt(rule.PrometheusAlertRule)

	// The PUT below creates a rule when nothing matches the key, so this read
	// is what makes update fail on a missing rule. It reads the file's id
	// first, then its origin, as apply does.
	get := func(ctx context.Context, key string) (*dash0api.PrometheusAlertRule, error) {
		return apiClient.GetCheckRule(ctx, key, dataset)
	}
	id, before, err := asset.RouteUpdate(ctx, args, updateKey, identity, get)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "check rule",
			AssetID:   id,
		})
	}
	// An update never sends the origin, and an id that is not the path's key
	// can only disagree with it.
	dash0api.StripCheckRuleServerFields(rule.PrometheusAlertRule)
	if id != identity.ID {
		dash0api.ClearCheckRuleID(rule.PrometheusAlertRule)
	}

	beforeDeletedAt := asset.CheckRuleDeletedAt(before)

	if flags.DryRun {
		return asset.PrintUpdateOutcome(os.Stdout, asset.UpdateOutcome{
			DisplayKind:     "Check rule",
			Name:            rule.Name,
			ID:              id,
			Before:          before,
			After:           rule.PrometheusAlertRule,
			BeforeDeletedAt: beforeDeletedAt,
			DryRun:          true,
		})
	}

	result, err := apiClient.UpdateCheckRule(ctx, id, rule.PrometheusAlertRule, dataset)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "check rule",
			AssetID:   id,
			AssetName: rule.Name,
		})
	}

	return asset.PrintUpdateOutcome(os.Stdout, asset.UpdateOutcome{
		DisplayKind:     "Check rule",
		Name:            result.Name,
		ID:              id,
		Before:          before,
		After:           result,
		BeforeDeletedAt: beforeDeletedAt,
		AfterDeletedAt:  asset.CheckRuleDeletedAt(result),
	})
}
