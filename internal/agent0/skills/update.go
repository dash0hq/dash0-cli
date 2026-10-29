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

type updateFlags struct {
	ApiUrl    string
	AuthToken string
	File      string
	DryRun    bool
}

func newUpdateCmd() *cobra.Command {
	flags := &updateFlags{}

	cmd := &cobra.Command{
		Use:   "update [id] -f <file>",
		Short: "[experimental] Update an Agent0 custom skill from a file",
		Long: `Update an existing Agent0 custom skill from a YAML or JSON definition file. Use '-f -' to read from stdin.

If the ID argument is omitted, the origin, or else the ID, is read from the file.
The skill must exist; use create to make a new one.
The files in the definition replace the stored files. When spec.permissions is absent, the stored grants stay unchanged.` + internal.CONFIG_HINT,
		Example: `  # Update a skill from a file
  dash0 --experimental agent0 skills update <id> -f skill.yaml

  # Update using the origin or ID from the file
  dash0 --experimental agent0 skills update -f skill.yaml

  # Preview the changes
  dash0 --experimental agent0 skills update -f skill.yaml --dry-run

  # Export, edit, and update
  dash0 --experimental agent0 skills get <id> -o yaml > skill.yaml
  # edit skill.yaml
  dash0 --experimental agent0 skills update -f skill.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runUpdate(cmd.Context(), args, flags)
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Path to YAML or JSON definition file (use '-' for stdin)")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Show the diff without updating")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *updateFlags) error {
	var skill dash0api.SkillDefinition
	if err := asset.ReadDefinition(flags.File, &skill, os.Stdin); err != nil {
		return fmt.Errorf("failed to read Agent0 skill definition: %w", err)
	}

	if err := asset.ValidateCustomSkill(&skill); err != nil {
		return err
	}

	key, err := asset.ResolveUpdateKey(args, asset.UpdateKey{
		Noun:       assetType,
		UsesOrigin: true,
		Origin:     dash0api.GetCustomSkillOrigin(&skill),
		ID:         dash0api.GetCustomSkillID(&skill),
	})
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	// PUT creates a skill for an unknown origin, so the GET turns a mistyped key into a not-found error.
	before, err := apiClient.GetCustomSkill(ctx, key)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType, AssetID: key})
	}
	asset.SortCustomSkillPermissions(before)

	if origin, stored := dash0api.GetCustomSkillOrigin(&skill), dash0api.GetCustomSkillOrigin(before); origin != "" && origin != stored {
		return fmt.Errorf("argument %q is Agent0 skill %q with origin %q, but the file's origin is %q", key, dash0api.GetCustomSkillID(before), stored, origin)
	}

	name := asset.CustomSkillName(&skill)
	dash0api.StripCustomSkillServerFields(&skill)
	skill.Kind = dash0api.Dash0Skill

	if flags.DryRun {
		preview, err := asset.PreviewUpdate[dash0api.SkillDefinition](before, &skill)
		if err != nil {
			return err
		}
		return asset.PrintDiff(os.Stdout, assetType, asset.CustomSkillName(preview), before, preview)
	}

	result, err := apiClient.UpdateCustomSkill(ctx, key, &skill)
	if err != nil {
		if explained := asset.ExplainCustomSkillError(err, name); explained != nil {
			return explained
		}
		return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType, AssetID: key, AssetName: name})
	}
	asset.SortCustomSkillPermissions(result)

	return asset.PrintDiff(os.Stdout, assetType, asset.CustomSkillName(result), before, result)
}
