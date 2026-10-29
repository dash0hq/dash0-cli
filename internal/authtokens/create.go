package authtokens

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type createFlags struct {
	ApiUrl          string
	AuthToken       string
	File            string
	DryRun          bool
	Output          string
	SecretFile      string
	AllowAllActions bool
}

type createFormat string

const (
	createFormatTable createFormat = "table"
	createFormatJSON  createFormat = "json"
	createFormatYAML  createFormat = "yaml"
)

func parseCreateFormat(s string) (createFormat, error) {
	switch strings.ToLower(s) {
	case "":
		if agentmode.Enabled {
			return createFormatJSON, nil
		}
		return createFormatTable, nil
	case "table":
		return createFormatTable, nil
	case "json":
		return createFormatJSON, nil
	case "yaml", "yml":
		return createFormatYAML, nil
	default:
		return "", fmt.Errorf("unknown output format: %s (valid formats: table, json, yaml)", s)
	}
}

func newCreateCmd() *cobra.Command {
	flags := &createFlags{}

	cmd := &cobra.Command{
		Use:     "create -f <file>",
		Aliases: []string{"add"},
		Short:   "[experimental] Create an auth token from a file",
		Long: `Create an auth token from a YAML or JSON definition file. Use '-f -' to read from stdin.

The API returns the token's secret only in this response, so create delivers it before it returns:
on an interactive terminal it prints the secret once; with --secret-file it writes the secret to a new file that only you can read.
In a pipe, in CI, in agent mode, or with -o json or -o yaml, --secret-file is required, and create fails before it creates anything when it is missing.

The file passed to --secret-file must not exist. A path inside a git repository is refused unless git ignores it.

An origin that already exists is refused; use update to change that token.
Granting the '*' action requires --allow-all-actions.` + internal.CONFIG_HINT,
		Example: `  # Create a token and print its secret once on this terminal
  dash0 --experimental auth-tokens create -f token.yaml

  # Create a token in GitHub Actions and write its secret to a new file outside the checkout
  dash0 --experimental auth-tokens create -f token.yaml --secret-file "$RUNNER_TEMP/ingest-token"

  # Create a token that holds every action
  dash0 --experimental auth-tokens create -f admin-token.yaml --secret-file "$RUNNER_TEMP/admin-token" --allow-all-actions

  # Validate without creating
  dash0 --experimental auth-tokens create -f token.yaml --dry-run`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			return runCreate(cmd.Context(), flags, os.Stdout, term.IsTerminal(int(os.Stdout.Fd())))
		},
	}

	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Path to YAML or JSON definition file (use '-' for stdin)")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Validate without creating")
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table, json, yaml (default: table); json and yaml never contain the secret")
	cmd.Flags().StringVar(&flags.SecretFile, "secret-file", "", "Write the secret to this new file, readable only by you (required outside an interactive terminal)")
	cmd.Flags().BoolVar(&flags.AllowAllActions, allowAllFlag, false, "Allow the definition to grant the '*' action")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

