package teams

import (
	"fmt"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/dash0hq/dash0-cli/internal/members"
	"github.com/spf13/cobra"
)

type addMembersFlags struct {
	ApiUrl    string
	AuthToken string
}

func newAddMembersCmd() *cobra.Command {
	flags := &addMembersFlags{}

	cmd := &cobra.Command{
		Use:   "add-members <team-id> <member-id-or-email> [<member-id-or-email>...]",
		Short: "[experimental] Add members to a team",
		Long: `Add one or more existing organization members to a team.
Members can be specified by member ID or email address.
Team and member IDs are base64 encoded strings prefixed with tea,_ and user_, respectively.
Any <member-id-or-email> argument not starting with "user_" is treated as an email address and resolved to a member ID automatically.` + internal.CONFIG_HINT,
		Example: `  # Add a single member to a team by ID
  dash0 --experimental teams add-members <team-id> <member-id>

  # Add a member by email address
  dash0 --experimental teams add-members <team-id> <member-id-or-email>

  # Add multiple members (mix of IDs and emails)
  dash0 --experimental teams add-members <team-id> <member-id-or-email> <member-id-or-email>`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runAddMembers(cmd, args[0], args[1:], flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")

	return cmd
}

func runAddMembers(cmd *cobra.Command, teamID string, memberIDs []string, flags *addMembersFlags) error {
	ctx := cmd.Context()

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	memberIDs, err = members.ResolveToMemberIDs(ctx, apiClient, memberIDs)
	if err != nil {
		return err
	}

	request := &dash0api.AddTeamMembersRequest{
		MemberIds: memberIDs,
	}

	err = apiClient.AddTeamMembers(ctx, teamID, request)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "team",
			AssetID:   teamID,
		})
	}

	if len(memberIDs) == 1 {
		fmt.Printf("1 member added to team %q\n", teamID)
	} else {
		fmt.Printf("%d members added to team %q\n", len(memberIDs), teamID)
	}
	return nil
}
