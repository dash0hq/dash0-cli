package checkrules

import (
	"context"
	"fmt"

	dash0 "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/client"
	res "github.com/dash0hq/dash0-cli/internal/resource"
	"github.com/spf13/cobra"
)

func newApplyCmd() *cobra.Command {
	var flags res.FileInputFlags

	cmd := &cobra.Command{
		Use:   "apply -f <file>",
		Short: "Apply a check rule definition (create or update)",
		Long: `Apply a check rule definition from a YAML or JSON file.
If the check rule exists, it will be updated. If it doesn't exist, it will be created.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApply(cmd.Context(), &flags)
		},
	}

	res.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runApply(ctx context.Context, flags *res.FileInputFlags) error {
	var rule dash0.PrometheusAlertRule
	if err := res.ReadDefinitionFile(flags.File, &rule); err != nil {
		return fmt.Errorf("failed to read check rule definition: %w", err)
	}

	if flags.DryRun {
		fmt.Println("Dry run: check rule definition is valid")
		return nil
	}

	apiClient, err := client.NewClient(flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, err := apiClient.ImportCheckRule(ctx, &rule, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err)
	}

	fmt.Printf("Check rule %q applied successfully\n", result.Name)
	return nil
}
