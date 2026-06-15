package login

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/cli/browser"
	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/config"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/dash0hq/dash0-cli/internal/oauth"
	"github.com/dash0hq/dash0-cli/internal/version"
	"golang.org/x/term"
)

// clientName is what we advertise to the Dash0 authorization server via
// dynamic client registration.
const clientName = "Dash0 CLI"

type loginOptions struct {
	APIURL      string
	ProfileName string
	Port        int
	Timeout     time.Duration
	// TimeoutExplicit reports whether the user passed --timeout on the
	// command line. If they did, runLogin treats it as a hard deadline for
	// the full flow (callback + token exchange). If not, the default
	// applies and a 30s exchange floor extends past the default to keep
	// late-callback flows healthy without surprising fast-fail users.
	TimeoutExplicit bool
}

// loginBudget describes the single time budget shared across the login
// flow's phases. It is computed once at the top of runLogin and threaded
// through helpers so the user's --timeout is a TOTAL budget across the
// browser wait + token exchange, not a per-phase budget. The 30s
// exchange floor applies only when Explicit is false — an explicit
// --timeout is honored as a hard wall-clock cap.
type loginBudget struct {
	Deadline time.Time
	Explicit bool
}

// isTerminal reports whether stdout is attached to a TTY. Override in tests
// to drive the non-interactive guardrail.
var isTerminal = func() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// browserOpenForTest is an indirection so integration tests can capture the
// authorize URL without launching a real browser. github.com/cli/browser
// handles the per-platform edge cases (WSL, Snap, Flatpak, Termux, …).
var browserOpenForTest = browser.OpenURL

// profileState classifies the auth shape of a profile at the moment login
// runs. The state machine decides what (if anything) to prompt the user.
type profileState int

const (
	profileStateMissing      profileState = iota // no profile with that name
	profileStateNoAuth                           // profile exists with no token of any kind
	profileStateStatic                           // static auth_* token, no OAuth block
	profileStateOAuthEmpty                       // OAuth block present, no refresh token
	profileStateOAuthActive                      // OAuth block populated; we will replace tokens
)

// loginTarget bundles the inputs `runLogin` needs after profile resolution
// and classification: which profile we're operating on, what state it's
// in, what tokens (if any) need superseding on success, and which AS the
// new login goes against. Carrying these together lets the phase helpers
// downstream avoid long parameter lists.
type loginTarget struct {
	Name           string       // resolved profile name (user-supplied or active)
	State          profileState // classification at the start of the flow
	OldRefresh     string       // refresh token to revoke on successful re-login (OAuth-active only)
	ExistingAPIURL string       // AS that issued OldRefresh (for cross-tenant revoke directionality)
	APIURL         string       // AS to authenticate against for the new login
	PassedExplicit bool         // user passed --profile (vs. implicit active profile)
}

// subjectPhrase returns the noun phrase used in prompts/messages to refer
// to the target profile — "the active profile 'X'" when implicit,
// "profile 'X'" when explicit.
func (t *loginTarget) subjectPhrase() string {
	return subjectPhraseFor(t.Name, t.PassedExplicit)
}

// subjectPhraseFor is the package-private helper shared by runLogin (via
// loginTarget.subjectPhrase) and runLogout. The two flows share the
// "active vs explicit" distinction for user-facing prompts; keeping the
// formatting in one place avoids the two paths drifting (e.g. different
// quote characters, different "the active" wording).
func subjectPhraseFor(name string, passedExplicit bool) string {
	if passedExplicit {
		return fmt.Sprintf("profile '%s'", name)
	}
	return fmt.Sprintf("the active profile '%s'", name)
}

