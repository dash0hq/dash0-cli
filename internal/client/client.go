package client

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/config"
	"github.com/dash0hq/dash0-cli/internal/version"
)

const (
	// DefaultMaxRetries is the default number of retries for failed API requests.
	DefaultMaxRetries = 3
)

// NewClientFromContext creates a new Dash0 API client using configuration from context.
// Flag overrides (apiUrl, authToken) are applied on top of the context configuration.
func NewClientFromContext(ctx context.Context, apiUrl, authToken string) (dash0api.Client, error) {
	cfg := profiles.FromContext(ctx)
	if cfg == nil {
		// Fallback to ResolveConfiguration if not in context
		resolved, err := profiles.ResolveConfiguration(apiUrl, authToken)
		if err != nil {
			return nil, translateConfigError(ctx, err)
		}
		cfg = resolved
		apiUrl = resolved.ApiUrl
		authToken = resolved.AuthToken
	} else {
		// Apply flag overrides on top of context configuration
		if apiUrl == "" {
			apiUrl = cfg.ApiUrl
		}
		if authToken == "" {
			authToken = cfg.AuthToken
		}
	}

	if err := checkOAuthEmpty(ctx, cfg, authToken); err != nil {
		return nil, err
	}

	maxRetries, err := resolveMaxRetries(ctx)
	if err != nil {
		return nil, err
	}

	client, err := dash0api.NewClient(
		dash0api.WithApiUrl(apiUrl),
		dash0api.WithAuthToken(authToken),
		dash0api.WithUserAgent(version.UserAgent()),
		dash0api.WithMaxRetries(maxRetries),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	return client, nil
}

// NewOtlpClientFromContext creates a new Dash0 API client configured for OTLP using configuration from context.
// Flag overrides (otlpUrl, authToken) are applied on top of the context configuration.
func NewOtlpClientFromContext(ctx context.Context, otlpUrl, authToken string) (dash0api.Client, error) {
	cfg := profiles.FromContext(ctx)

	var finalOtlpUrl, finalAuthToken string
	if cfg != nil {
		finalOtlpUrl = cfg.OtlpUrl
		finalAuthToken = cfg.AuthToken
	}

	// Apply flag overrides
	if otlpUrl != "" {
		finalOtlpUrl = otlpUrl
	}
	if authToken != "" {
		finalAuthToken = authToken
	}

	if err := checkOAuthEmpty(ctx, cfg, finalAuthToken); err != nil {
		return nil, err
	}
	if err := checkOAuthOnOtlp(ctx, cfg, finalAuthToken); err != nil {
		return nil, err
	}

	if finalOtlpUrl == "" {
		return nil, fmt.Errorf("otlp-url is required; provide it as a flag, environment variable, or configure a profile")
	}
	if finalAuthToken == "" {
		return nil, fmt.Errorf("auth-token is required; provide it as a flag, environment variable, or configure a profile")
	}

	maxRetries, err := resolveMaxRetries(ctx)
	if err != nil {
		return nil, err
	}

	client, err := dash0api.NewClient(
		dash0api.WithOtlpEndpoint(dash0api.OtlpEncodingJson, finalOtlpUrl),
		dash0api.WithAuthToken(finalAuthToken),
		dash0api.WithUserAgent(version.UserAgent()),
		dash0api.WithMaxRetries(maxRetries),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP client: %w", err)
	}

	return client, nil
}

// ResolveApiUrl returns the effective API URL from context and flag overrides.
// This is useful when the resolved URL is needed outside of client creation
// (e.g. for constructing deeplink URLs).
func ResolveApiUrl(ctx context.Context, flagApiUrl string) string {
	if flagApiUrl != "" {
		return flagApiUrl
	}
	if cfg := profiles.FromContext(ctx); cfg != nil && cfg.ApiUrl != "" {
		return cfg.ApiUrl
	}
	return ""
}

// resolveMaxRetries returns the maximum number of retries for API requests.
// It checks (in order): the --max-retries flag from context, the
// DASH0_MAX_RETRIES environment variable, then falls back to DefaultMaxRetries (3).
func resolveMaxRetries(ctx context.Context) (int, error) {
	// Check --max-retries flag from context (persistent flag on root command).
	if cmd, ok := ctx.Value(maxRetriesCmdKey{}).(*int); ok && cmd != nil && *cmd >= 0 {
		return validateMaxRetries(*cmd, "--max-retries flag")
	}

	raw := os.Getenv("DASH0_MAX_RETRIES")
	if raw == "" {
		return DefaultMaxRetries, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid DASH0_MAX_RETRIES value %q: must be an integer", raw)
	}
	return validateMaxRetries(n, "DASH0_MAX_RETRIES")
}

func validateMaxRetries(n int, source string) (int, error) {
	if n < 0 {
		return 0, fmt.Errorf("invalid %s value %q: must not be negative", source, strconv.Itoa(n))
	}
	if n > dash0api.MaxRetries {
		return 0, fmt.Errorf("invalid %s value %q: must not exceed %d", source, strconv.Itoa(n), dash0api.MaxRetries)
	}
	return n, nil
}

// translateConfigError rewrites a profile-resolution error so that OAuth
// refresh failures point users at the right next step instead of leaking
// a wrapped SDK error. In agent mode the next step is NOT `dash0 login`
// (login refuses to run without a TTY) — agents are routed to the
// DASH0_AUTH_TOKEN / `--oauth=false` escape hatches mirroring
// checkOAuthEmpty.
func translateConfigError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if dash0api.IsOAuthTokenError(err) {
		selector := config.ProfileSelectorFromContext(ctx)
		displayName := selector.Name
		if agentmode.Enabled {
			return fmt.Errorf(
				"your Dash0 session has expired or was revoked, and `dash0 login` cannot run in agent mode.\nHint: Set DASH0_AUTH_TOKEN to a static `auth_*` token, or convert %s to a static profile with `dash0 config profiles update %s --oauth=false --auth-token auth_<...> --force`.",
				config.ProfileDisplayName(displayName),
				profileNameForUpdate(ctx, displayName),
			)
		}
		return fmt.Errorf("your Dash0 session has expired or was revoked.\nHint: Run `dash0 login%s` to re-authenticate.", config.ProfileFlagFragment(displayName))
	}
	return fmt.Errorf("failed to resolve configuration: %w", err)
}

