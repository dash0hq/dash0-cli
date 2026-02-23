package teams

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

type deleteFlags struct {
	ApiUrl    string
	AuthToken string
	Force     bool
}

func newDeleteCmd() *cobra.Command {
	flags := &deleteFlags{}

	cmd := &cobra.Command{
		Use:     "delete <id>",
		Aliases: []string{"remove"},
		Short:   "[experimental] Delete a team",
		Long:    `Delete a team by its ID or origin. Use --force to skip the confirmation prompt.` + internal.CONFIG_HINT,
		Example: `  # Delete with confirmation prompt
  dash0 --experimental teams delete <id>

  # Delete without confirmation (for scripts and automation)
  dash0 --experimental teams delete <id> --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runDelete(cmd, args[0], flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().BoolVar(&flags.Force, "force", false, "Skip confirmation prompt")

	return cmd
}

func runDelete(cmd *cobra.Command, originOrID string, flags *deleteFlags) error {
	ctx := cmd.Context()

	if !flags.Force {
		fmt.Printf("Are you sure you want to delete team %q? [y/N]: ", originOrID)
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

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	err = apiClient.DeleteTeam(ctx, originOrID)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "team",
			AssetID:   originOrID,
		})
	}

	fmt.Printf("Team %q deleted\n", originOrID)
	return nil
}