// runLogin performs the full OAuth 2.0 + PKCE authorization-code flow on top
// of the state-machine rules described in the OAuth-UX plan: it never
// silently overwrites a static-token profile, and it always prompts before
// creating or converting a profile.
//
// The function is intentionally an orchestrator: each phase is its own
// helper so the top-to-bottom flow stays scannable.
func runLogin(ctx context.Context, opts loginOptions) error {
	if agentmode.Enabled || !isTerminal() {
		return errors.New(nonInteractiveErrorMessage())
	}

	target, err := resolveLoginTarget(opts)
	if err != nil {
		return err
	}

	if err := promptForDestructiveTransition(ctx, target); err != nil {
		return err
	}

	oauthClient, err := dash0api.NewOAuthClient(
		dash0api.WithApiUrl(target.APIURL),
		dash0api.WithUserAgent(version.UserAgent()),
	)
	if err != nil {
		return fmt.Errorf("failed to create OAuth client: %w", err)
	}
	defer func() { _ = oauthClient.Close(context.Background()) }()

	// Discovery up front so we surface a clear error before we bind a socket.
	if err := discoverAndVerifyPKCE(ctx, oauthClient, target.APIURL); err != nil {
		return err
	}

	// Compute the single login deadline ONCE so the browser wait + token
	// exchange share one budget. Threading a per-phase WithTimeout into
	// each helper would silently double the user's --timeout across the
	// flow (round-3 regression of the round-2 fix).
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = DefaultCallbackTimeout
	}
	budget := loginBudget{
		Deadline: time.Now().Add(timeout),
		Explicit: opts.TimeoutExplicit,
	}

	authz, listener, err := obtainAuthorizationCode(ctx, budget, opts.Port, target, oauthClient)
	if err != nil {
		return err
	}
	defer listener.Close()

	accessToken, refreshToken, expiresAt, err := exchangeAndValidateTokens(ctx, budget, oauthClient, target.APIURL, authz)
	if err != nil {
		return err
	}

	return persistAndRevokeOld(target, accessToken, refreshToken, authz.ClientID, expiresAt)
}

// resolveLoginTarget figures out which profile we're logging into, what
// state it's in, and which AS to authenticate against. This is the entire
// pre-network preamble: no OAuth client is built and no socket is bound
// until the user has been resolved.
func resolveLoginTarget(opts loginOptions) (*loginTarget, error) {
	passedExplicit := opts.ProfileName != ""
	profileName := opts.ProfileName
	if profileName == "" {
		active := activeProfileName()
		if active == "" {
			return nil, errors.New(
				"no profile is active.\n" +
					"Hint: Create one with `dash0 config profiles create <name> --oauth --api-url <url>`,\n" +
					"or pass `dash0 login --profile <name> --api-url <url>` to create it ad-hoc.",
			)
		}
		profileName = active
	}

	state, oldRefreshToken, existingAPIURL, err := classifyProfile(profileName)
	if err != nil {
		return nil, err
	}

	// Decide which API URL to authenticate against — flag -> env -> existing
	// profile -> active profile (only when target is implicit).
	apiURL, err := resolveTargetAPIURL(opts.APIURL, existingAPIURL, !passedExplicit)
	if err != nil {
		return nil, err
	}

	return &loginTarget{
		Name:           profileName,
		State:          state,
		OldRefresh:     oldRefreshToken,
		ExistingAPIURL: existingAPIURL,
		APIURL:         apiURL,
		PassedExplicit: passedExplicit,
	}, nil
}

// promptForDestructiveTransition asks the user before running OAuth login
// against a profile state that would discard or transform existing
// credentials. Returns nil immediately for already-OAuth profiles
// (OAuth-empty / OAuth-active), an error for "aborted by user", or any
// error propagated from the confirmation prompt.
func promptForDestructiveTransition(ctx context.Context, target *loginTarget) error {
	var prompt string
	switch target.State {
	case profileStateMissing:
		prompt = fmt.Sprintf("Profile %q does not exist. Create it as an OAuth profile and log in? [y/N]: ", target.Name)
	case profileStateNoAuth:
		prompt = fmt.Sprintf("%s has no auth token configured. Mark it as OAuth and log in? [y/N]: ", target.subjectPhrase())
	case profileStateStatic:
		prompt = fmt.Sprintf(
			"%s uses a static auth token. Convert it to an OAuth profile (the existing auth token will be discarded)? [y/N]: ",
			target.subjectPhrase(),
		)
	case profileStateOAuthEmpty, profileStateOAuthActive:
		return nil // No prompt: the profile is already opted into OAuth.
	}

	ok, err := confirmation.ConfirmDestructiveOperation(ctx, prompt, false)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("aborted by user")
	}
	return nil
}

// discoverAndVerifyPKCE fetches OAuth Authorization Server Metadata
// (RFC 8414) and verifies the AS supports S256 PKCE — the only method
// this CLI implements. Pulled out of the listener-bind path so a hostile
// or misconfigured AS surfaces before we touch a TCP socket.
func discoverAndVerifyPKCE(ctx context.Context, oauthClient dash0api.OAuthClient, apiURL string) error {
	meta, err := oauthClient.GetAuthorizationServerMetadata(ctx)
	if err != nil {
		return fmt.Errorf("failed to discover OAuth metadata from %s: %w", apiURL, err)
	}
	return requirePKCEs256(meta)
}