// profileNameForUpdate returns the bare profile name to splice into a
// `dash0 config profiles update <name>` hint. Used in agent-mode error
// hints where the update subcommand requires the name as a positional
// argument (the `--profile` flag selects which profile reads run against,
// not which profile is the subject of the update). Falls back to looking
// up the active profile when displayName is empty (i.e. no --profile was
// passed) so the hint is always copy-pasteable.
func profileNameForUpdate(_ context.Context, displayName string) string {
	if displayName != "" {
		return displayName
	}
	store, err := profiles.NewStore()
	if err != nil {
		return "<active-profile>"
	}
	active, err := store.GetActiveProfile()
	if err != nil || active == nil {
		return "<active-profile>"
	}
	return active.Name
}

// checkOAuthEmpty surfaces a friendly "not authenticated" error when the
// resolved profile is OAuth-typed but has no refresh token (i.e. nobody
// has run `dash0 login` against it yet, or the user has just logged out).
// It returns nil when the profile is fine or when an env-var auth token is
// shadowing the OAuth state.
func checkOAuthEmpty(ctx context.Context, cfg *profiles.Configuration, resolvedAuthToken string) error {
	if cfg == nil || cfg.OAuth == nil {
		return nil
	}
	if cfg.OAuth.RefreshToken != "" {
		return nil
	}
	// If the user has shadowed the OAuth profile with an explicit
	// DASH0_AUTH_TOKEN (or a static token via a flag), let them through —
	// the static path takes precedence.
	if os.Getenv(profiles.EnvAuthToken) != "" {
		return nil
	}
	if resolvedAuthToken != "" && cfg.AuthToken != resolvedAuthToken {
		// resolvedAuthToken came from a flag override; trust it.
		return nil
	}

	selector := config.ProfileSelectorFromContext(ctx)
	// When the user passed --profile, refer to the profile by name; otherwise
	// say "the active profile". The hint mirrors that — `dash0 login` when
	// implicitly active, `dash0 login --profile X` when explicit.
	displayName := selector.Name

	// `dash0 login` refuses to run in agent mode (no browser handoff), so
	// pointing an agent at it is a dead end. Route agents to the escape
	// hatches documented in docs/commands.md — DASH0_AUTH_TOKEN, or
	// `profiles update <name> --oauth=false --auth-token … --force`.
	// `profiles update` takes the profile name as a POSITIONAL arg, not
	// via `--profile`, so splice the bare name (resolved when implicit).
	if agentmode.Enabled {
		return fmt.Errorf(
			"%s is OAuth-typed but not authenticated, and `dash0 login` cannot run in agent mode.\nHint: Set DASH0_AUTH_TOKEN to a static `auth_*` token, or convert the profile with `dash0 config profiles update %s --oauth=false --auth-token auth_<...> --force`.",
			config.ProfileDisplayName(displayName),
			profileNameForUpdate(ctx, displayName),
		)
	}
	return fmt.Errorf(
		"%s is OAuth-typed but not authenticated.\nHint: Run `dash0 login%s` to log in.",
		config.ProfileDisplayName(displayName),
		config.ProfileFlagFragment(displayName),
	)
}

