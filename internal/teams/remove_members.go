package teams

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/dash0hq/dash0-cli/internal/members"
	"github.com/spf13/cobra"
)

type removeMembersFlags struct {
	ApiUrl    string
	AuthToken string
	Force     bool
}

func newRemoveMembersCmd() *cobra.Command {
	flags := &removeMembersFlags{}

	cmd := &cobra.Command{
		Use:     "remove-members <team-id> <member-id-or-email> [<member-id-or-email>...]",
		Aliases: []string{"delete-members"},
		Short:   "[experimental] Remove members from a team",
		Long: `Remove one or more members from a team.
Members can be specified by member ID or email address.
Team and member IDs are base64 encoded strings prefixed with tea,_ and user_, respectively.
Any <member-id-or-email> argument not starting with "user_" is treated as an email address and resolved to a member ID automatically.` + internal.CONFIG_HINT,
		Example: `  # Remove a single member from a team by ID
  dash0 --experimental teams remove-members <team-id> <member-id>

  # Remove a member by email address
  dash0 --experimental teams remove-members <team-id> <member-id-or-email> --force

  # Remove multiple members without confirmation
  dash0 --experimental teams remove-members <team-id> <member-id-or-email> <member-id-or-email> --force`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runRemoveMembers(cmd, args[0], args[1:], flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().BoolVar(&flags.Force, "force", false, "Skip confirmation prompt")

	return cmd
}

func runRemoveMembers(cmd *cobra.Command, teamID string, memberIDs []string, flags *removeMembersFlags) error {
	ctx := cmd.Context()

	if !flags.Force {
		noun := "member"
		if len(memberIDs) > 1 {
			noun = "members"
		}
		fmt.Printf("Are you sure you want to remove %d %s from team %q? [y/N]: ", len(memberIDs), noun, teamID)
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

	resolved, err := members.ResolveMembers(ctx, apiClient, memberIDs)
	if err != nil {
		return err
	}

	for _, member := range resolved {
		err = apiClient.RemoveTeamMember(ctx, teamID, member.ID)
		if err != nil {
			return client.HandleAPIError(err, client.ErrorContext{
				AssetType: "team",
				AssetID:   teamID,
			})
		}
		fmt.Printf("Member %s removed from team %q\n", member.DisplayString(), teamID)
	}
	return nil
}