// authorizationResult captures the artifacts of a successful authorization
// flow needed to drive token exchange: the auth code (and the verifier
// that proves the code-challenge match), plus the client_id and
// redirect_uri that the AS will require us to echo in the exchange.
type authorizationResult struct {
	Code         string
	CodeVerifier string
	ClientID     string
	RedirectURI  string
}

// obtainAuthorizationCode runs the user-facing leg of the OAuth flow:
// generate PKCE + state, bind the localhost callback listener (already
// armed with state so the very first request can be checked), do the DCR
// dance, build the authorize URL, open the browser, and wait for the
// callback. The returned listener is owned by the caller and must be
// closed via defer.
func obtainAuthorizationCode(
	ctx context.Context,
	budget loginBudget,
	port int,
	target *loginTarget,
	oauthClient dash0api.OAuthClient,
) (_ *authorizationResult, _ *callbackListener, retErr error) {
	// Generate the OAuth `state` parameter BEFORE binding the listener so it
	// can be passed into startCallbackListener — the listener checks state
	// from the very first accepted request, with no race window where a
	// forged early /callback could claim the single-shot slot.
	pkce, err := dash0api.GeneratePKCEPair()
	if err != nil {
		return nil, nil, err
	}
	stateParam, err := dash0api.GenerateOAuthState()
	if err != nil {
		return nil, nil, err
	}

	listener, err := startCallbackListener(port, target.APIURL, stateParam)
	if err != nil {
		return nil, nil, err
	}
	// Close the listener on any error return. A `commit := true` flag at
	// the success path keeps it alive for the caller, who then takes
	// ownership via its own `defer listener.Close()`. Without this, future
	// maintenance that adds a new error branch could leak the listener.
	commit := false
	defer func() {
		if !commit {
			listener.Close()
		}
	}()

	redirectURI := listener.RedirectURI()

	entry, err := ensureRegisteredClient(ctx, oauthClient, target.APIURL, redirectURI)
	if err != nil {
		return nil, nil, err
	}

	authorizeURL, err := oauthClient.AuthorizeURL(&dash0api.AuthorizeURLParams{
		ResponseType:        dash0api.Code,
		ClientID:            entry.ClientID,
		RedirectURI:         redirectURI,
		CodeChallenge:       pkce.Challenge,
		CodeChallengeMethod: dash0api.S256,
		State:               stateParam,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build authorize URL: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Opening your browser to log in to %s\n", target.APIURL)
	fmt.Fprintf(os.Stderr, "If the browser does not open automatically, paste this URL:\n  %s\n", authorizeURL)
	if err := browserOpenForTest(authorizeURL); err != nil {
		fmt.Fprintf(os.Stderr, "(could not open browser automatically: %v)\n", err)
	}

	// Wait against the shared login budget — no per-phase deadline so a
	// late callback does not consume "extra" time that the exchange phase
	// also needs.
	waitCtx, cancel := context.WithDeadline(ctx, budget.Deadline)
	defer cancel()
	result, err := listener.Wait(waitCtx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			displayName := ""
			if target.PassedExplicit {
				displayName = target.Name
			}
			return nil, nil, fmt.Errorf("timed out waiting for the OAuth callback; re-run `dash0 login%s` to try again", config.ProfileFlagFragment(displayName))
		}
		if errors.Is(err, context.Canceled) {
			return nil, nil, errors.New("login aborted")
		}
		return nil, nil, fmt.Errorf("OAuth callback wait failed: %w", err)
	}

	if err := validateCallbackResult(result, stateParam); err != nil {
		return nil, nil, err
	}

	commit = true
	return &authorizationResult{
		Code:         result.Code,
		CodeVerifier: pkce.Verifier,
		ClientID:     entry.ClientID,
		RedirectURI:  redirectURI,
	}, listener, nil
}

// validateCallbackResult enforces the post-callback invariants documented
// in RFC 6749 §10.12: `state` must echo the value we sent, an
// `error=...` parameter takes precedence and is rendered through
// SanitizeASText to neutralize terminal-escape injection from a hostile
// AS, and a missing code is its own failure mode.
func validateCallbackResult(result callbackResult, stateParam string) error {
	// Validate state BEFORE inspecting the error code: a hostile AS that
	// returns `error=...&state=<attacker>` would otherwise have its
	// error_description surface to the user verbatim. The listener already
	// rejects state mismatches at the HTTP layer, so reaching this check
	// with a wrong state would mean the listener was never armed — fail
	// closed.
	if result.State != stateParam {
		return errors.New("OAuth state mismatch; aborting (this can indicate a CSRF attempt)")
	}
	if result.ErrorCode != "" {
		// Strip ASCII control characters from AS-supplied strings before
		// writing them to a terminal: a hostile or compromised AS could embed
		// ANSI escape sequences in `error_description` to redraw the line and
		// mislead the user about login outcome. Cap each field at 512 chars
		// so a runaway/abusive AS cannot flood the terminal.
		errCode := truncate(oauth.SanitizeASText(result.ErrorCode), 512)
		errDesc := truncate(oauth.SanitizeASText(result.ErrorDescription), 512)
		if errCode == "" {
			// AS returned a non-empty error code whose only characters were
			// control bytes — surface a generic message rather than a
			// confusing empty-code formatting.
			errCode = "(authorization-server error code contained only control characters)"
		}
		if errDesc != "" {
			return fmt.Errorf("authorization server returned an error: %s: %s", errCode, errDesc)
		}
		return fmt.Errorf("authorization server returned an error: %s", errCode)
	}
	if result.Code == "" {
		return errors.New("authorization server did not return a code in the callback")
	}
	return nil
}

// exchangeAndValidateTokens runs the OAuth token-exchange RPC and validates
// the response. On success it returns (accessToken, refreshToken,
// expiresAt) ready for persistence. On any validation failure
// post-exchange, it best-effort revokes the just-issued refresh token
// before returning the error — the AS state must match the discarded
// local state. The function applies the 24h expires_in cap and warns to
// stderr when truncation occurs.
func exchangeAndValidateTokens(
	ctx context.Context,
	budget loginBudget,
	oauthClient dash0api.OAuthClient,
	apiURL string,
	authz *authorizationResult,
) (accessToken, refreshToken string, expiresAt time.Time, err error) {
	// Token-exchange budget. Two modes, sharing the SAME wall-clock budget
	// as the browser wait via `budget.Deadline`:
	//   - Explicit --timeout: honor budget.Deadline as a hard cap. A
	//     near-deadline callback may leave a short exchange window, but
	//     extending past the user's deadline would silently override
	//     their fast-fail intent.
	//   - Default --timeout: if budget.Deadline still has >= 30s of
	//     headroom, use it directly (so a Ctrl-C still propagates). If
	//     remaining < 30s, derive a fresh 30s budget from the parent ctx
	//     so the exchange does not get starved by a late callback.
	const exchangeFloor = 30 * time.Second
	var (
		exchangeCtx    context.Context
		exchangeCancel context.CancelFunc
	)
	remaining := time.Until(budget.Deadline)
	if !budget.Explicit && remaining < exchangeFloor {
		exchangeCtx, exchangeCancel = context.WithTimeout(ctx, exchangeFloor)
	} else {
		exchangeCtx, exchangeCancel = context.WithDeadline(ctx, budget.Deadline)
	}
	defer exchangeCancel()

	tokenResp, err := oauthClient.ExchangeToken(exchangeCtx, &dash0api.OAuthTokenRequest{
		GrantType:    dash0api.OAuthGrantTypeAuthorizationCode,
		Code:         dash0api.Ptr(authz.Code),
		CodeVerifier: dash0api.Ptr(authz.CodeVerifier),
		ClientId:     dash0api.Ptr(authz.ClientID),
		RedirectUri:  dash0api.Ptr(authz.RedirectURI),
	})
	if err != nil {
		// If the server says the client is unknown, invalidate the cache so the
		// next login re-registers cleanly.
		var oauthErr *dash0api.OAuthTokenError
		if errors.As(err, &oauthErr) && oauthErr.Code == "invalid_client" {
			invalidateOAuthClient(apiURL)
		}
		if errors.Is(err, context.Canceled) {
			return "", "", time.Time{}, errors.New("login aborted during token exchange")
		}
		return "", "", time.Time{}, fmt.Errorf("token exchange failed: %w", err)
	}
	if tokenResp.RefreshToken == nil || *tokenResp.RefreshToken == "" {
		return "", "", time.Time{}, errors.New("token exchange succeeded but the server did not return a refresh token; aborting")
	}
	// Past this point we hold a non-empty refresh token. Any abort here must
	// revoke it best-effort so the AS state matches the discarded local
	// state — same compensation pattern as the persist-failure branch in
	// persistAndRevokeOld.
	if tokenResp.AccessToken == "" {
		oauth.Revoke(apiURL, *tokenResp.RefreshToken)
		return "", "", time.Time{}, errors.New("token exchange succeeded but the server returned an empty access token; aborting")
	}
	if tokenResp.ExpiresIn <= 0 {
		oauth.Revoke(apiURL, *tokenResp.RefreshToken)
		return "", "", time.Time{}, fmt.Errorf("token exchange succeeded but the server returned a non-positive expires_in (%d); aborting", tokenResp.ExpiresIn)
	}

	// Cap expires_in defensively so a hostile or buggy AS that returns a
	// near-MaxInt64 value cannot overflow time.Duration into a negative
	// duration (which would persist an ExpiresAt in the past, defeating
	// the refresh-before-expiry SDK logic). 24 hours is well above any
	// reasonable access-token lifetime. Surface the clamp on stderr so a
	// misconfigured AS is visible rather than silently truncated.
	cappedExpiresIn := tokenResp.ExpiresIn
	if cappedExpiresIn > maxAccessTokenLifetimeSeconds {
		fmt.Fprintf(os.Stderr, "warning: authorization server returned an access-token lifetime of %d seconds; clamping to %d seconds (24h) defensively.\n", tokenResp.ExpiresIn, maxAccessTokenLifetimeSeconds)
		cappedExpiresIn = maxAccessTokenLifetimeSeconds
	}
	expiresAt = time.Now().Add(time.Duration(cappedExpiresIn) * time.Second)

	return tokenResp.AccessToken, *tokenResp.RefreshToken, expiresAt, nil
}

// persistAndRevokeOld writes the new OAuth tokens to the profile store,
// best-effort revokes the superseded refresh token (when re-logging into
// an OAuth-active profile), and prints the success line. Compensation on
// persist failure mirrors the exchange-validation aborts: the just-issued
// refresh token is revoked so the AS state matches the discarded local
// state. Cross-tenant directionality is enforced — the old refresh token
// MUST be sent to the AS that issued it (recorded in
// target.ExistingAPIURL), never to the new apiURL.
func persistAndRevokeOld(
	target *loginTarget,
	accessToken, refreshToken, clientID string,
	expiresAt time.Time,
) error {
	if err := persistProfile(target.Name, target.APIURL, accessToken, refreshToken, clientID, expiresAt); err != nil {
		// Persist failed AFTER we successfully exchanged the code for tokens.
		// The just-issued refresh token would otherwise linger server-side
		// indefinitely. Best-effort revoke it so the AS state matches what
		// the user sees locally. Old refresh token is intentionally left
		// alone: it is still the active session on disk.
		if !oauth.Revoke(target.APIURL, refreshToken) {
			return fmt.Errorf(
				"login succeeded but the new tokens could not be persisted and the compensating revoke also failed; the newly-issued refresh token may still be valid on the authorization server — visit your Dash0 account settings to revoke active sessions manually: %w",
				err,
			)
		}
		return fmt.Errorf("login succeeded but the new tokens could not be persisted: %w", err)
	}

	// Best-effort: revoke the now-superseded refresh token if we just replaced
	// one. The revoke MUST target the authorization server that issued the
	// token (recorded in `target.ExistingAPIURL`). When ExistingAPIURL is
	// empty — a degenerate profile carrying an OAuth refresh token with no
	// recorded issuer — skip the revoke entirely rather than sending it to
	// the new apiURL: that may be a different (or attacker-controlled) URL,
	// and the token would not be valid there anyway. Surface a Note instead.
	oldRevoked := true
	if target.OldRefresh != "" {
		if target.ExistingAPIURL == "" {
			oldRevoked = false
		} else {
			oldRevoked = oauth.Revoke(target.ExistingAPIURL, target.OldRefresh)
		}
	}

	fmt.Printf("Logged in as profile %q (access token expires in %s).\n", target.Name, friendlyDuration(time.Until(expiresAt)))
	if !oldRevoked {
		fmt.Println("Note: could not revoke the previous refresh token; it will remain valid on the authorization server until natural expiry.")
	}
	return nil
}

// classifyProfile inspects the named profile (if any) and returns its
// auth state, the refresh token that should be revoked after a successful
// new login (only set for OAuth-active), and the existing API URL that
// should be used as the default when no --api-url flag is given. A non-nil
// error indicates the profile store could not be opened or read — callers
// must surface it instead of treating store I/O failures as "no such
// profile" (which would then proceed to a confusing prompt-then-create
// flow that ultimately fails on the same I/O error).
func classifyProfile(name string) (state profileState, oldRefreshToken, existingAPIURL string, err error) {
	store, err := profiles.NewStore()
	if err != nil {
		return profileStateMissing, "", "", fmt.Errorf("failed to open profile store: %w", err)
	}
	all, err := store.GetProfiles()
	if err != nil {
		return profileStateMissing, "", "", fmt.Errorf("failed to read profiles: %w", err)
	}
	for _, p := range all {
		if p.Name != name {
			continue
		}
		existingAPIURL = p.Configuration.ApiUrl
		switch {
		case p.Configuration.OAuth == nil && p.Configuration.AuthToken == "":
			return profileStateNoAuth, "", existingAPIURL, nil
		case p.Configuration.OAuth == nil:
			return profileStateStatic, "", existingAPIURL, nil
		case p.Configuration.OAuth.RefreshToken == "":
			return profileStateOAuthEmpty, "", existingAPIURL, nil
		default:
			return profileStateOAuthActive, p.Configuration.OAuth.RefreshToken, existingAPIURL, nil
		}
	}
	return profileStateMissing, "", "", nil
}

// resolveTargetAPIURL chooses the API URL to authenticate against. Precedence:
// 1. --api-url flag
// 2. DASH0_API_URL environment variable
// 3. ApiUrl of the existing target profile (if any)
// 4. ApiUrl of the active profile — ONLY when the target is implicit
//    (no --profile passed). When the user said `--profile <other>`, we
//    must not silently authenticate against the active profile's URL —
//    that could leak tokens into the wrong tenant.
func resolveTargetAPIURL(flag, existingAPIURL string, targetIsActive bool) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if env := os.Getenv(profiles.EnvApiUrl); env != "" {
		return env, nil
	}
	if existingAPIURL != "" {
		return existingAPIURL, nil
	}
	if targetIsActive {
		store, err := profiles.NewStore()
		if err == nil {
			if active, err := store.GetActiveProfile(); err == nil && active != nil && active.Configuration.ApiUrl != "" {
				return active.Configuration.ApiUrl, nil
			}
		}
	}
	return "", errors.New("no API URL set; pass --api-url, set DASH0_API_URL, or configure a profile first")
}

