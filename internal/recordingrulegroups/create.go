package recordingrulegroups

import (
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/spf13/cobra"
	sigsyaml "sigs.k8s.io/yaml"
)

func newCreateCmd() *cobra.Command {
	var flags asset.FileInputFlags

	cmd := &cobra.Command{
		Use:     "create -f <file>",
		Aliases: []string{"add"},
		Short: "Create a recording rule group from a file",
		Long:  `Create a new recording rule group from a YAML or JSON definition file. Use '-f -' to read from stdin.` + internal.CONFIG_HINT,
		Example: `  # Create from a YAML file
  dash0 recording-rule-groups create -f group.yaml

  # Create from stdin
  cat group.yaml | dash0 recording-rule-groups create -f -

  # Validate without creating
  dash0 recording-rule-groups create -f group.yaml --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd.Context(), &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runCreate(ctx context.Context, flags *asset.FileInputFlags) error {
	var group dash0api.RecordingRuleGroupDefinition
	if err := asset.ReadDefinition(flags.File, &group, os.Stdin); err != nil {
		return fmt.Errorf("failed to read recording rule group definition: %w", err)
	}

	if flags.DryRun {
		// Validate that it's valid YAML by marshaling
		if _, err := sigsyaml.Marshal(&group); err != nil {
			return fmt.Errorf("recording rule group definition is not valid: %w", err)
		}
		fmt.Println("Dry run: recording rule group definition is valid")
		return nil
	}

	asset.StripRecordingRuleGroupServerFields(&group)
	asset.InjectRecordingRuleGroupDataset(&group, client.ResolveDataset(ctx, flags.Dataset))

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, err := apiClient.CreateRecordingRuleGroup(ctx, &group)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "recording rule group",
			AssetName: asset.ExtractRecordingRuleGroupName(&group),
		})
	}

	fmt.Printf("Recording rule group %q created\n", asset.ExtractRecordingRuleGroupName(result))
	return nil
}
