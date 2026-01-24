package checkrules

import (
	"context"
	"fmt"

	dash0 "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/client"
	res "github.com/dash0hq/dash0-cli/internal/resource"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var flags res.FileInputFlags

	cmd := &cobra.Command{
		Use:   "update <id> -f <file>",
		Short: "Update a check rule from a file",
		Long:  `Update an existing check rule from a YAML or JSON definition file`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context(), args[0], &flags)
		},
	}

	res.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runUpdate(ctx context.Context, id string, flags *res.FileInputFlags) error {
	var rule dash0.PrometheusAlertRule
	if err := res.ReadDefinitionFile(flags.File, &rule); err != nil {
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

	apiClient, err := client.NewClient(flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, err := apiClient.UpdateCheckRule(ctx, id, &rule, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err)
	}

	fmt.Printf("Check rule %q updated successfully\n", result.Name)
	return nil
}
