package skills

import (
	"context"
	"fmt"
	"os"

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
}

func newCreateCmd() *cobra.Command {
	flags := &createFlags{}

	cmd := &cobra.Command{
		Use:     "create -f <file>",
		Aliases: []string{"add"},
		Short:   "[experimental] Create an Agent0 custom skill from a file",
		Long: `Create an Agent0 custom skill from a YAML or JSON definition file. Use '-f -' to read from stdin.

The skill's name comes from the name field in the frontmatter of its SKILL.md file, and is unique per organization.
If the file sets metadata.labels["dash0.com/origin"] and a skill with that origin exists, it is updated.
spec.source and the per-user metadata.annotations["dash0.com/enabled"] are never sent; use spec.permissions[].defaultEnabled to ship a skill turned off.` + internal.CONFIG_HINT,
		Example: `  # Create from a YAML file
  dash0 --experimental agent0 skills create -f skill.yaml

  # Create from stdin
  cat skill.yaml | dash0 --experimental agent0 skills create -f -

  # Validate without creating
  dash0 --experimental agent0 skills create -f skill.yaml --dry-run`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runCreate(cmd.Context(), flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Path to YAML or JSON definition file (use '-' for stdin)")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Validate without creating/updating")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

func runCreate(ctx context.Context, flags *createFlags) error {
	var skill dash0api.SkillDefinition
	if err := asset.ReadDefinition(flags.File, &skill, os.Stdin); err != nil {
		return fmt.Errorf("failed to read Agent0 skill definition: %w", err)
	}

	if err := asset.ValidateCustomSkill(&skill); err != nil {
		return err
	}

	if flags.DryRun {
		fmt.Println("Dry run: Agent0 skill definition is valid")
		return nil
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	name := asset.CustomSkillName(&skill)
	result, err := asset.ImportCustomSkill(ctx, apiClient, &skill)
	if err != nil {
		if explained := asset.ExplainCustomSkillError(err, name); explained != nil {
			return explained
		}
		return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType, AssetName: name})
	}

	fmt.Printf("Agent0 skill %q %s (id: %s)\n", result.Name, result.Action, result.ID)
	return nil
}