// checkOAuthOnOtlp refuses to construct an OTLP client whose auth token
// would be an OAuth-issued access token. The Dash0 OTLP ingress does not
// (yet) accept OAuth access tokens — it only honors static `auth_*`
// tokens — so a `dash0 logs send` or `dash0 spans send` driven from an
// OAuth profile would otherwise emit a noisy 401 from the server with no
// hint at the actual root cause. Fail upfront with a copy-pasteable
// recovery path instead.
//
// Mirrors checkOAuthEmpty's escape-hatch rules: an explicit
// DASH0_AUTH_TOKEN env var or a per-command --auth-token flag override
// shadows the OAuth state and is trusted as-is. That keeps the
// "set DASH0_AUTH_TOKEN to a static auth_* token" workaround working
// without forcing the user to convert the whole profile.
func checkOAuthOnOtlp(ctx context.Context, cfg *profiles.Configuration, resolvedAuthToken string) error {
	if cfg == nil || cfg.OAuth == nil {
		return nil // not an OAuth profile
	}
	// Env-var override: trust the static token shadow.
	if os.Getenv(profiles.EnvAuthToken) != "" {
		return nil
	}
	// Flag override: the resolved token differs from what the profile
	// would have supplied, so the user passed --auth-token explicitly.
	if resolvedAuthToken != "" && cfg.AuthToken != resolvedAuthToken {
		return nil
	}

	selector := config.ProfileSelectorFromContext(ctx)
	displayName := selector.Name

	// OTLP-bound commands are routinely run from CI / agent contexts, so
	// surface the static-token fallback (env var or `--auth-token`)
	// front-and-center in both modes; the agent-mode branch additionally
	// names the per-profile conversion command since `dash0 login` is
	// off-limits there.
	if agentmode.Enabled {
		return fmt.Errorf(
			"OTLP ingestion does not accept OAuth access tokens; %s is OAuth-typed.\n"+
				"Hint: Set DASH0_AUTH_TOKEN to a static `auth_*` token for this invocation, "+
				"or convert the profile with `dash0 config profiles update %s --oauth=false --auth-token auth_<...> --force`.",
			config.ProfileDisplayName(displayName),
			profileNameForUpdate(ctx, displayName),
		)
	}
	return fmt.Errorf(
		"OTLP ingestion does not accept OAuth access tokens; %s is OAuth-typed.\n"+
			"Hint: Pass `--auth-token auth_<...>` for this invocation, set DASH0_AUTH_TOKEN, "+
			"or convert the profile with `dash0 config profiles update %s --oauth=false --auth-token auth_<...> --force`.",
		config.ProfileDisplayName(displayName),
		profileNameForUpdate(ctx, displayName),
	)
}

type maxRetriesCmdKey struct{}

