package authtokens

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
	ApiUrl          string
	AuthToken       string
	File            string
	DryRun          bool
	AllowAllActions bool
}

func newUpdateCmd() *cobra.Command {
	flags := &updateFlags{}

	cmd := &cobra.Command{
		Use:   "update [id] -f <file>",
		Short: "[experimental] Update an auth token from a file",
		Long: `Update an existing auth token from a YAML or JSON definition file. Use '-f -' to read from stdin.

If the ID argument is omitted, the origin or ID is taken from the file.
The token must exist: update never creates one. The secret does not change and is never shown.
A spec field left out of the file keeps its stored value. Granting the '*' action requires --allow-all-actions.` + internal.CONFIG_HINT,
		Example: `  # Update an auth token from a file
  dash0 --experimental auth-tokens update <id> -f token.yaml

  # Update using the origin or ID from the file
  dash0 --experimental auth-tokens update -f token.yaml

  # Preview the changes
  dash0 --experimental auth-tokens update -f token.yaml --dry-run`,
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
	cmd.Flags().BoolVar(&flags.AllowAllActions, allowAllFlag, false, "Allow the definition to grant the '*' action")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

func runUpdate(ctx context.Context, args []string, flags *updateFlags) error {
	var token dash0api.AuthTokenDefinition
	if err := asset.ReadDefinition(flags.File, &token, os.Stdin); err != nil {
		return fmt.Errorf("failed to read auth token definition: %w", err)
	}
	if err := validateAPIVersion(&token); err != nil {
		return err
	}
	if err := requireAllowAll(&token, flags.AllowAllActions); err != nil {
		return err
	}
	key, err := asset.ResolveUpdateKey(args, asset.UpdateKey{
		Noun:       assetType,
		UsesOrigin: true,
		Origin:     originOf(&token.Metadata),
		ID:         idOf(&token.Metadata),
	})
	if err != nil {
		return err
	}

	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	// A PUT to an unknown origin creates a token, so the GET is what makes a
	// mistyped key fail instead.
	before, err := apiClient.GetAuthToken(ctx, key)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType, AssetID: key})
	}
	takeSecret(before)
	storedID, storedOrigin := idOf(&before.Metadata), originOf(&before.Metadata)
	keySource := fmt.Sprintf("argument %q", key)
	if len(args) == 0 {
		keySource = fmt.Sprintf("%q from the file", key)
	}
	if id := idOf(&token.Metadata); id != "" && id != storedID {
		return fmt.Errorf("%s is auth token %q, but the file names auth token %q", keySource, storedID, id)
	}
	if origin := originOf(&token.Metadata); origin != "" && origin != storedOrigin {
		return fmt.Errorf("%s is auth token %q with origin %q, but the file's origin is %q", keySource, storedID, storedOrigin, origin)
	}

	if token.Kind == "" {
		token.Kind = dash0api.AuthTokenDefinitionKind(before.Kind)
	}
	if token.Metadata.Name == "" {
		token.Metadata.Name = before.Metadata.Name
	}
	dash0api.StripAuthTokenServerFields(&token)
	if storedOrigin != "" {
		if token.Metadata.Labels == nil {
			token.Metadata.Labels = &dash0api.AuthTokenLabels{}
		}
		token.Metadata.Labels.Dash0Comorigin = &storedOrigin
	}

	if flags.DryRun {
		preview, err := asset.PreviewUpdate[dash0api.AuthTokenResponseDefinition](before, &token)
		if err != nil {
			return err
		}
		return asset.PrintDiff(os.Stdout, "Auth token", preview.Metadata.Name, before, preview)
	}

	// PUT by the stored UUID: unlike an origin, an unknown UUID answers 404
	// instead of creating a token.
	after, err := apiClient.UpdateAuthToken(ctx, storedID, &token)
	if err != nil {
		return client.HandleAPIError(err, client.ErrorContext{AssetType: assetType, AssetID: storedID, AssetName: token.Metadata.Name})
	}
	if value := takeSecret(after); !value.empty() {
		return compensate(ctx, flags.ApiUrl, flags.AuthToken, after,
			fmt.Errorf("auth token %q was deleted while it was being updated, so the update created a new token", key))
	}

	return asset.PrintDiff(os.Stdout, "Auth token", after.Metadata.Name, before, after)
}
