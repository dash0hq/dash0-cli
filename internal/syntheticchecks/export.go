package syntheticchecks

import (
	"context"
	"fmt"

	"github.com/dash0hq/dash0-cli/internal/client"
	res "github.com/dash0hq/dash0-cli/internal/resource"
	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	var flags res.ExportFlags

	cmd := &cobra.Command{
		Use:   "export <id>",
		Short: "Export a synthetic check to a file",
		Long:  `Export a synthetic check definition to a YAML or JSON file`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(cmd.Context(), args[0], &flags)
		},
	}

	res.RegisterExportFlags(cmd, &flags)
	return cmd
}

func runExport(ctx context.Context, id string, flags *res.ExportFlags) error {
	apiClient, err := client.NewClient(flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	check, err := apiClient.GetSyntheticCheck(ctx, id, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err)
	}

	if flags.File != "" {
		if err := res.WriteDefinitionFile(flags.File, check); err != nil {
			return fmt.Errorf("failed to write synthetic check to file: %w", err)
		}
		fmt.Printf("Synthetic check exported to %s\n", flags.File)
	} else {
		format := "yaml"
		if flags.Output == "json" {
			format = "json"
		}
		if err := res.WriteToStdout(format, check); err != nil {
			return fmt.Errorf("failed to write synthetic check: %w", err)
		}
	}

	return nil
}
