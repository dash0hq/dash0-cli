package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/dash0hq/dash0-cli/internal"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	oauthpkg "github.com/dash0hq/dash0-cli/internal/oauth"
	"github.com/spf13/cobra"
)

// NewConfigCmd creates a new config command
func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Dash0 CLI configuration",
		Long:  `View and manage Dash0 CLI configuration settings`,
	}

	// Add subcommands
	cmd.AddCommand(newShowCmd())
	cmd.AddCommand(newProfileCmd())

	return cmd
}

// configShowJSON is the JSON-serializable representation of config show output.
type configShowJSON struct {
	Profile   *configShowField `json:"profile"`
	ApiUrl    *configShowField `json:"apiUrl"`
	OtlpUrl   *configShowField `json:"otlpUrl"`
	Dataset   *configShowField `json:"dataset"`
	AuthToken *configShowField `json:"authToken"`
	OAuth     *configShowOAuth `json:"oauth,omitempty"`
}

// configShowOAuth is the JSON-serializable representation of the OAuth state
// attached to a profile, when present. The Hint field is populated for
// OAuth-empty profiles so an agent inspecting the JSON output can discover
// how to recover (the human-targeted `Run \`dash0 login\`` hint is
// unrunnable in agent mode).
type configShowOAuth struct {
	ClientID      string `json:"clientId,omitempty"`
	ExpiresAt     string `json:"expiresAt,omitempty"`
	Authenticated bool   `json:"authenticated"`
	Hint          string `json:"hint,omitempty"`
}

type configShowField struct {
	Value  string `json:"value"`
	Source string `json:"source,omitempty"`
}

