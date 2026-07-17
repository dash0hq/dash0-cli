package teams

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/dash0hq/dash0-cli/internal/members"
	"github.com/spf13/cobra"
	sigsyaml "sigs.k8s.io/yaml"
)

type getFlags struct {
	ApiUrl    string
	AuthToken string
	Output    string
}

func newGetCmd() *cobra.Command {
	flags := &getFlags{}

	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "[experimental] Get team details",
		Long:  `Get detailed information about a team, including its members and accessible assets.` + internal.CONFIG_HINT,
		Example: `  # Get team details
  dash0 --experimental teams get <id>

  # Output as JSON
  dash0 --experimental teams get <id> -o json

  # Output as YAML
  dash0 --experimental teams get <id> -o yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runGet(cmd, args[0], flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table, json, yaml (default: table)")

	return cmd
}

func runGet(cmd *cobra.Command, originOrID string, flags *getFlags) error {
	ctx := cmd.Context()

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	team, err := apiClient.GetTeam(ctx, originOrID)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "team",
			AssetID:   originOrID,
		})
	}

	switch strings.ToLower(flags.Output) {
	case "json":
		if err := dash0api.ResolveTeamMembersToEmails(ctx, apiClient, team); err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(team)
	case "yaml", "yml":
		if err := dash0api.ResolveTeamMembersToEmails(ctx, apiClient, team); err != nil {
			return err
		}
		data, err := sigsyaml.Marshal(team)
		if err != nil {
			return fmt.Errorf("failed to marshal team as YAML: %w", err)
		}
		fmt.Print(string(data))
		return nil
	case "":
		if agentmode.Enabled {
			if err := dash0api.ResolveTeamMembersToEmails(ctx, apiClient, team); err != nil {
				return err
			}
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(team)
		}
		return printTeamDetails(ctx, apiClient, team)
	case "table":
		return printTeamDetails(ctx, apiClient, team)
	default:
		return fmt.Errorf("unknown output format: %s (valid formats: table, json, yaml)", flags.Output)
	}
}

func printTeamDetails(ctx context.Context, apiClient dash0api.Client, team *dash0api.TeamDefinitionV1Alpha1) error {
	id := ""
	if team.Metadata.Labels != nil && team.Metadata.Labels.Dash0Comid != nil {
		id = *team.Metadata.Labels.Dash0Comid
	}
	origin := ""
	if team.Metadata.Labels != nil && team.Metadata.Labels.Dash0Comorigin != nil {
		origin = *team.Metadata.Labels.Dash0Comorigin
	}

	fmt.Printf("Kind:    Team\n")
	fmt.Printf("Name:    %s\n", team.Spec.Display.Name)
	fmt.Printf("Slug:    %s\n", team.Metadata.Name)
	if id != "" {
		fmt.Printf("ID:      %s\n", id)
	}
	if origin != "" {
		fmt.Printf("Origin:  %s\n", origin)
	}
	fmt.Printf("Color:   %s -> %s\n", team.Spec.Display.Color.From, team.Spec.Display.Color.To)
	fmt.Printf("Members: %d\n", len(team.Spec.Members))

	teamMembers, err := resolveTeamMembers(ctx, apiClient, team)
	if err != nil {
		return err
	}
	if len(teamMembers) > 0 {
		fmt.Println()
		fmt.Println("Team members:")
		for _, m := range teamMembers {
			name := members.MemberDisplayName(m)
			email := ""
			if m.Spec.Display.Email != nil {
				email = *m.Spec.Display.Email
			}
			if email != "" {
				fmt.Printf("  - %s (%s)\n", name, email)
			} else {
				fmt.Printf("  - %s\n", name)
			}
		}
	}

	return nil
}