// activeProfileName returns the name of the currently-active profile, or
// "" if no profile is configured or the store cannot be read. The caller
// is expected to error out on "" rather than invent a default name.
func activeProfileName() string {
	store, err := profiles.NewStore()
	if err != nil {
		return ""
	}
	active, err := store.GetActiveProfile()
	if err != nil || active == nil {
		return ""
	}
	return active.Name
}

// requirePKCEs256 ensures the authorization server advertises S256 PKCE
// support. RFC 8414 allows omission of `code_challenge_methods_supported`, in
// which case the server is presumed to support S256 (it is the only method we
// implement). A nil metadata struct is treated as a discovery failure rather
// than allowed through as "no methods declared".
func requirePKCEs256(meta *dash0api.OAuthAuthorizationServerMetadata) error {
	if meta == nil {
		return errors.New("authorization server returned empty metadata; cannot verify PKCE support")
	}
	if meta.CodeChallengeMethodsSupported == nil {
		return nil
	}
	if slices.Contains(*meta.CodeChallengeMethodsSupported, dash0api.S256) {
		return nil
	}
	return errors.New("the authorization server does not advertise S256 PKCE support, which dash0 requires")
}

// ensureRegisteredClient returns a DCR cache record for apiURL, registering
// a fresh client when there is no cached entry or when the cached entry's
// redirect URI does not match the listener we just bound.
func ensureRegisteredClient(ctx context.Context, oauthClient dash0api.OAuthClient, apiURL, redirectURI string) (profiles.OAuthClientRecord, error) {
	store, storeErr := profiles.NewOAuthClientStore()
	if storeErr == nil {
		if rec, ok, err := store.Get(apiURL); err == nil && ok && rec.RedirectURI == redirectURI {
			return rec, nil
		}
	}

	authMethod := dash0api.None
	clientURI := "https://github.com/dash0hq/dash0-cli"
	resp, err := oauthClient.RegisterClient(ctx, &dash0api.OAuthClientRegistrationRequest{
		ClientName:              clientName,
		ClientUri:               &clientURI,
		RedirectUris:            []string{redirectURI},
		GrantTypes:              &[]dash0api.OAuthGrantType{dash0api.OAuthGrantTypeAuthorizationCode, dash0api.OAuthGrantTypeRefreshToken},
		ResponseTypes:           &[]dash0api.OAuthResponseType{dash0api.Code},
		TokenEndpointAuthMethod: &authMethod,
	})
	if err != nil {
		return profiles.OAuthClientRecord{}, fmt.Errorf("dynamic client registration failed: %w", err)
	}

	rec := profiles.OAuthClientRecord{
		ClientID:                resp.ClientId,
		RegistrationAccessToken: resp.RegistrationAccessToken,
		RedirectURI:             redirectURI,
	}
	if store != nil {
		if err := store.Put(apiURL, rec); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to cache OAuth client registration: %v\n", err)
		}
	}
	return rec, nil
}