// newShowCmd creates a new show command
func newShowCmd() *cobra.Command {
	var outputFmt string

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		Long: `Display the current active configuration.

Configuration is resolved in the following order (highest priority first):

  1. Environment variables (DASH0_API_URL, DASH0_OTLP_URL, DASH0_AUTH_TOKEN, DASH0_DATASET)
  2. CLI flags (--api-url, --otlp-url, --auth-token, --dataset)
  3. Active profile settings

When an environment variable overrides a profile value, config show annotates the field with "(from <VAR> environment variable)".

The DASH0_CONFIG_DIR environment variable changes the configuration directory (default: ~/.dash0).`,
		Example: `  # Show the active profile and its settings
  dash0 config show

  # See the effect of an environment variable override
  DASH0_API_URL='https://api.example.com' dash0 config show

  # Use a different configuration directory
  DASH0_CONFIG_DIR=/tmp/dash0-test dash0 config show`,
		RunE: func(cmd *cobra.Command, args []string) error {
			profileSelector := ProfileSelectorFromContext(cmd.Context())

			// Check if environment variables are being used
			envApiUrl := os.Getenv(profiles.EnvApiUrl)
			envAuthToken := os.Getenv(profiles.EnvAuthToken)
			envOtlpUrl := os.Getenv(profiles.EnvOtlpUrl)
			envDataset := os.Getenv(profiles.EnvDataset)

			profileName := ""
			var config *profiles.Configuration

			if profileSelector.IsSet() {
				profileName = profileSelector.Name
				resolved, err := ResolveConfigurationForProfile(profileSelector.Name)
				if err != nil {
					return err
				}
				config = resolved
			} else {
				store, err := profiles.NewStore()
				if err != nil {
					return err
				}
				activeProfile, err := store.GetActiveProfile()
				if err == nil && activeProfile != nil {
					profileName = activeProfile.Name
					// Use the raw active profile rather than
					// GetActiveConfiguration so `config show` works on
					// OAuth-empty profiles (which would otherwise trigger
					// a failing refresh attempt). Apply env-var overrides
					// here to match the resolution order.
					cfg := activeProfile.Configuration
					if v := os.Getenv(profiles.EnvApiUrl); v != "" {
						cfg.ApiUrl = v
					}
					if v := os.Getenv(profiles.EnvAuthToken); v != "" {
						cfg.AuthToken = v
						cfg.OAuth = nil
					}
					if v := os.Getenv(profiles.EnvOtlpUrl); v != "" {
						cfg.OtlpUrl = v
					}
					if v := os.Getenv(profiles.EnvDataset); v != "" {
						cfg.Dataset = v
					}
					config = &cfg
				} else if envAuthToken != "" && (envApiUrl != "" || envOtlpUrl != "") {
					config = &profiles.Configuration{
						ApiUrl:    envApiUrl,
						AuthToken: envAuthToken,
						OtlpUrl:   envOtlpUrl,
						Dataset:   envDataset,
					}
				}
			}

			apiUrl := ""
			authToken := ""
			otlpUrl := ""
			dataset := ""
			if config != nil {
				apiUrl = config.ApiUrl
				authToken = config.AuthToken
				otlpUrl = config.OtlpUrl
				dataset = config.Dataset
			}

			datasetDisplay := dataset
			if datasetDisplay == "" {
				datasetDisplay = "default"
			}

			useJSON := strings.ToLower(outputFmt) == "json" ||
				(outputFmt == "" && agentmode.Enabled)

			if useJSON {
				result := configShowJSON{
					Profile:   profileField(profileName, profileSelector),
					ApiUrl:    showField(apiUrl, envApiUrl, profiles.EnvApiUrl),
					OtlpUrl:   showField(otlpUrl, envOtlpUrl, profiles.EnvOtlpUrl),
					Dataset:   showField(datasetDisplay, envDataset, profiles.EnvDataset),
					AuthToken: showField(maskToken(authToken), envAuthToken, profiles.EnvAuthToken),
				}
				if config != nil && config.OAuth != nil && envAuthToken == "" {
					o := &configShowOAuth{
						Authenticated: config.OAuth.RefreshToken != "",
					}
					if o.Authenticated {
						o.ClientID = config.OAuth.ClientID
						o.ExpiresAt = config.OAuth.ExpiresAt.UTC().Format(time.RFC3339)
					} else {
						// OAuth-empty: emit an agent-friendly hint that
						// names a usable recovery path (`dash0 login` is
						// unrunnable in agent mode). `profiles update`
						// takes the name as a POSITIONAL arg — splice the
						// bare name, not a `--profile X` fragment.
						o.Hint = fmt.Sprintf(
							"set DASH0_AUTH_TOKEN to a static `auth_*` token, or convert the profile with `dash0 config profiles update %s --oauth=false --auth-token auth_<...> --force`",
							profileName,
						)
					}
					result.OAuth = o
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			fmt.Printf("Profile:    ")
			if profileName == "" {
				fmt.Printf("(none)")
			} else {
				fmt.Printf("%s", profileName)
			}
			if desc := profileSelector.Source.Description(); desc != "" {
				fmt.Printf("    (%s)", desc)
			}
			fmt.Println()

			if apiUrl != "" {
				fmt.Printf("API URL:    %s", apiUrl)
				if envApiUrl != "" {
					fmt.Printf("    (from %s environment variable)", profiles.EnvApiUrl)
				}
			} else {
				fmt.Printf("API URL:    (not set)")
			}
			fmt.Println()

			if otlpUrl != "" {
				fmt.Printf("OTLP URL:   %s", otlpUrl)
				if envOtlpUrl != "" {
					fmt.Printf("    (from %s environment variable)", profiles.EnvOtlpUrl)
				}
			} else {
				fmt.Printf("OTLP URL:   (not set)")
			}
			fmt.Println()

			fmt.Printf("Dataset:    %s", datasetDisplay)
			if envDataset != "" {
				fmt.Printf("    (from %s environment variable)", profiles.EnvDataset)
			}
			fmt.Println()

			switch {
			case authToken != "":
				fmt.Printf("Auth Token: %s", maskToken(authToken))
				if envAuthToken != "" {
					fmt.Printf("    (from %s environment variable)", profiles.EnvAuthToken)
				} else if config != nil && config.OAuth != nil {
					fmt.Printf("    (OAuth, expires in %s)", friendlyExpiry(time.Until(config.OAuth.ExpiresAt)))
				}
				fmt.Println()
			case config != nil && config.OAuth != nil && envAuthToken == "":
				// OAuth-empty: marker present but no token yet.
				fmt.Println("Auth Token: (OAuth, not logged in)")
				// Only include " --profile X" when X isn't the active profile.
				fmt.Printf("            Hint: Run `dash0 login%s` to authenticate.\n", FlagFragmentIfNotActive(profileName))
			default:
				fmt.Println("Auth Token: (not set)")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputFmt, "output", "o", "", "Output format: table, json (default: table; json in agent mode)")

	return cmd
}

func showField(value, envVar, envName string) *configShowField {
	f := &configShowField{Value: value}
	if envVar != "" {
		f.Source = envName
	}
	return f
}

func profileField(name string, selector ProfileSelector) *configShowField {
	f := &configShowField{Value: name}
	switch selector.Source {
	case ProfileSourceFlag:
		f.Source = "flag:--profile"
	case ProfileSourceEnv:
		f.Source = EnvProfile
	}
	return f
}

// maskToken masks all but the last 7 characters of a token. An empty input
// returns "" so callers that pass through to JSON output do not falsely
// imply a token exists for OAuth-empty or token-less profiles.
func maskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 12 {
		return "********"
	}

	return "..." + token[len(token)-7:]
}

// friendlyExpiry renders a time.Duration as a short human-readable string for
// the `config show` line that annotates OAuth tokens.
// Negative durations render as "expired".
func friendlyExpiry(d time.Duration) string {
	if d <= 0 {
		return "expired"
	}
	return d.Round(time.Second).String()
}

// newProfileCmd creates a new profiles command
func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "profiles",
		Aliases: []string{"profile"},
		Short:   "Manage configuration profiles",
		Long:    `Create, list, delete, and select configuration profiles`,
	}

	// Add subcommands
	cmd.AddCommand(newCreateProfileCmd())
	cmd.AddCommand(newUpdateProfileCmd())
	cmd.AddCommand(newListProfileCmd())
	cmd.AddCommand(newDeleteProfileCmd())
	cmd.AddCommand(newSelectProfileCmd())

	return cmd
}

