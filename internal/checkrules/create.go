package checkrules

import (
	"context"
	"fmt"
	"os"

	dash0 "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/client"
	res "github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var flags res.FileInputFlags

	cmd := &cobra.Command{
		Use:     "create -f <file>",
		Aliases: []string{"add"},
		Short:   "Create a check rule from a file",
		Long:    `Create a new check rule from a YAML or JSON definition file. Use '-f -' to read from stdin.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd.Context(), &flags)
		},
	}

	res.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runCreate(ctx context.Context, flags *res.FileInputFlags) error {
	var rule dash0.PrometheusAlertRule
	if err := res.ReadDefinition(flags.File, &rule, os.Stdin); err != nil {
		return fmt.Errorf("failed to read check rule definition: %w", err)
	}

	// Set origin to dash0-cli
	if rule.Labels == nil {
		labels := make(map[string]string)
		rule.Labels = &labels
	}
	(*rule.Labels)["dash0.com/origin"] = "dash0-cli"

	if flags.DryRun {
		fmt.Println("Dry run: check rule definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, err := apiClient.CreateCheckRule(ctx, &rule, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "check rule",
			AssetName: rule.Name,
		})
	}

	fmt.Printf("Check rule %q created successfully\n", result.Name)
	return nil
}
