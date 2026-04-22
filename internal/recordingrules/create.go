package recordingrules

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
		Short:   "Create a recording rule from a file",
		Long: `Create a new recording rule from a YAML or JSON definition file in PrometheusRule CRD format. Use '-f -' to read from stdin.` + internal.CONFIG_HINT,
		Example: `  # Create from a YAML file
  dash0 recording-rules create -f rule.yaml

  # Create from stdin
  cat rule.yaml | dash0 recording-rules create -f -

  # Validate without creating
  dash0 recording-rules create -f rule.yaml --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd.Context(), &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runCreate(ctx context.Context, flags *asset.FileInputFlags) error {
	var rule dash0api.RecordingRule
	if err := asset.ReadDefinition(flags.File, &rule, os.Stdin); err != nil {
		return fmt.Errorf("failed to read recording rule definition: %w", err)
	}

	if flags.DryRun {
		fmt.Println("Dry run: recording rule definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, importErr := asset.ImportRecordingRule(ctx, apiClient, &rule, client.ResolveDataset(ctx, flags.Dataset))
	if importErr != nil {
		return client.HandleAPIError(importErr, client.ErrorContext{
			AssetType: "recording rule",
			AssetName: dash0api.GetRecordingRuleName(&rule),
		})
	}

	fmt.Printf("Recording rule %q %s\n", result.Name, result.Action)
	return nil
}