// WithMaxRetries stores the --max-retries flag value in the context.
func WithMaxRetries(ctx context.Context, maxRetries *int) context.Context {
	return context.WithValue(ctx, maxRetriesCmdKey{}, maxRetries)
}

// ErrorContext provides context about the asset involved in an error.
// This context is used to generate more specific and actionable error messages.
type ErrorContext struct {
	AssetType string // e.g., "dashboard", "check rule"
	AssetID   string // e.g., "a1b2c3d4-..." (optional, empty for create/list)
	AssetName string // e.g., "Production Overview" (optional, for user-friendly messages)
}

// HandleAPIError provides user-friendly error messages for API errors.
// It checks for common error types and returns descriptive messages.
// Optional context can be provided to include asset details in error messages.
func HandleAPIError(err error, ctx ...ErrorContext) error {
	if err == nil {
		return nil
	}

	// Helper to get the best identifier (prefer name over ID)
	getIdentifier := func() string {
		if len(ctx) > 0 {
			if ctx[0].AssetName != "" {
				return ctx[0].AssetName
			}
			return ctx[0].AssetID
		}
		return ""
	}

	// Helper to get asset type
	getAssetType := func() string {
		if len(ctx) > 0 {
			return ctx[0].AssetType
		}
		return ""
	}

	if dash0api.IsNotFound(err) {
		assetType := getAssetType()
		identifier := getIdentifier()
		var prefix string
		switch {
		case assetType != "" && identifier != "":
			prefix = fmt.Sprintf("%s %q not found", assetType, identifier)
		case assetType != "":
			prefix = fmt.Sprintf("%s not found", assetType)
		default:
			prefix = "asset not found"
		}
		return formatAPIError(prefix, err)
	}
	if dash0api.IsUnauthorized(err) {
		return formatAPIError("authentication failed; check your auth token", err)
	}
	if dash0api.IsForbidden(err) {
		return formatAPIError("access denied; check your permissions", err)
	}
	if dash0api.IsBadRequest(err) {
		return formatAPIError("invalid request", err)
	}
	if dash0api.IsConflict(err) {
		assetType := getAssetType()
		identifier := getIdentifier()
		if assetType != "" {
			if identifier != "" {
				return formatAPIError(fmt.Sprintf("%s %q already exists or conflicts with existing asset", assetType, identifier), err)
			}
			return formatAPIError(fmt.Sprintf("%s already exists or conflicts with existing asset", assetType), err)
		}
		return formatAPIError("asset conflict", err)
	}
	if dash0api.IsRateLimited(err) {
		return formatAPIError("rate limited; please try again later", err)
	}
	if dash0api.IsServerError(err) {
		return formatAPIError("server error; please try again later", err)
	}

	return err
}

// formatAPIError builds a user-friendly error message. When the underlying
// error is an APIError, the output uses a two-line format with the status
// metadata on the first line and the parsed server message indented on the
// second:
//
//	invalid request (status: 400, trace_id: abc123):
//	  The submitted check rule has an invalid expression.
//
// The parsed APIError.Message (extracted by the SDK from the nested
// { "error": { "message": ... } } shape) is preferred. When no message was
// parsed, the raw response body is used as a fallback. When neither is
// available, only the status line is emitted so the trace ID is still surfaced.
func formatAPIError(prefix string, err error) error {
	var apiErr *dash0api.APIError
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("%s: %w", prefix, err)
	}

	detail := strings.TrimSpace(apiErr.Message)
	if detail == "" {
		detail = strings.TrimSpace(apiErr.Body)
	}

	const maxDetailLen = 500
	if len(detail) > maxDetailLen {
		detail = detail[:maxDetailLen] + "..."
	}

	statusLine := fmt.Sprintf("%s (status: %d", prefix, apiErr.StatusCode)
	if apiErr.TraceID != "" {
		statusLine += ", trace_id: " + apiErr.TraceID
	}
	statusLine += ")"

	if detail == "" {
		return fmt.Errorf("%s", statusLine)
	}
	return fmt.Errorf("%s:\n  %s", statusLine, detail)
}

