package members

import (
	"fmt"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

type inviteFlags struct {
	ApiUrl    string
	AuthToken string
	Role      string
}

func newInviteCmd() *cobra.Command {
	flags := &inviteFlags{}

	cmd := &cobra.Command{
		Use:     "invite <email> [<email>...]",
		Aliases: []string{"add"},
		Short:   "[experimental] Invite members to the organization",
		Long:    `Invite one or more members to your Dash0 organization by email address.` + internal.CONFIG_HINT,
		Example: `  # Invite a single member (default role: basic_member)
  dash0 --experimental members invite user@example.com

  # Invite multiple members as admins
  dash0 --experimental members invite user1@example.com user2@example.com --role admin`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runInvite(cmd, args, flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVar(&flags.Role, "role", "basic_member", "Role to assign to invited members: basic_member, admin")

	return cmd
}

var validRoles = map[string]bool{
	"basic_member": true,
	"admin":        true,
}

func runInvite(cmd *cobra.Command, emails []string, flags *inviteFlags) error {
	ctx := cmd.Context()

	if !validRoles[flags.Role] {
		return fmt.Errorf("unknown role %q (valid roles: basic_member, admin)", flags.Role)
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	for _, email := range emails {
		request := &dash0api.InviteMemberRequest{
			EmailAddress: email,
			Role:         flags.Role,
		}
		err = apiClient.InviteMember(ctx, request)
		if err != nil {
			return client.HandleAPIError(err, client.ErrorContext{
				AssetType: "member",
				AssetName: email,
			})
		}
	}

	if len(emails) == 1 {
		fmt.Printf("Invitation sent to %s\n", emails[0])
	} else {
		fmt.Printf("Invitations sent to %d email addresses\n", len(emails))
	}
	return nil
}
