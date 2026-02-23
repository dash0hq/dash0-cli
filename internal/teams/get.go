package teams

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
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

	resp, err := apiClient.GetTeam(ctx, originOrID)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "team",
			AssetID:   originOrID,
		})
	}

	switch strings.ToLower(flags.Output) {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(resp)
	case "yaml", "yml":
		data, err := sigsyaml.Marshal(resp)
		if err != nil {
			return fmt.Errorf("failed to marshal team as YAML: %w", err)
		}
		fmt.Print(string(data))
		return nil
	case "table", "":
		return printTeamDetails(resp)
	default:
		return fmt.Errorf("unknown output format: %s (valid formats: table, json, yaml)", flags.Output)
	}
}

func printTeamDetails(resp *dash0api.GetTeamResponse) error {
	team := resp.Team

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
	if id != "" {
		fmt.Printf("ID:      %s\n", id)
	}
	if origin != "" {
		fmt.Printf("Origin:  %s\n", origin)
	}
	fmt.Printf("Color:   %s -> %s\n", team.Spec.Display.Color.From, team.Spec.Display.Color.To)
	fmt.Printf("Members: %d\n", len(resp.Members))

	if len(resp.Members) > 0 {
		fmt.Println()
		fmt.Println("Team members:")
		for _, m := range resp.Members {
			name := members.MemberDisplayName(&m)
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

	printAccessibleAssets("Dashboards", resp.Dashboards)
	printAccessibleAssets("Check rules", resp.CheckRules)
	printAccessibleAssets("Views", resp.Views)
	printAccessibleAssets("Synthetic checks", resp.SyntheticChecks)
	printAccessibleAssets("Datasets", resp.Datasets)

	return nil
}

func printAccessibleAssets(label string, assets []dash0api.AccessibleAsset) {
	if len(assets) == 0 {
		return
	}
	fmt.Println()
	fmt.Printf("Accessible %s:\n", label)
	for _, a := range assets {
		fmt.Printf("  - %s\n", a.Name)
	}
}
