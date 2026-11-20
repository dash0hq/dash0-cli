package samplingrules

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
		Short:   "Create a sampling rule from a file",
		Long: `Create a sampling rule from a YAML or JSON definition file. Use '-f -' to
read from stdin.

The CLI reads dash0.com/id first and updates the rule at the id when it
exists. Otherwise it writes by dash0.com/origin, or by an origin it infers from
the kind, metadata.name, and dataset, so repeated runs of the same file update
the same rule instead of creating duplicates. A rule is never created by POST.

Origins are unique per organization, and each rule belongs to one dataset.
Applying a document whose origin already exists in another dataset fails
before anything is written, so use a distinct origin per dataset.

The document must set spec.enabled. When the new rule is the first enabled
rule in the dataset, it turns sampling on, and the CLI prints a warning.` + internal.CONFIG_HINT,
		Example: `  # Create from a YAML file
  dash0 sampling-rules create -f sampling-rule.yaml

  # Create from stdin
  cat sampling-rule.yaml | dash0 sampling-rules create -f -

  # Validate without creating
  dash0 sampling-rules create -f sampling-rule.yaml --dry-run`,
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
		return fmt.Errorf("failed to read sampling rule definition: %w", err)
	}

	rule, err := asset.ParseSamplingRule(raw)
	if err != nil {
		return err
	}

	if flags.DryRun {
		fmt.Println("Dry run: sampling rule definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, err := asset.ImportSamplingRule(ctx, apiClient, rule, client.ResolveDataset(ctx, flags.Dataset))
	if err != nil {
		if asset.IsSamplingRuleWrongDataset(err) {
			return err
		}
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: assetType,
			AssetName: dash0api.GetSamplingRuleName(rule),
		})
	}

	printWarnings(os.Stderr, result.Warnings)
	if result.ID != "" {
		fmt.Printf("%s %q %s (id: %s)\n", displayKind, result.Name, result.Action, result.ID)
	} else {
		fmt.Printf("%s %q %s\n", displayKind, result.Name, result.Action)
	}
	return nil
}
