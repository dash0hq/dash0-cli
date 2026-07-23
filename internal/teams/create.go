package teams

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

type createFlags struct {
	ApiUrl    string
	AuthToken string
	File      string
	DryRun    bool
	ColorFrom string
	ColorTo   string
	Member    []string
}

func newCreateCmd() *cobra.Command {
	flags := &createFlags{}

	cmd := &cobra.Command{
		Use:     "create [<name>]",
		Aliases: []string{"add"},
		Short:   "[experimental] Create or upsert a team",
		Long: `Create a new team in your Dash0 organization.

Two modes are supported:

- Declarative: -f <file> reads a TeamDefinitionV1Alpha1 YAML/JSON document.
  If the document carries a dash0.com/origin label, the team is created or
  replaced via PUT (idempotent). Otherwise it is created via POST and the
  server assigns id and origin.
- Imperative: create <name> [--color-from ...] [--color-to ...] [--member ...]
  posts a minimal envelope built from flags. --member accepts an email or an
  internal member id; the server resolves emails.` + internal.CONFIG_HINT,
		Example: `  # Declarative from a YAML file
  dash0 --experimental teams create -f team.yaml

  # Declarative from stdin
  cat team.yaml | dash0 --experimental teams create -f -

  # Imperative: create a team with flags
  dash0 --experimental teams create "Backend Team"

  # Imperative with a color gradient and initial members
  dash0 --experimental teams create "Backend Team" \
      --color-from "#FF0000" --color-to "#00FF00" \
      --member alice@example.com --member bob@example.com`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runCreate(cmd.Context(), args, flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Path to YAML or JSON definition file (use '-' for stdin)")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Validate without creating/updating (declarative mode only)")
	cmd.Flags().StringVar(&flags.ColorFrom, "color-from", "", "Gradient start color (e.g. \"#FF0000\") (imperative mode)")
	cmd.Flags().StringVar(&flags.ColorTo, "color-to", "", "Gradient end color (e.g. \"#00FF00\") (imperative mode)")
	cmd.Flags().StringArrayVar(&flags.Member, "member", nil, "Member email or ID to add to the team (imperative mode; repeatable)")

	return cmd
}

func runCreate(ctx context.Context, args []string, flags *createFlags) error {
	if flags.File != "" {
		if len(args) > 0 {
			return fmt.Errorf("cannot combine -f/--file with a positional <name> argument")
		}
		if flags.ColorFrom != "" || flags.ColorTo != "" || len(flags.Member) > 0 {
			return fmt.Errorf("cannot combine -f/--file with --color-from, --color-to, or --member")
		}
		return runCreateFromFile(ctx, flags)
	}
	if len(args) == 0 {
		return fmt.Errorf("either -f/--file or a positional <name> argument is required")
	}
	if flags.DryRun {
		return fmt.Errorf("--dry-run is only valid with -f/--file")
	}
	return runCreateImperative(ctx, args[0], flags)
}

func runCreateFromFile(ctx context.Context, flags *createFlags) error {
	var team dash0api.TeamDefinitionV1Alpha1
	if err := asset.ReadDefinition(flags.File, &team, os.Stdin); err != nil {
		return fmt.Errorf("failed to read team definition: %w", err)
	}

	if flags.DryRun {
		fmt.Println("Dry run: team definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	result, err := asset.ImportTeam(ctx, apiClient, &team)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "team",
			AssetName: dash0api.GetTeamDisplayName(&team),
		})
	}

	name := result.Name
	if name == "" {
		name = dash0api.GetTeamDisplayName(&team)
	}
	if result.ID != "" {
		fmt.Printf("Team %q %s (id: %s).\n", name, result.Action, result.ID)
	} else {
		fmt.Printf("Team %q %s.\n", name, result.Action)
	}
	return nil
}

func runCreateImperative(ctx context.Context, name string, flags *createFlags) error {
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	members := flags.Member
	if members == nil {
		members = []string{}
	}

	team := &dash0api.TeamDefinitionV1Alpha1{
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

	id := dash0api.GetTeamID(created)
	if id != "" {
		fmt.Printf("Team %q created (id: %s).\n", name, id)
	} else {
		fmt.Printf("Team %q created.\n", name)
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