// runCreate takes stdout and whether it is a terminal as arguments, so a test
// can stand in for an interactive terminal.
func runCreate(ctx context.Context, flags *createFlags, stdout io.Writer, stdoutIsTerminal bool) error {
	format, err := parseCreateFormat(flags.Output)
	if err != nil {
		return err
	}

	var token dash0api.AuthTokenDefinition
	if err := asset.ReadDefinition(flags.File, &token, os.Stdin); err != nil {
		return fmt.Errorf("failed to read auth token definition: %w", err)
	}
	if err := validateAPIVersion(&token); err != nil {
		return err
	}
	if strings.TrimSpace(token.Metadata.Name) == "" {
		return errors.New("auth token definition requires metadata.name")
	}
	if token.Spec.Actions == nil || len(*token.Spec.Actions) == 0 {
		return errors.New("auth token definition requires spec.actions")
	}
	if err := requireAllowAll(&token, flags.AllowAllActions); err != nil {
		return err
	}
	// A dry run creates no token, so it has no secret to deliver.
	if !flags.DryRun && flags.SecretFile == "" && !secretOnTerminal(stdoutIsTerminal, format) {
		return errNoSecretDestination
	}

	if token.Kind == "" {
		token.Kind = "Dash0AuthToken"
	}
	dash0api.StripAuthTokenServerFields(&token)
	if flags.DryRun {
		if flags.SecretFile != "" {
			if err := requireNotCommittable(ctx, flags.SecretFile); err != nil {
				return err
			}
			if _, err := os.Lstat(flags.SecretFile); err == nil {
				return fmt.Errorf("--secret-file %q already exists; pass a path to a new file", flags.SecretFile)
			}
		}
		_, _ = fmt.Fprintln(stdout, "Dry run: auth token definition is valid")
		return nil
	}

	var secretFile *os.File
	if flags.SecretFile != "" {
		if secretFile, err = openSecretFile(ctx, flags.SecretFile); err != nil {
			return err
		}
	}
	created, value, err := createToken(ctx, flags, &token)
	if err != nil {
		if secretFile != nil {
			discardSecretFile(secretFile)
		}
		return err
	}

	if secretFile != nil {
		if err := writeSecretFile(secretFile, value); err != nil {
			_ = os.Remove(secretFile.Name())
			return compensate(ctx, flags.ApiUrl, flags.AuthToken, created,
				fmt.Errorf("failed to write the secret to %q: %w", flags.SecretFile, err))
		}
	}

	switch format {
	case createFormatJSON:
		return printJSON(stdout, created)
	case createFormatYAML:
		return printYAML(stdout, created)
	}
	_, _ = fmt.Fprintf(stdout, "Auth token %q created (id: %s, origin: %s)\n", created.Metadata.Name, idOf(&created.Metadata), originOf(&created.Metadata))
	if secretFile != nil {
		_, _ = fmt.Fprintf(stdout, "Secret written to %s\n", flags.SecretFile)
		return nil
	}
	_, _ = fmt.Fprintln(os.Stderr, "Store this secret now: it is shown only once, and Dash0 cannot show it again.")
	if _, err := fmt.Fprintln(stdout, value.reveal()); err != nil {
		return compensate(ctx, flags.ApiUrl, flags.AuthToken, created, fmt.Errorf("failed to print the secret: %w", err))
	}
	return nil
}

// createToken refuses an origin that already exists, then creates the token.
// It returns the response with the secret already moved out.
func createToken(ctx context.Context, flags *createFlags, token *dash0api.AuthTokenDefinition) (*dash0api.AuthTokenResponseDefinition, secret, error) {
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return nil, secret{}, err
	}
	origin := originOf(&token.Metadata)
	ectx := client.ErrorContext{AssetType: assetType, AssetID: origin, AssetName: token.Metadata.Name}

	// The API answers a duplicate origin with a bare 400; the GET turns that into
	// a message that names the way out.
	if origin != "" {
		_, err = apiClient.GetAuthToken(ctx, origin)
		switch {
		case err == nil:
			return nil, secret{}, fmt.Errorf("auth token with origin %q already exists\n"+
				"Hint: use `dash0 --experimental auth-tokens update %s -f <file>` to change it", origin, origin)
		case !dash0api.IsNotFound(err):
			return nil, secret{}, client.HandleAPIError(err, ectx)
		}
	}

	created, err := apiClient.CreateAuthToken(ctx, token)
	if err != nil {
		return nil, secret{}, client.HandleAPIError(err, ectx)
	}
	value := takeSecret(created)
	if value.empty() {
		return nil, secret{}, compensate(ctx, flags.ApiUrl, flags.AuthToken, created,
			errors.New("the API created the auth token but returned no secret"))
	}
	return created, value, nil
}

// compensate deletes a token whose secret cannot be delivered, because a token
// nobody holds the secret of is only an attack surface.
func compensate(ctx context.Context, apiURL, authToken string, token *dash0api.AuthTokenResponseDefinition, cause error) error {
	// The rollback must still run after Ctrl-C cancels the command's context.
	// The client's request timeout bounds it.
	ctx = context.WithoutCancel(ctx)
	id := idOf(&token.Metadata)
	apiClient, err := client.NewClientFromContext(ctx, apiURL, authToken)
	if err == nil {
		err = apiClient.DeleteAuthToken(ctx, id)
	}
	if err != nil {
		return fmt.Errorf("%w\nauth token %q (id: %s) was created and could not be deleted again: %v\n"+
			"Hint: delete it with `dash0 --experimental auth-tokens delete %s --force`", cause, token.Metadata.Name, id, err, id)
	}
	return fmt.Errorf("%w\nauth token %q (id: %s) was deleted again, because its secret could not be delivered", cause, token.Metadata.Name, id)
}
