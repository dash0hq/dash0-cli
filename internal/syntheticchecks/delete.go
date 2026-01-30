package syntheticchecks

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	var flags asset.DeleteFlags

	cmd := &cobra.Command{
		Use:     "delete <id>",
		Aliases: []string{"remove"},
		Short:   "Delete a synthetic check",
		Long:    `Delete a synthetic check by its ID`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(cmd.Context(), args[0], &flags)
		},
	}

	asset.RegisterDeleteFlags(cmd, &flags)
	return cmd
}

func runDelete(ctx context.Context, id string, flags *asset.DeleteFlags) error {
	if !flags.Force {
		fmt.Printf("Are you sure you want to delete synthetic check %q? [y/N]: ", id)
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Deletion cancelled")
			return nil
		}
	}

	apiClient, err := client.NewClient(flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	err = apiClient.DeleteSyntheticCheck(ctx, id, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err)
	}

	fmt.Printf("Synthetic check %q deleted successfully\n", id)
	return nil
}
