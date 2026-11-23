package automerge

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
	connectionFlags
	File   string
	DryRun bool
}

func newUpdateCmd() *cobra.Command {
	flags := &updateFlags{}

	cmd := &cobra.Command{
		Use:   "update [owner/repo | repository-id] -f <file>",
		Short: "[experimental] Replace the AutoMerge settings of a repository from a file",
		Long: `Replace the AutoMerge settings of a repository with the settings in a YAML or
JSON file. Use '-f -' to read from stdin. Changing settings needs the admin role.

The whole spec is replaced, so start from 'get -o yaml'. A file that carries
metadata.labels["dash0.com/version"] is rejected if the settings changed since
that version was read; remove the label to overwrite them regardless.

When the positional argument is omitted, the repository is taken from the
document's 'dash0.com/id' label, or else from metadata.name.

spec.protectedPaths is deprecated and kept in step with the protected path rules
that name no authors. A pattern in it that no protected rule covers is added as a
protected rule, so a path is never un-protected by accident.

The output is a unified diff of the spec before and after.` + internal.CONFIG_HINT,
		Example: `  # Replace the settings, taking the repository from the file
  dash0 --experimental automerge settings update -f automerge.yaml

  # Replace the settings of an explicit repository
  dash0 --experimental automerge settings update acme/api -f automerge.yaml

  # Preview the diff without applying
  dash0 --experimental automerge settings update -f automerge.yaml --dry-run`,
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
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Show the diff without applying it")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *updateFlags) error {
	raw, err := asset.ReadRawInput(flags.File, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read AutoMerge settings definition: %w", err)
	}
	settings, warnings, err := asset.ParseDarkplaneRepositorySettings(raw)
	if err != nil {
		return err
	}
	ref, err := updateTarget(args, settings)
	if err != nil {
		return err
	}
	printWarnings(warnings)

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}
	repositoryID, err := resolveRepositoryID(ctx, apiClient, ref)
	if err != nil {
		return err
	}
	if fileID := dash0api.GetDarkplaneRepositorySettingsID(settings); fileID != "" && fileID != repositoryID {
		return fmt.Errorf("repository %s has id %s, but the file's dash0.com/id label is %s", ref, repositoryID, fileID)
	}

	before, err := apiClient.GetDarkplaneRepositorySettings(ctx, repositoryID)
	if err != nil {
		return handleError(err, ref)
	}
	name := before.Metadata.Name
	if flags.DryRun {
		return asset.PrintDiff(os.Stdout, displayKind, name, &before.Spec, &settings.Spec)
	}

	after, err := apiClient.UpdateDarkplaneRepositorySettings(ctx, repositoryID, settings)
	if err != nil {
		return handleError(err, ref)
	}
	if err := asset.PrintDiff(os.Stdout, displayKind, name, &before.Spec, &after.Spec); err != nil {
		return err
	}
	fmt.Printf("%s %q updated (version %s)\n", displayKind, name, dash0api.GetDarkplaneRepositorySettingsVersion(after))
	return nil
}

// updateTarget picks the repository: the argument, else the file's id label, else its metadata.name.
func updateTarget(args []string, settings *dash0api.RepositorySettingsDefinition) (repositoryRef, error) {
	if len(args) == 1 {
		return parseRepositoryRef(args[0]), nil
	}
	if id := dash0api.GetDarkplaneRepositorySettingsID(settings); id != "" {
		return repositoryRef{ID: id}, nil
	}
	if settings.Metadata.Name != "" {
		return parseRepositoryRef(settings.Metadata.Name), nil
	}
	return repositoryRef{}, fmt.Errorf("name the repository: pass 'owner/repo' or set metadata.name or metadata.labels[\"dash0.com/id\"] in the file")
}
