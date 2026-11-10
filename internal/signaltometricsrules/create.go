package signaltometricsrules

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

func newCreateCmd() *cobra.Command {
	var flags asset.FileInputFlags

	cmd := &cobra.Command{
		Use:     "create -f <file>",
		Aliases: []string{"add"},
		Short:   "Create a signal-to-metrics rule from a file",
		Long: `Create a signal-to-metrics rule from a YAML or JSON definition file. Use
'-f -' to read from stdin.

When the document carries a 'dash0.com/origin' label under metadata.labels,
the rule is upserted by that origin: repeated runs of the same file update the
same rule instead of creating duplicates.

With only a 'dash0.com/id' label, the rule with that ID is updated. When no
rule has that ID, for example in a definition exported from another
organization, a new rule is created with an ID assigned by the server, so
every further run creates another one. With neither label, every run creates
a new rule.

Origins are unique per organization, and each rule belongs to one dataset.
Applying a document whose origin already exists in another dataset fails
before anything is written, so use a distinct origin per dataset.` + internal.CONFIG_HINT,
		Example: `  # Create from a YAML file
  dash0 s2m create -f rule.yaml

  # Create from stdin
  cat rule.yaml | dash0 s2m create -f -

  # Validate without creating
  dash0 s2m create -f rule.yaml --dry-run

  # Check how many signals the rule would match before creating it
  dash0 s2m test -f rule.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd.Context(), &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runCreate(ctx context.Context, flags *asset.FileInputFlags) error {
	raw, err := asset.ReadRawInput(flags.File, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read signal-to-metrics rule definition: %w", err)
	}

	rule, err := decode(raw)
	if err != nil {
		return err
	}

	if flags.DryRun {
		fmt.Println("Dry run: signal-to-metrics rule definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, err := asset.ImportSignalToMetricsRule(ctx, apiClient, rule, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		if asset.IsSignalToMetricsRuleWrongDataset(err) {
			return err
		}
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: assetType,
			AssetName: dash0api.GetSignalToMetricsName(rule),
		})
	}

	if result.ID != "" {
		fmt.Printf("%s %q %s (id: %s)\n", displayKind, result.Name, result.Action, result.ID)
	} else {
		fmt.Printf("%s %q %s\n", displayKind, result.Name, result.Action)
	}
	return nil
}