// invalidateOAuthClient is a best-effort delete used when the server reports
// `invalid_client` during token exchange (the registration is stale or the
// server lost it). Errors are intentionally swallowed.
func invalidateOAuthClient(apiURL string) {
	store, err := profiles.NewOAuthClientStore()
	if err != nil {
		return
	}
	_ = store.Delete(apiURL)
}

// persistProfile creates or updates the named profile with the freshly-issued
// OAuth tokens. Existing OtlpUrl / Dataset values are preserved.
func persistProfile(name, apiURL, accessToken, refreshToken, clientID string, expiresAt time.Time) error {
	store, err := profiles.NewStore()
	if err != nil {
		return fmt.Errorf("failed to open profile store: %w", err)
	}

	existing, err := store.GetProfiles()
	if err != nil {
		return fmt.Errorf("failed to read existing profiles before persisting login: %w", err)
	}
	for _, p := range existing {
		if p.Name == name {
			return store.UpdateProfile(name, func(c *profiles.Configuration) {
				c.ApiUrl = apiURL
				c.AuthToken = accessToken
				c.OAuth = &profiles.OAuthState{
					ClientID:     clientID,
					RefreshToken: refreshToken,
					ExpiresAt:    expiresAt,
				}
			})
		}
	}

	return store.AddProfile(profiles.Profile{
		Name: name,
		Configuration: profiles.Configuration{
			ApiUrl:    apiURL,
			AuthToken: accessToken,
			OAuth: &profiles.OAuthState{
				ClientID:     clientID,
				RefreshToken: refreshToken,
				ExpiresAt:    expiresAt,
			},
		},
	})
}

// nonInteractiveErrorMessage is the error shown when login is invoked in
// agent mode or without a TTY.
func nonInteractiveErrorMessage() string {
	return "dash0 login requires an interactive terminal and cannot run in agent mode or non-TTY environments\n" +
		"Hint: Use a static auth token instead:\n" +
		"  dash0 config profiles create <name> --api-url <url> --auth-token <auth_...>\n" +
		"or set DASH0_AUTH_TOKEN in the environment."
}

// friendlyDuration rounds a duration to whole seconds and returns its
// string representation, e.g. "47m23s" or "1h0m0s". Defensive against
// negative values just in case the clock jumps.
func friendlyDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return d.Round(time.Second).String()
}

// truncate returns s capped at max bytes (with an ellipsis suffix when
// truncation occurred). Used to bound AS-supplied error strings so a
// runaway server cannot flood the terminal.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// maxAccessTokenLifetimeSeconds is the upper bound applied to the AS-reported
// `expires_in` before computing `ExpiresAt`. 24 hours is well above any
// reasonable Dash0 access-token lifetime and far below the threshold at
// which `time.Duration(expiresIn) * time.Second` overflows int64 nanoseconds
// (~292 years).
const maxAccessTokenLifetimeSeconds = 24 * 60 * 60
