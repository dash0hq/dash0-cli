package syntheticchecks

import (
	"context"
	"fmt"
	"os"

	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/output"
	"github.com/dash0hq/dash0-cli/internal/resource"
	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	var flags resource.GetFlags

	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a synthetic check by ID",
		Long:  `Retrieve a synthetic check definition by its ID`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd.Context(), args[0], &flags)
		},
	}

	resource.RegisterGetFlags(cmd, &flags)
	return cmd
}

func runGet(ctx context.Context, id string, flags *resource.GetFlags) error {
	apiClient, err := client.NewClient(flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	check, err := apiClient.GetSyntheticCheck(ctx, id, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err)
	}

	format, err := output.ParseFormat(flags.Output)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(format, os.Stdout)

	switch format {
	case output.FormatJSON, output.FormatYAML:
		return formatter.Print(check)
	default:
		fmt.Printf("Kind: %s\n", check.Kind)
		fmt.Printf("Name: %s\n", check.Metadata.Name)
		if check.Metadata.Description != nil {
			fmt.Printf("Description: %s\n", *check.Metadata.Description)
		}
		return nil
	}
}