// newCreateProfileCmd creates a new create profile command
func newCreateProfileCmd() *cobra.Command {
	var (
		apiUrl, authToken, otlpUrl, dataset string
		oauth                               bool
	)

	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"add"},
		Short:   "Create a new configuration profile",
		Long: `Create a new named configuration profile.

By default the profile holds a static auth token, supplied via --auth-token.
Pass --oauth to create a profile that authenticates via OAuth 2.0; the
profile is created empty and must be populated by running 'dash0 login'.
--oauth and --auth-token are mutually exclusive.`,
		Example: `  # Static-token profile
  dash0 config profiles create dev \
      --api-url https://api.us-west-2.aws.dash0.com \
      --otlp-url https://ingress.us-west-2.aws.dash0.com \
      --auth-token auth_xxx

  # OAuth profile (run 'dash0 login' next)
  dash0 config profiles create dev --oauth \
      --api-url https://api.us-west-2.aws.dash0.com

  # Minimal static profile (fill the rest in later with 'profiles update')
  dash0 config profiles create staging --api-url https://api.example.com`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if oauth && authToken != "" {
				return fmt.Errorf("--oauth and --auth-token are mutually exclusive")
			}
			if oauth && apiUrl == "" {
				return fmt.Errorf("--oauth requires --api-url so that 'dash0 login' knows where to authenticate")
			}

			store, err := profiles.NewStore()
			if err != nil {
				return err
			}

			config := profiles.Configuration{
				ApiUrl:    apiUrl,
				AuthToken: authToken,
				OtlpUrl:   otlpUrl,
				Dataset:   dataset,
			}
			if oauth {
				config.OAuth = &profiles.OAuthState{}
			}

			profile := profiles.Profile{
				Name:          name,
				Configuration: config,
			}

			if err := store.AddProfile(profile); err != nil {
				return fmt.Errorf("failed to add profile: %w", err)
			}

			if oauth {
				fmt.Printf("Profile %q created (OAuth).\n", name)
				fmt.Println(OAuthAuthenticateHint(name))
			} else {
				fmt.Printf("Profile %q added\n", name)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&apiUrl, "api-url", "", "API URL for the Dash0 API")
	cmd.Flags().StringVar(&authToken, "auth-token", "", "Authentication token for the Dash0 API")
	cmd.Flags().StringVar(&otlpUrl, "otlp-url", "", "OTLP endpoint URL for sending telemetry data")
	cmd.Flags().StringVar(&dataset, "dataset", "", "Dataset to operate on")
	cmd.Flags().BoolVar(&oauth, "oauth", false, "Create the profile in OAuth mode (run 'dash0 login' to authenticate)")

	return cmd
}

