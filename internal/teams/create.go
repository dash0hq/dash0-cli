package teams

import (
	"fmt"
	"regexp"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

type createFlags struct {
	ApiUrl    string
	AuthToken string
	ColorFrom string
	ColorTo   string
	Member    []string
}

func newCreateCmd() *cobra.Command {
	flags := &createFlags{}

	cmd := &cobra.Command{
		Use:     "create <name>",
		Aliases: []string{"add"},
		Short:   "[experimental] Create a team",
		Long:    `Create a new team in your Dash0 organization.` + internal.CONFIG_HINT,
		Example: `  # Create a team
  dash0 --experimental teams create "Backend Team"

  # Create a team with a custom color gradient
  dash0 --experimental teams create "Backend Team" \
      --color-from "#FF0000" --color-to "#00FF00"

  # Create a team with initial members
  dash0 --experimental teams create "Backend Team" \
      --member <member-id-1> --member <member-id-2>`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runCreate(cmd, args[0], flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVar(&flags.ColorFrom, "color-from", "", "Gradient start color (e.g. \"#FF0000\")")
	cmd.Flags().StringVar(&flags.ColorTo, "color-to", "", "Gradient end color (e.g. \"#00FF00\")")
	cmd.Flags().StringArrayVar(&flags.Member, "member", nil, "Member ID to add to the team (repeatable)")

	return cmd
}

func runCreate(cmd *cobra.Command, name string, flags *createFlags) error {
	ctx := cmd.Context()

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	members := flags.Member
	if members == nil {
		members = []string{}
	}

	team := &dash0api.TeamDefinition{
		Kind: dash0api.Dash0Team,
		Metadata: dash0api.TeamMetadata{
			Name: slugify(name),
		},
		Spec: dash0api.TeamSpec{
			Display: dash0api.TeamDisplay{
				Name:  name,
				Color: buildGradient(flags.ColorFrom, flags.ColorTo),
			},
			Members: members,
		},
	}

	created, err := apiClient.CreateTeam(ctx, team)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "team",
			AssetName: name,
		})
	}

	id := ""
	if created.Metadata.Labels != nil && created.Metadata.Labels.Dash0Comid != nil {
		id = *created.Metadata.Labels.Dash0Comid
	}
	if id != "" {
		fmt.Printf("Team %q created (id: %s)\n", name, id)
	} else {
		fmt.Printf("Team %q created\n", name)
	}
	return nil
}

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

// slugify converts a display name to a kebab-case metadata name.
func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonAlphanumeric.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// buildGradient returns a Gradient with the given colors.
// When both colors are empty, it returns sensible defaults so the API
// does not reject the request due to empty color strings.
func buildGradient(from, to string) dash0api.Gradient {
	if from == "" && to == "" {
		return dash0api.Gradient{
			From: "#6366F1",
			To:   "#8B5CF6",
		}
	}
	return dash0api.Gradient{
		From: from,
		To:   to,
	}
}
