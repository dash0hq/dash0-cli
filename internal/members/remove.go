package members

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

type removeFlags struct {
	ApiUrl    string
	AuthToken string
	Force     bool
}

func newRemoveCmd() *cobra.Command {
	flags := &removeFlags{}

	cmd := &cobra.Command{
		Use:     "remove <member-id-or-email> [<member-id-or-email>...]",
		Aliases: []string{"delete"},
		Short:   "[experimental] Remove members from the organization",
		Long: `Remove one or more members from your Dash0 organization.
Members can be specified by member ID or email address.
Member IDs are base64 encoded strings prefixed with user_.
Any argument not starting with "user_" is treated as an email address and resolved to a member ID automatically.` + internal.CONFIG_HINT,
		Example: `  # Remove a single member by ID
  dash0 --experimental members remove <member-id>

  # Remove a member by email address
  dash0 --experimental members remove <member-id-or-email> --force

  # Remove multiple members without confirmation
  dash0 --experimental members remove <member-id-or-email> <member-id-or-email> --force`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runRemove(cmd, args, flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().BoolVar(&flags.Force, "force", false, "Skip confirmation prompt")

	return cmd
}

func runRemove(cmd *cobra.Command, memberIDs []string, flags *removeFlags) error {
	ctx := cmd.Context()

	if !flags.Force {
		noun := "member"
		if len(memberIDs) > 1 {
			noun = "members"
		}
		fmt.Printf("Are you sure you want to remove %d %s? [y/N]:", len(memberIDs), noun)
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Removal cancelled")
			return nil
		}
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	resolved, err := ResolveMembers(ctx, apiClient, memberIDs)
	if err != nil {
		return err
	}

	for _, member := range resolved {
		err = apiClient.DeleteMember(ctx, member.ID)
		if err != nil {
			return client.HandleAPIError(err, client.ErrorContext{
				AssetType: "member",
				AssetID:   member.ID,
			})
		}
		fmt.Printf("Member %s removed\n", member.DisplayString())
	}
	return nil
}
