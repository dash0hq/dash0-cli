package automerge

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
	"github.com/spf13/cobra"
	sigsyaml "sigs.k8s.io/yaml"
)

type getFlags struct {
	connectionFlags
	Output string
}

func newGetCmd() *cobra.Command {
	flags := &getFlags{}

	cmd := &cobra.Command{
		Use:   "get <owner/repo | repository-id>",
		Short: "[experimental] Get the AutoMerge settings of a repository",
		Long: `Retrieve the AutoMerge settings of a repository by its 'owner/repo' slug or its id.
A repository that was never configured shows the defaults.

The YAML output can be edited and passed back to 'update -f'. It carries the
settings version, so the update is rejected if someone changed the settings in
between.` + internal.CONFIG_HINT,
		Example: `  # Show a summary
  dash0 --experimental automerge settings get acme/api

  # Export as YAML, edit, and apply
  dash0 --experimental automerge settings get acme/api -o yaml > automerge.yaml
  dash0 --experimental automerge settings update -f automerge.yaml

  # Export as JSON
  dash0 --experimental automerge settings get acme/api -o json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runGet(cmd.Context(), parseRepositoryRef(args[0]), flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table, json, yaml (default: table; json in agent mode)")

	return cmd
}

func runGet(ctx context.Context, ref repositoryRef, flags *getFlags) error {
	format, err := parseGetFormat(flags.Output)
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}
	repositoryID, err := resolveRepositoryID(ctx, apiClient, ref)
	if err != nil {
		return err
	}
	settings, err := apiClient.GetDarkplaneRepositorySettings(ctx, repositoryID)
	if err != nil {
		return handleError(err, ref)
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(settings)
	case "yaml":
		data, err := sigsyaml.Marshal(settings)
		if err != nil {
			return fmt.Errorf("failed to marshal AutoMerge settings as YAML: %w", err)
		}
		fmt.Print(string(data))
		return nil
	default:
		printSummary(settings)
		return nil
	}
}

func parseGetFormat(s string) (string, error) {
	switch strings.ToLower(s) {
	case "":
		if agentmode.Enabled {
			return "json", nil
		}
		return "table", nil
	case "table", "json":
		return strings.ToLower(s), nil
	case "yaml", "yml":
		return "yaml", nil
	default:
		return "", fmt.Errorf("unknown output format: %s (valid formats: table, json, yaml)", s)
	}
}

func printSummary(settings *dash0api.RepositorySettingsDefinition) {
	spec := settings.Spec
	fmt.Printf("Repository: %s\n", settings.Metadata.Name)
	fmt.Printf("Repository ID: %s\n", dash0api.GetDarkplaneRepositorySettingsID(settings))
	if version := dash0api.GetDarkplaneRepositorySettingsVersion(settings); version != "" {
		fmt.Printf("Version: %s\n", version)
	}
	fmt.Printf("Bot mode: %s\n", spec.BotMode)
	fmt.Printf("Trigger: %s\n", spec.TriggerMode)
	fmt.Printf("Wait for checks: %t\n", spec.WaitForChecks)
	fmt.Printf("Score threshold: %g\n", spec.SoftThreshold)

	criteria := "Criteria:"
	if settings.UsesDefaultCriteria != nil && *settings.UsesDefaultCriteria {
		criteria = "Criteria (defaults):"
	}
	fmt.Println(criteria)
	for _, criterion := range spec.SoftRiskCriteria {
		floor := "no floor"
		if criterion.Threshold != nil {
			floor = fmt.Sprintf("floor %g", *criterion.Threshold)
		}
		fmt.Printf("  %s (weight %g, %s)\n", criterion.Name, criterion.Multiplier, floor)
	}

	if spec.PathRules == nil || len(*spec.PathRules) == 0 {
		fmt.Println("Path rules: none")
	} else {
		fmt.Println("Path rules:")
		for _, rule := range *spec.PathRules {
			fmt.Printf("  %s: %s%s\n", rule.Level, strings.Join(rule.Paths, ", "), pathRuleScope(rule))
		}
	}

	reviewer := "disabled"
	if spec.Reviewer != nil && spec.Reviewer.Enabled {
		reviewer = "enabled"
	}
	fmt.Printf("Reviewer: %s\n", reviewer)
	if updatedAt := settings.Metadata.Annotations.Dash0ComupdatedAt; updatedAt != nil {
		fmt.Printf("Updated: %s\n", updatedAt.Format("2006-01-02 15:04:05"))
	}
}

func pathRuleScope(rule dash0api.RepositoryPathRule) string {
	if rule.Authors == nil || len(*rule.Authors) == 0 {
		return ""
	}
	if len(*rule.Authors) == 1 {
		return " (1 author)"
	}
	return fmt.Sprintf(" (%d authors)", len(*rule.Authors))
}
