package syntheticchecks

import (
	"context"
	"fmt"
	"os"

	dash0 "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/client"
	res "github.com/dash0hq/dash0-cli/internal/resource"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var flags res.FileInputFlags

	cmd := &cobra.Command{
		Use:   "create -f <file>",
		Short: "Create a synthetic check from a file",
		Long:  `Create a new synthetic check from a YAML or JSON definition file. Use '-f -' to read from stdin.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(cmd.Context(), &flags)
		},
	}

	res.RegisterFileInputFlags(cmd, &flags)
	return cmd
}

func runCreate(ctx context.Context, flags *res.FileInputFlags) error {
	var check dash0.SyntheticCheckDefinition
	if err := res.ReadDefinition(flags.File, &check, os.Stdin); err != nil {
		return fmt.Errorf("failed to read synthetic check definition: %w", err)
	}

	// Set origin to dash0-cli
	if check.Metadata.Labels == nil {
		check.Metadata.Labels = &dash0.SyntheticCheckLabels{}
	}
	origin := "dash0-cli"
	check.Metadata.Labels.Dash0Comorigin = &origin

	if flags.DryRun {
		fmt.Println("Dry run: synthetic check definition is valid")
		return nil
	}

	apiClient, err := client.NewClient(flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, err := apiClient.CreateSyntheticCheck(ctx, &check, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err)
	}

	fmt.Printf("Synthetic check %q created successfully\n", result.Metadata.Name)
	return nil
}
