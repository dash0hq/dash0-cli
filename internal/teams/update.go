package teams

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
	Name      string
	ColorFrom string
	ColorTo   string
}

func newUpdateCmd() *cobra.Command {
	flags := &updateFlags{}

	cmd := &cobra.Command{
		Use:   "update [id] -f <file>",
		Short: "[experimental] Update a team",
		Long: `Update a team from a YAML or JSON TeamDefinitionV1Alpha1 document.

The file must have the same shape ` + "`teams create -f`" + ` accepts and
` + "`teams get -o yaml`" + ` produces, so the export → edit → reapply loop
round-trips cleanly. If the positional <id> argument is omitted, the CLI
derives the target team from the document's dash0.com/origin label (preferred)
or dash0.com/id label. Use '-f -' to read from stdin.

The team must already exist; unlike ` + "`teams create -f`" + `, ` + "`update`" + ` does not
create a new team on 404.

The imperative --name, --color-from, and --color-to flags are still accepted
for backward compatibility but are deprecated in favor of -f.` + internal.CONFIG_HINT,
		Example: `  # Update a team from a file
  dash0 --experimental teams update -f team.yaml

  # Update using an explicit id (validated against the file's origin/id)
  dash0 --experimental teams update <id> -f team.yaml

  # Update from stdin
  cat team.yaml | dash0 --experimental teams update -f -

  # Preview the diff without mutating
  dash0 --experimental teams update -f team.yaml --dry-run

  # Export, edit, reapply
  dash0 --experimental teams get <id> -o yaml > team.yaml
  # edit team.yaml
  dash0 --experimental teams update -f team.yaml`,
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
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Print the diff without applying (declarative mode only)")
	cmd.Flags().StringVar(&flags.Name, "name", "", "New team name (imperative mode)")
	cmd.Flags().StringVar(&flags.ColorFrom, "color-from", "", "Gradient start color, e.g. \"#FF0000\" (imperative mode)")
	cmd.Flags().StringVar(&flags.ColorTo, "color-to", "", "Gradient end color, e.g. \"#00FF00\" (imperative mode)")
	_ = cmd.Flags().MarkDeprecated("name", "use -f/--file with a TeamDefinitionV1Alpha1 document instead")
	_ = cmd.Flags().MarkDeprecated("color-from", "use -f/--file with a TeamDefinitionV1Alpha1 document instead")
	_ = cmd.Flags().MarkDeprecated("color-to", "use -f/--file with a TeamDefinitionV1Alpha1 document instead")

	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *updateFlags) error {
	if flags.File != "" {
		if flags.Name != "" || flags.ColorFrom != "" || flags.ColorTo != "" {
			return fmt.Errorf("cannot combine -f/--file with --name, --color-from, or --color-to")
		}
		return runUpdateFromFile(ctx, args, flags)
	}
	if len(args) != 1 {
		return fmt.Errorf("either -f/--file or a positional <id> argument is required")
	}
	if flags.DryRun {
		return fmt.Errorf("--dry-run is only valid with -f/--file")
	}
	return runUpdateImperative(ctx, args[0], flags)
}

func runUpdateFromFile(ctx context.Context, args []string, flags *updateFlags) error {
	var team dash0api.TeamDefinitionV1Alpha1
	if err := asset.ReadDefinition(flags.File, &team, os.Stdin); err != nil {
		return fmt.Errorf("failed to read team definition: %w", err)
	}

	fileOrigin := dash0api.GetTeamOrigin(&team)
	fileID := dash0api.GetTeamID(&team)

	var addressor string
	if len(args) == 1 {
		addressor = args[0]
		// Consistency check: if the file names an origin or id, the positional
		// argument must equal one of them. This is the analogue of the check
		// in `views update` and `dashboards update`, adapted for the fact that
		// a team YAML can carry two identifiers.
		if fileOrigin != "" || fileID != "" {
			if addressor != fileOrigin && addressor != fileID {
				switch {
				case fileOrigin != "" && fileID != "":
					return fmt.Errorf("the ID argument %q does not match dash0.com/origin %q or dash0.com/id %q in the file", addressor, fileOrigin, fileID)
				case fileOrigin != "":
					return fmt.Errorf("the ID argument %q does not match dash0.com/origin %q in the file", addressor, fileOrigin)
				default:
					return fmt.Errorf("the ID argument %q does not match dash0.com/id %q in the file", addressor, fileID)
				}
			}
		}
	} else {
		// Origin wins over id, mirroring asset.ImportTeam's routing.
		addressor = fileOrigin
		if addressor == "" {
			addressor = fileID
		}
		if addressor == "" {
			return fmt.Errorf("no team ID provided as argument, and the file does not contain a dash0.com/origin or dash0.com/id label")
		}
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	before, err := apiClient.GetTeam(ctx, addressor)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "team",
			AssetID:   addressor,
		})
	}

	// Strip server-managed labels and annotations from the request body.
	// A YAML exported via `teams get -o yaml` will carry dash0.com/created-at,
	// dash0.com/updated-at, and dash0.com/id — leaving them in place either
	// gets rejected by the server or makes the diff render spurious changes.
	// This matches how ImportTeam and views update prepare the payload.
	dash0api.StripTeamServerFields(&team)

	displayName := dash0api.GetTeamDisplayName(&team)
	if displayName == "" {
		displayName = dash0api.GetTeamName(&team)
	}

	if flags.DryRun {
		// Resolve members to emails for a legible diff. Failures are non-fatal;
		// raw ids in the diff are still readable, just less friendly.
		_ = dash0api.ResolveTeamMembersToEmails(ctx, apiClient, before)
		_ = dash0api.ResolveTeamMembersToEmails(ctx, apiClient, &team)
		return asset.PrintDiff(os.Stdout, "Team", displayName, before, &team)
	}

	result, err := apiClient.UpsertTeam(ctx, addressor, &team)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "team",
			AssetID:   addressor,
			AssetName: displayName,
		})
	}

	_ = dash0api.ResolveTeamMembersToEmails(ctx, apiClient, before)
	_ = dash0api.ResolveTeamMembersToEmails(ctx, apiClient, result)

	resultName := dash0api.GetTeamDisplayName(result)
	if resultName == "" {
		resultName = dash0api.GetTeamName(result)
	}
	return asset.PrintDiff(os.Stdout, "Team", resultName, before, result)
}

func runUpdateImperative(ctx context.Context, originOrID string, flags *updateFlags) error {
	if flags.Name == "" && flags.ColorFrom == "" && flags.ColorTo == "" {
		return fmt.Errorf("nothing to update: pass -f/--file or at least one of --name, --color-from, --color-to")
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	// Fetch current state so unspecified flags round-trip unchanged. The
	// imperative `.../display` endpoint replaces the full display block, so
	// partial updates require a client-side merge.
	current, err := apiClient.GetTeam(ctx, originOrID)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "team",
			AssetID:   originOrID,
		})
	}

	display := current.Spec.Display
	if flags.Name != "" {
		display.Name = flags.Name
	}
	if flags.ColorFrom != "" {
		display.Color.From = flags.ColorFrom
	}
	if flags.ColorTo != "" {
		display.Color.To = flags.ColorTo
	}

	err = apiClient.UpdateTeamDisplay(ctx, originOrID, &display)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{
			AssetType: "team",
			AssetID:   originOrID,
		})
	}

	fmt.Printf("Team %q updated\n", dash0api.GetTeamID(current))
	return nil
}