// newUpdateProfileCmd creates a new update profile command
func newUpdateProfileCmd() *cobra.Command {
	var (
		apiUrl, authToken, otlpUrl, dataset string
		oauth, force                        bool
	)

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update a configuration profile",
		Long: `Update an existing configuration profile. Only the specified flags are
changed; unspecified flags are left as-is. Pass an empty string to remove a
field.

Pass --oauth to convert a static-token profile to an OAuth profile, or
--oauth=false to convert an OAuth profile back to a static-token profile.
Both transitions are destructive (they discard the existing credentials) and
will prompt for confirmation unless --force is set. --oauth and --auth-token
are mutually exclusive.`,
		Example: `  # Update the API URL of a profile
  dash0 config profiles update prod --api-url https://api.us-east-1.aws.dash0.com

  # Add an OTLP URL to an existing profile
  dash0 config profiles update prod --otlp-url https://ingress.us-east-1.aws.dash0.com

  # Remove a field by passing an empty string
  dash0 config profiles update prod --dataset ''

  # Convert a static-token profile to OAuth
  dash0 config profiles update prod --oauth

  # Convert an OAuth profile back to a static-token profile in one step
  dash0 config profiles update prod --oauth=false --auth-token auth_xxx --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			apiUrlChanged := cmd.Flags().Changed("api-url")
			authTokenChanged := cmd.Flags().Changed("auth-token")
			otlpUrlChanged := cmd.Flags().Changed("otlp-url")
			datasetChanged := cmd.Flags().Changed("dataset")
			oauthChanged := cmd.Flags().Changed("oauth")

			if !apiUrlChanged && !authTokenChanged && !otlpUrlChanged && !datasetChanged && !oauthChanged {
				return fmt.Errorf("at least one of --api-url, --auth-token, --otlp-url, --dataset, or --oauth must be specified")
			}

			if oauthChanged && oauth && authTokenChanged && authToken != "" {
				return fmt.Errorf("--oauth and --auth-token are mutually exclusive")
			}

			store, err := profiles.NewStore()
			if err != nil {
				return err
			}

			// Load existing config so we can validate the requested transition
			// before any prompts or writes.
			existing, err := loadProfileConfig(store, name)
			if err != nil {
				return err
			}

			currentlyOAuth := existing.OAuth != nil
			targetOAuth := currentlyOAuth
			if oauthChanged {
				targetOAuth = oauth
			}

			// Reject `--auth-token` on an OAuth-active profile unless the user
			// is also requesting --oauth=false in the same call.
			if authTokenChanged && currentlyOAuth && targetOAuth {
				return fmt.Errorf(
					"%s is an OAuth profile; setting --auth-token requires --oauth=false in the same command",
					ProfileDisplayName(name),
				)
			}

			// Confirmation prompts for destructive transitions.
			// In agent mode the default ConfirmDestructiveOperation skip would
			// silently discard credentials and wedge the profile, because
			// `dash0 login` refuses to run in agent mode to recover it.
			// Require an explicit --force for these transitions in agent mode.
			if oauthChanged && !currentlyOAuth && targetOAuth {
				// Static -> OAuth-empty. Confirm only when there is a token to
				// destroy.
				if existing.AuthToken != "" {
					if agentmode.Enabled && !force {
						return fmt.Errorf(
							"refusing to discard the static auth token of %s in agent mode without --force; pass --force to confirm",
							ProfileDisplayName(name),
						)
					}
					prompt := fmt.Sprintf("%s currently uses a static auth token. Marking it as OAuth will discard that token. Continue? [y/N]: ", ProfileDisplayName(name))
					ok, err := confirmation.ConfirmDestructiveOperation(cmd.Context(), prompt, force)
					if err != nil {
						return err
					}
					if !ok {
						return fmt.Errorf("aborted by user")
					}
				}
			}
			if oauthChanged && currentlyOAuth && !targetOAuth {
				// OAuth -> Static. Confirm only when there are tokens to revoke.
				if existing.AuthToken != "" || existing.OAuth.RefreshToken != "" {
					if agentmode.Enabled && !force {
						return fmt.Errorf(
							"refusing to revoke and discard the OAuth tokens of %s in agent mode without --force; pass --force to confirm",
							ProfileDisplayName(name),
						)
					}
					prompt := fmt.Sprintf("%s is logged in via OAuth. Disabling OAuth will revoke its tokens. Continue? [y/N]: ", ProfileDisplayName(name))
					ok, err := confirmation.ConfirmDestructiveOperation(cmd.Context(), prompt, force)
					if err != nil {
						return err
					}
					if !ok {
						return fmt.Errorf("aborted by user")
					}
				}
				// Best-effort revoke; failure is non-fatal but surfaced
				// after the update so the user knows the AS still holds
				// the token.
				if existing.OAuth != nil {
					if !oauthpkg.Revoke(existing.ApiUrl, existing.OAuth.RefreshToken) {
						defer fmt.Println("Note: server-side refresh-token revocation failed; the token will remain valid on the authorization server until natural expiry.")
					}
				}
			}

			if err := store.UpdateProfile(name, func(cfg *profiles.Configuration) {
				if apiUrlChanged {
					cfg.ApiUrl = apiUrl
				}
				if otlpUrlChanged {
					cfg.OtlpUrl = otlpUrl
				}
				if datasetChanged {
					cfg.Dataset = dataset
				}
				if oauthChanged {
					if targetOAuth {
						// Static -> OAuth-empty: clear the static token and
						// mark the profile as OAuth-typed.
						cfg.AuthToken = ""
						cfg.OAuth = &profiles.OAuthState{}
					} else {
						// OAuth -> Static / no-auth: clear the OAuth block and
						// the (now-revoked) access token.
						cfg.OAuth = nil
						cfg.AuthToken = ""
					}
				}
				if authTokenChanged {
					cfg.AuthToken = authToken
				}
			}); err != nil {
				return fmt.Errorf("failed to update profile: %w", err)
			}

			fmt.Printf("Profile %q updated\n", name)
			if oauthChanged && targetOAuth && !currentlyOAuth {
				fmt.Println(OAuthAuthenticateHint(name))
			}
			if oauthChanged && !targetOAuth && (!authTokenChanged || authToken == "") {
				// OAuth → no-auth: the profile is now token-less.
				fmt.Printf("Hint: This profile has no auth token configured. Set one with `dash0 config profiles update %s --auth-token auth_<...>`.\n", name)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&apiUrl, "api-url", "", "API URL for the Dash0 API")
	cmd.Flags().StringVar(&authToken, "auth-token", "", "Authentication token for the Dash0 API")
	cmd.Flags().StringVar(&otlpUrl, "otlp-url", "", "OTLP endpoint URL for sending telemetry data")
	cmd.Flags().StringVar(&dataset, "dataset", "", "Dataset to operate on")
	cmd.Flags().BoolVar(&oauth, "oauth", false, "Mark the profile as OAuth (--oauth=false to disable). Discards existing credentials on transition.")
	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompts for destructive transitions")

	return cmd
}

// loadProfileConfig looks up a profile by name and returns a copy of its
// Configuration. It returns ErrProfileNotFound-style errors when the profile
// does not exist.
func loadProfileConfig(store *profiles.Store, name string) (profiles.Configuration, error) {
	all, err := store.GetProfiles()
	if err != nil {
		return profiles.Configuration{}, fmt.Errorf("failed to read profiles: %w", err)
	}
	for _, p := range all {
		if p.Name == name {
			return p.Configuration, nil
		}
	}
	return profiles.Configuration{}, fmt.Errorf("profile %q does not exist", name)
}

// profileListFormat represents the output format for profile list.
type profileListFormat string

const (
	profileListFormatTable profileListFormat = "table"
	profileListFormatJSON  profileListFormat = "json"
)

func parseProfileListFormat(s string) (profileListFormat, error) {
	switch strings.ToLower(s) {
	case "":
		if agentmode.Enabled {
			return profileListFormatJSON, nil
		}
		return profileListFormatTable, nil
	case "table":
		return profileListFormatTable, nil
	case "json":
		return profileListFormatJSON, nil
	default:
		return "", fmt.Errorf("unknown output format: %s (valid formats: table, json)", s)
	}
}

// profileJSON is the JSON-serializable representation of a profile in list
// output. Auth tokens are always masked.
type profileJSON struct {
	Name      string `json:"name"`
	Active    bool   `json:"active"`
	ApiUrl    string `json:"apiUrl,omitempty"`
	OtlpUrl   string `json:"otlpUrl,omitempty"`
	Dataset   string `json:"dataset"`
	AuthToken string `json:"authToken,omitempty"`
	AuthKind  string `json:"authKind"`
}

// authKind classifies the auth state of a profile for both human and JSON
// output. Values: "static", "oauth-active", "oauth-empty", "none".
func authKind(cfg profiles.Configuration) string {
	switch {
	case cfg.OAuth != nil && cfg.OAuth.RefreshToken != "":
		return "oauth-active"
	case cfg.OAuth != nil:
		return "oauth-empty"
	case cfg.AuthToken != "":
		return "static"
	default:
		return "none"
	}
}

// formatAuthColumn produces the cell displayed under the AUTH column of
// `profiles list`. The format is `<kind> <detail>` where detail varies by
// kind so the column carries useful information at a glance.
func formatAuthColumn(cfg profiles.Configuration) string {
	switch authKind(cfg) {
	case "oauth-active":
		return fmt.Sprintf("oauth %s (%s)", maskToken(cfg.AuthToken), friendlyExpiry(time.Until(cfg.OAuth.ExpiresAt)))
	case "oauth-empty":
		return "oauth (not logged in)"
	case "static":
		return fmt.Sprintf("static %s", maskToken(cfg.AuthToken))
	default:
		return "(none)"
	}
}

// newListProfileCmd creates a new list profile command
func newListProfileCmd() *cobra.Command {
	var (
		skipHeader bool
		outputFmt  string
	)

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all configuration profiles",
		Long:    `Display a list of all available configuration profiles`,
		Example: `  # List all profiles (active profile marked with *)
  dash0 config profiles list

  # List without the header row (pipe-friendly)
  dash0 config profiles list --skip-header

  # Output as JSON
  dash0 config profiles list -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := parseProfileListFormat(outputFmt)
			if err != nil {
				return err
			}

			if skipHeader && format != profileListFormatTable {
				return fmt.Errorf("--skip-header is not supported with output format %q", outputFmt)
			}

			store, err := profiles.NewStore()
			if err != nil {
				return err
			}

			allProfiles, err := store.GetProfiles()
			if err != nil {
				return fmt.Errorf("failed to get profiles: %w", err)
			}

			if len(allProfiles) == 0 {
				if format == profileListFormatJSON {
					fmt.Println("[]")
				} else {
					fmt.Println("No profiles configured")
				}
				return nil
			}

			activeProfileName := ""
			activeProfile, err := store.GetActiveProfile()
			if err == nil && activeProfile != nil {
				activeProfileName = activeProfile.Name
			}

			switch format {
			case profileListFormatJSON:
				return renderProfilesJSON(allProfiles, activeProfileName)
			default:
				return renderProfilesTable(allProfiles, activeProfileName, skipHeader)
			}
		},
	}

	cmd.Flags().BoolVar(&skipHeader, "skip-header", false, "Omit the header row from table output")
	cmd.Flags().StringVarP(&outputFmt, "output", "o", "", "Output format: table, json (default: table)")

	return cmd
}

