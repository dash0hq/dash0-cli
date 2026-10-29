package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
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
		Short: "[experimental] Get an Agent0 custom skill by ID or origin",
		Long:  `Retrieve an Agent0 custom skill, including its files and permissions, by its ID or origin.` + internal.CONFIG_HINT,
		Example: `  # Get skill details
  dash0 --experimental agent0 skills get <id>

  # Output as JSON
  dash0 --experimental agent0 skills get <id> -o json

  # Export as YAML, ready for apply
  dash0 --experimental agent0 skills get <id> -o yaml > skill.yaml`,
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

	format, err := parseGetFormat(flags.Output)
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	skill, err := apiClient.GetCustomSkill(ctx, originOrID)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType, AssetID: originOrID})
	}
	asset.SortCustomSkillPermissions(skill)

	switch format {
	case getFormatJSON:
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(skill)
	case getFormatYAML:
		data, err := sigsyaml.Marshal(skill)
		if err != nil {
			return fmt.Errorf("failed to marshal Agent0 skill as YAML: %w", err)
		}
		fmt.Print(string(data))
		return nil
	default:
		printDetails(skill)
		return nil
	}
}

type getFormat string

const (
	getFormatTable getFormat = "table"
	getFormatJSON  getFormat = "json"
	getFormatYAML  getFormat = "yaml"
)

func parseGetFormat(s string) (getFormat, error) {
	switch strings.ToLower(s) {
	case "":
		if agentmode.Enabled {
			return getFormatJSON, nil
		}
		return getFormatTable, nil
	case "table":
		return getFormatTable, nil
	case "json":
		return getFormatJSON, nil
	case "yaml", "yml":
		return getFormatYAML, nil
	default:
		return "", fmt.Errorf("unknown output format: %s (valid formats: table, json, yaml)", s)
	}
}

func printDetails(skill *dash0api.SkillDefinition) {
	fmt.Printf("Kind:    %s\n", skill.Kind)
	fmt.Printf("Name:    %s\n", asset.CustomSkillName(skill))
	if id := dash0api.GetCustomSkillID(skill); id != "" {
		fmt.Printf("ID:      %s\n", id)
	}
	if origin := dash0api.GetCustomSkillOrigin(skill); origin != "" {
		fmt.Printf("Origin:  %s\n", origin)
	}
	if enabled := enabledValue(skill); enabled != "-" {
		fmt.Printf("Enabled: %s\n", enabled)
	}
	if source := skill.Spec.Source; source != nil {
		fmt.Printf("Source:  %s (%s)\n", source.RepositoryUrl, source.Path)
	}
	fmt.Println("Files:")
	for _, file := range skill.Spec.Files {
		fmt.Printf("  %s\n", file.RelativePath)
	}
}
