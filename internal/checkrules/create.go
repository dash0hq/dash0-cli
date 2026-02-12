package checkrules

import (
	"context"
	"fmt"
	"os"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

func newCreateCmd() *cobra.Command {
	var flags asset.FileInputFlags

	cmd := &cobra.Command{
		Use:     "create -f <file>",
		Aliases: []string{"add"},
		Short:   "Create a check rule from a file",
		Long: `Create a new check rule from a YAML or JSON definition file. Use '-f -' to read from stdin.

Accepts both plain CheckRule definitions and PrometheusRule CRD files.
When a PrometheusRule CRD is provided, each alerting rule in the CRD is
created as a separate check rule (recording rules are skipped).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd.Context(), &flags)
		},
	}

	asset.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runCreate(ctx context.Context, flags *asset.FileInputFlags) error {
	data, err := asset.ReadRawInput(flags.File, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read check rule definition: %w", err)
	}

	kind := strings.ToLower(asset.DetectKind(data))
	if kind == "prometheusrule" {
		return createFromPrometheusRule(ctx, flags, data)
	}
	return createFromCheckRule(ctx, flags, data)
}

func createFromCheckRule(ctx context.Context, flags *asset.FileInputFlags, data []byte) error {
	var rule dash0api.PrometheusAlertRule
	if err := yaml.Unmarshal(data, &rule); err != nil {
		return fmt.Errorf("failed to read check rule definition: %w", err)
	}

	if flags.DryRun {
		fmt.Println("Dry run: check rule definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, importErr := asset.ImportCheckRule(ctx, apiClient, &rule, client.DatasetPtr(flags.Dataset))
	if importErr != nil {
		return client.HandleAPIError(importErr, client.ErrorContext{
			AssetType: "check rule",
			AssetName: rule.Name,
		})
	}

	fmt.Printf("Check rule %q %s successfully\n", result.Name, result.Action)
	return nil
}

func createFromPrometheusRule(ctx context.Context, flags *asset.FileInputFlags, data []byte) error {
	var promRule asset.PrometheusRule
	if err := yaml.Unmarshal(data, &promRule); err != nil {
		return fmt.Errorf("failed to read PrometheusRule definition: %w", err)
	}

	if flags.DryRun {
		fmt.Println("Dry run: PrometheusRule definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	results, importErr := asset.ImportPrometheusRule(ctx, apiClient, &promRule, client.DatasetPtr(flags.Dataset))
	for _, r := range results {
		fmt.Printf("Check rule %q %s successfully\n", r.Name, r.Action)
	}
	if importErr != nil {
		return client.HandleAPIError(importErr, client.ErrorContext{
			AssetType: "check rule",
		})
	}

	return nil
}