func renderProfilesJSON(profiles []profiles.Profile, activeProfileName string) error {
	items := make([]profileJSON, len(profiles))
	for i, p := range profiles {
		dataset := p.Configuration.Dataset
		if dataset == "" {
			dataset = "default"
		}
		items[i] = profileJSON{
			Name:      p.Name,
			Active:    p.Name == activeProfileName,
			ApiUrl:    p.Configuration.ApiUrl,
			OtlpUrl:   p.Configuration.OtlpUrl,
			Dataset:   dataset,
			AuthToken: maskToken(p.Configuration.AuthToken),
			AuthKind:  authKind(p.Configuration),
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

func renderProfilesTable(profiles []profiles.Profile, activeProfileName string, skipHeader bool) error {
	// Check if any profile has an OTLP URL configured
	hasOtlpUrl := false
	for _, profile := range profiles {
		if profile.Configuration.OtlpUrl != "" {
			hasOtlpUrl = true
			break
		}
	}

	// Calculate column widths (including 2 chars for active marker "* ")
	nameWidth := len(internal.HEADER_NAME)
	apiUrlWidth := len("API URL")
	otlpUrlWidth := len("OTLP URL")
	datasetWidth := len("DATASET")
	authWidth := len("AUTH")

	for _, profile := range profiles {
		if len(profile.Name) > nameWidth {
			nameWidth = len(profile.Name)
		}
		if len(profile.Configuration.ApiUrl) > apiUrlWidth {
			apiUrlWidth = len(profile.Configuration.ApiUrl)
		}
		if len(profile.Configuration.OtlpUrl) > otlpUrlWidth {
			otlpUrlWidth = len(profile.Configuration.OtlpUrl)
		}
		ds := profile.Configuration.Dataset
		if ds == "" {
			ds = "default"
		}
		if len(ds) > datasetWidth {
			datasetWidth = len(ds)
		}
		auth := formatAuthColumn(profile.Configuration)
		if len(auth) > authWidth {
			authWidth = len(auth)
		}
	}

	// Print header
	if !skipHeader {
		if hasOtlpUrl {
			fmt.Printf("  %-*s  %-*s  %-*s  %-*s  %s\n", nameWidth, "NAME", apiUrlWidth, "API URL", otlpUrlWidth, "OTLP URL", datasetWidth, "DATASET", "AUTH")
		} else {
			fmt.Printf("  %-*s  %-*s  %-*s  %s\n", nameWidth, "NAME", apiUrlWidth, "API URL", datasetWidth, "DATASET", "AUTH")
		}
	}

	// Print rows
	for _, profile := range profiles {
		marker := " "
		if profile.Name == activeProfileName {
			marker = "*"
		}
		dataset := profile.Configuration.Dataset
		if dataset == "" {
			dataset = "default"
		}
		auth := formatAuthColumn(profile.Configuration)
		if hasOtlpUrl {
			fmt.Printf("%s %-*s  %-*s  %-*s  %-*s  %s\n",
				marker,
				nameWidth, profile.Name,
				apiUrlWidth, profile.Configuration.ApiUrl,
				otlpUrlWidth, profile.Configuration.OtlpUrl,
				datasetWidth, dataset,
				auth)
		} else {
			fmt.Printf("%s %-*s  %-*s  %-*s  %s\n",
				marker,
				nameWidth, profile.Name,
				apiUrlWidth, profile.Configuration.ApiUrl,
				datasetWidth, dataset,
				auth)
		}
	}

	return nil
}

// newDeleteProfileCmd creates a new delete profile command
func newDeleteProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete",
		Aliases: []string{"remove"},
		Short:   "Delete a configuration profile",
		Long:    `Delete a named configuration profile`,
		Example: `  # Delete a profile
  dash0 config profiles delete staging`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			store, err := profiles.NewStore()
			if err != nil {
				return err
			}

			if err := store.RemoveProfile(name); err != nil {
				return fmt.Errorf("failed to remove profile: %w", err)
			}

			fmt.Printf("Profile '%s' deleted\n", name)

			return nil
		},
	}

	return cmd
}

// newSelectProfileCmd creates a new select profile command
func newSelectProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "select <name>",
		Aliases: []string{"activate"},
		Short:   "Select a configuration profile",
		Long:    `Set the active configuration profile`,
		Example: `  # Switch to a different profile
  dash0 config profiles select prod`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			store, err := profiles.NewStore()
			if err != nil {
				return err
			}

			if err := store.SetActiveProfile(name); err != nil {
				return fmt.Errorf("failed to select profile: %w", err)
			}

			fmt.Printf("Profile '%s' is now active\n", name)

			return nil
		},
	}

	return cmd
}
