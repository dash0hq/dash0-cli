//go:build integration

package login

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeOAuthServer implements just enough of the Dash0 OAuth surface to drive
// runLogin and runLogout end-to-end: discovery, dynamic client registration,
// authorize, token (auth_code + refresh_token), and revoke.
type fakeOAuthServer struct {
	t        *testing.T
	mux      *http.ServeMux
	server   *httptest.Server
	clientID string

	issuedCode      atomic.Value // string
	issuedChallenge atomic.Value // string -- PKCE code_challenge stored on /authorize
	revokeCount     atomic.Int32
	revoked         sync.Mutex
	revokedList     []string

	// tokenCounter lets us hand out distinct access/refresh tokens so the
	// re-login test can prove the old refresh was revoked.
	tokenCounter atomic.Int32

	// Failure-injection knobs for error-path tests. Set before the test
	// drives the browser flow.
	authorizeErrorCode atomic.Value // string -- redirect with error=<code> instead of code/state
	authorizeStateOver atomic.Value // string -- redirect with a substituted state value
	tokenErrorCode     atomic.Value // string -- 400 with this OAuth error code
	tokenOmitRefresh   atomic.Bool  // 200 OK with access_token but no refresh_token
	tokenExpiresIn     atomic.Int64 // when > 0, overrides the 3600s default expires_in
}

func newFakeOAuthServer(t *testing.T) *fakeOAuthServer {
	t.Helper()
	f := &fakeOAuthServer{
		t:        t,
		mux:      http.NewServeMux(),
		clientID: "client-fake-1234",
	}
	f.mux.HandleFunc("/.well-known/oauth-authorization-server", f.handleDiscovery)
	f.mux.HandleFunc("/oauth/register", f.handleRegister)
	f.mux.HandleFunc("/oauth/authorize", f.handleAuthorize)
	f.mux.HandleFunc("/oauth/token", f.handleToken)
	f.mux.HandleFunc("/oauth/revoke", f.handleRevoke)
	f.server = httptest.NewServer(f.mux)
	return f
}

func (f *fakeOAuthServer) URL() string { return f.server.URL }
func (f *fakeOAuthServer) Close()      { f.server.Close() }

func (f *fakeOAuthServer) Revoked() []string {
	f.revoked.Lock()
	defer f.revoked.Unlock()
	out := make([]string, len(f.revokedList))
	copy(out, f.revokedList)
	return out
}

func (f *fakeOAuthServer) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issuer":                                f.server.URL,
		"authorization_endpoint":                f.server.URL + "/oauth/authorize",
		"token_endpoint":                        f.server.URL + "/oauth/token",
		"registration_endpoint":                 f.server.URL + "/oauth/register",
		"revocation_endpoint":                   f.server.URL + "/oauth/revoke",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
	})
}

func (f *fakeOAuthServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)
	var req map[string]any
	_ = json.Unmarshal(body, &req)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"client_id":                  f.clientID,
		"client_name":                req["client_name"],
		"redirect_uris":              req["redirect_uris"],
		"grant_types":                req["grant_types"],
		"response_types":             req["response_types"],
		"token_endpoint_auth_method": "none",
		"registration_access_token":  "reg-access-token",
	})
}

func (f *fakeOAuthServer) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirect := q.Get("redirect_uri")
	state := q.Get("state")
	require.NotEmpty(f.t, redirect, "authorize requires redirect_uri")

	// Validate PKCE: the CLI must send a non-empty code_challenge with
	// S256 method. Store the challenge so handleToken can verify the
	// verifier we get later actually hashes to this value.
	challenge := q.Get("code_challenge")
	require.NotEmpty(f.t, challenge, "authorize must include a code_challenge")
	require.Equal(f.t, "S256", q.Get("code_challenge_method"), "authorize must request S256 PKCE")
	f.issuedChallenge.Store(challenge)

	u, err := url.Parse(redirect)
	require.NoError(f.t, err)
	rq := u.Query()

	if errCode, _ := f.authorizeErrorCode.Load().(string); errCode != "" {
		// Simulate the AS rejecting the request (user denied, scope error,
		// etc.). Per RFC 6749 §4.1.2.1 the AS still echoes the state.
		rq.Set("error", errCode)
		rq.Set("error_description", "fake server injected error")
		rq.Set("state", state)
	} else {
		code := "auth-code-test-1"
		f.issuedCode.Store(code)
		rq.Set("code", code)
		if override, _ := f.authorizeStateOver.Load().(string); override != "" {
			rq.Set("state", override)
		} else {
			rq.Set("state", state)
		}
	}
	u.RawQuery = rq.Encode()

	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (f *fakeOAuthServer) handleToken(w http.ResponseWriter, r *http.Request) {
	require.NoError(f.t, r.ParseForm())
	if r.Form.Get("grant_type") != "authorization_code" {
		http.Error(w, "unsupported_grant_type", http.StatusBadRequest)
		return
	}
	if expected, _ := f.issuedCode.Load().(string); expected != "" {
		require.Equal(f.t, expected, r.Form.Get("code"))
	}
	verifier := r.Form.Get("code_verifier")
	require.NotEmpty(f.t, verifier)

	// Verify PKCE: BASE64URL(SHA256(verifier)) must equal the challenge
	// the CLI sent on /authorize. RFC 7636 §4.6.
	if challenge, _ := f.issuedChallenge.Load().(string); challenge != "" {
		sum := sha256.Sum256([]byte(verifier))
		got := base64.RawURLEncoding.EncodeToString(sum[:])
		require.Equal(f.t, challenge, got, "PKCE verifier does not hash to the issued challenge")
	}

	// Failure-injection: invalid_client (or any error_code set on the fake)
	// must terminate the exchange with a 400 OAuth error body.
	if errCode, _ := f.tokenErrorCode.Load().(string); errCode != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":             errCode,
			"error_description": "fake server injected error",
		})
		return
	}

	n := f.tokenCounter.Add(1)
	access := fmt.Sprintf("dash0_at_test_access_v%d", n)
	refresh := fmt.Sprintf("dash0_rt_test_refresh_v%d", n)

	expiresIn := int64(3600)
	if override := f.tokenExpiresIn.Load(); override > 0 {
		expiresIn = override
	}
	body := map[string]any{
		"access_token": access,
		"token_type":   "Bearer",
		"expires_in":   expiresIn,
	}
	if !f.tokenOmitRefresh.Load() {
		body["refresh_token"] = refresh
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func (f *fakeOAuthServer) handleRevoke(w http.ResponseWriter, r *http.Request) {
	require.NoError(f.t, r.ParseForm())
	token := r.Form.Get("token")
	require.NotEmpty(f.t, token, "revoke requires a token")
	f.revokeCount.Add(1)
	f.revoked.Lock()
	f.revokedList = append(f.revokedList, token)
	f.revoked.Unlock()
	w.WriteHeader(http.StatusOK)
}

// followCallback synthesises a "browser" by performing the redirect dance:
// GET authorize → follow 302 to the callback URL on the local listener.
// The listener may respond 200 (post-login fallback), 302 (redirect to a
// Dash0 web app), or 400 (state mismatch rejection); the helper does not
// assert on the status so it can drive both legitimate and forged-callback
// scenarios.
func followCallback(t *testing.T, authorizeURL string) {
	t.Helper()
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if strings.Contains(req.URL.Host, "127.0.0.1") {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	resp, err := client.Get(authorizeURL)
	require.NoError(t, err)
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusFound {
		loc, err := resp.Location()
		require.NoError(t, err)
		resp2, err := http.Get(loc.String())
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, resp2.Body)
		resp2.Body.Close()
	}
}

// driveBrowserOnce wires the in-memory "browser" capture to the OAuth flow.
// The returned cleanup function restores the previous opener.
func driveBrowserOnce(t *testing.T) func() {
	t.Helper()
	authorizeURLs := make(chan string, 1)
	prev := browserOpenForTest
	browserOpenForTest = func(u string) error {
		authorizeURLs <- u
		return nil
	}
	go func() {
		select {
		case u := <-authorizeURLs:
			followCallback(t, u)
		case <-time.After(5 * time.Second):
			t.Errorf("did not receive authorize URL in time")
		}
	}()
	return func() { browserOpenForTest = prev }
}

func forceInteractive(t *testing.T) {
	t.Helper()
	prev := isTerminal
	isTerminal = func() bool { return true }
	t.Cleanup(func() { isTerminal = prev })
	t.Setenv("DASH0_AGENT_MODE", "0")
}

func TestRunLogin_OnOAuthEmpty_Silent(t *testing.T) {
	forceInteractive(t)
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	server := newFakeOAuthServer(t)
	defer server.Close()

	// Pre-create the OAuth-empty profile so login proceeds without a prompt.
	store, err := profiles.NewStore()
	require.NoError(t, err)
	require.NoError(t, store.AddProfile(profiles.Profile{
		Name: "test-profile",
		Configuration: profiles.Configuration{
			ApiUrl: server.URL(),
			OAuth:  &profiles.OAuthState{},
		},
	}))

	defer driveBrowserOnce(t)()

	err = runLogin(context.Background(), loginOptions{
		ProfileName: "test-profile",
		Timeout:     5 * time.Second,
	})
	require.NoError(t, err)

	active, err := store.GetActiveProfile()
	require.NoError(t, err)
	require.Equal(t, "test-profile", active.Name)
	require.NotNil(t, active.Configuration.OAuth)
	require.NotEmpty(t, active.Configuration.OAuth.RefreshToken)
	require.Equal(t, "client-fake-1234", active.Configuration.OAuth.ClientID)
	require.NotEmpty(t, active.Configuration.AuthToken)
}

func TestRunLogin_OnMissingProfile_PromptsThenCreates(t *testing.T) {
	forceInteractive(t)
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	server := newFakeOAuthServer(t)
	defer server.Close()

	restore := confirmation.SetReaderForTest(strings.NewReader("y\n"))
	defer restore()
	defer driveBrowserOnce(t)()

	err := runLogin(context.Background(), loginOptions{
		APIURL:      server.URL(),
		ProfileName: "fresh-profile",
		Timeout:     5 * time.Second,
	})
	require.NoError(t, err)

	store, _ := profiles.NewStore()
	all, _ := store.GetProfiles()
	require.Len(t, all, 1)
	require.Equal(t, "fresh-profile", all[0].Name)
}

func TestRunLogin_OnStaticProfile_AbortAtPrompt(t *testing.T) {
	forceInteractive(t)
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	server := newFakeOAuthServer(t)
	defer server.Close()

	store, _ := profiles.NewStore()
	require.NoError(t, store.AddProfile(profiles.Profile{
		Name: "static-prof",
		Configuration: profiles.Configuration{
			ApiUrl:    server.URL(),
			AuthToken: "auth_static_xxxxxxxxxxxx",
		},
	}))

	restore := confirmation.SetReaderForTest(strings.NewReader("n\n"))
	defer restore()

	err := runLogin(context.Background(), loginOptions{
		ProfileName: "static-prof",
		Timeout:     5 * time.Second,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "aborted by user")

	// Static profile must be untouched.
	all, _ := store.GetProfiles()
	require.Len(t, all, 1)
	require.Equal(t, "auth_static_xxxxxxxxxxxx", all[0].Configuration.AuthToken)
	require.Nil(t, all[0].Configuration.OAuth)
}

func TestRunLogin_OnOAuthActive_RevokesOldRefresh(t *testing.T) {
	forceInteractive(t)
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	server := newFakeOAuthServer(t)
	defer server.Close()

	store, _ := profiles.NewStore()
	require.NoError(t, store.AddProfile(profiles.Profile{
		Name: "active-prof",
		Configuration: profiles.Configuration{
			ApiUrl:    server.URL(),
			AuthToken: "dash0_at_old_xxxxxxxxxxxx",
			OAuth: &profiles.OAuthState{
				ClientID:     "client-fake-1234",
				RefreshToken: "dash0_rt_OLD_to_be_revoked",
				ExpiresAt:    time.Now().Add(time.Hour),
			},
		},
	}))

	defer driveBrowserOnce(t)()

	err := runLogin(context.Background(), loginOptions{
		ProfileName: "active-prof",
		Timeout:     5 * time.Second,
	})
	require.NoError(t, err)

	revoked := server.Revoked()
	require.Len(t, revoked, 1)
	require.Equal(t, "dash0_rt_OLD_to_be_revoked", revoked[0])
}

func TestRunLogin_RejectsInAgentMode(t *testing.T) {
	prev := isTerminal
	isTerminal = func() bool { return false }
	t.Cleanup(func() { isTerminal = prev })

	err := runLogin(context.Background(), loginOptions{APIURL: "https://api.example.com"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "interactive terminal")
}

func TestRunLogout_OnOAuthActive_ClearsState(t *testing.T) {
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	server := newFakeOAuthServer(t)
	defer server.Close()

	store, _ := profiles.NewStore()
	require.NoError(t, store.AddProfile(profiles.Profile{
		Name: "oauth-prof",
		Configuration: profiles.Configuration{
			ApiUrl:    server.URL(),
			AuthToken: "dash0_at_aaaaaaaaaaaaaa",
			OAuth: &profiles.OAuthState{
				ClientID:     "client-fake-1234",
				RefreshToken: "dash0_rt_to_revoke",
				ExpiresAt:    time.Now().Add(time.Hour),
			},
		},
	}))

	require.NoError(t, runLogout(context.Background(), logoutOptions{Force: true}))

	require.Equal(t, []string{"dash0_rt_to_revoke"}, server.Revoked())

	// Profile must still exist, in OAuth-empty state.
	all, _ := store.GetProfiles()
	require.Len(t, all, 1)
	require.Equal(t, "oauth-prof", all[0].Name)
	require.Equal(t, "", all[0].Configuration.AuthToken)
	require.NotNil(t, all[0].Configuration.OAuth)
	require.Equal(t, "", all[0].Configuration.OAuth.RefreshToken)
}

func TestRunLogout_OnOAuthEmpty_NoOp(t *testing.T) {
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	store, _ := profiles.NewStore()
	require.NoError(t, store.AddProfile(profiles.Profile{
		Name: "empty-prof",
		Configuration: profiles.Configuration{
			ApiUrl: "https://api.example.com",
			OAuth:  &profiles.OAuthState{},
		},
	}))

	require.NoError(t, runLogout(context.Background(), logoutOptions{Force: true}))
}

// TestRunLogout_AgentModeRequiresForce verifies that agent-mode-driven
// invocations cannot silently destroy an OAuth session: refresh-token
// revocation and clearing of the local OAuth state must be opted into
// with --force, mirroring `dash0 login`'s blanket refusal to run in agent
// mode. Without this gate, an AI agent invoking `dash0 logout` (whether
// or not it passes --profile <name>) would tear down whichever session
// the env points at, because ConfirmDestructiveOperation auto-confirms
// in agent mode.
func TestRunLogout_AgentModeRequiresForce(t *testing.T) {
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	prev := agentmode.Enabled
	agentmode.Enabled = true
	defer func() { agentmode.Enabled = prev }()

	server := newFakeOAuthServer(t)
	defer server.Close()

	store, _ := profiles.NewStore()
	require.NoError(t, store.AddProfile(profiles.Profile{
		Name: "agent-prof",
		Configuration: profiles.Configuration{
			ApiUrl:    server.URL(),
			AuthToken: "dash0_at_xxxxxxxxxxxxxx",
			OAuth: &profiles.OAuthState{
				ClientID:     "client-fake-1234",
				RefreshToken: "dash0_rt_should_survive",
				ExpiresAt:    time.Now().Add(time.Hour),
			},
		},
	}))

	// Without --force, logout in agent mode must refuse cleanly and must
	// NOT call /revoke or mutate the profile.
	err := runLogout(context.Background(), logoutOptions{Force: false})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent mode")
	assert.Contains(t, err.Error(), "--force")

	require.Empty(t, server.Revoked(), "no revoke call should have been made")
	all, _ := store.GetProfiles()
	require.Len(t, all, 1)
	require.NotNil(t, all[0].Configuration.OAuth)
	require.Equal(t, "dash0_rt_should_survive", all[0].Configuration.OAuth.RefreshToken,
		"refresh token must NOT be cleared by an agent-mode logout without --force")
	require.Equal(t, "dash0_at_xxxxxxxxxxxxxx", all[0].Configuration.AuthToken,
		"access token must NOT be cleared by an agent-mode logout without --force")

	// With --force, the same agent-mode invocation proceeds normally.
	require.NoError(t, runLogout(context.Background(), logoutOptions{Force: true}))
	require.Equal(t, []string{"dash0_rt_should_survive"}, server.Revoked())
	all, _ = store.GetProfiles()
	require.Equal(t, "", all[0].Configuration.OAuth.RefreshToken)
	require.Equal(t, "", all[0].Configuration.AuthToken)
}

func TestRunLogout_OnStaticActive_ErrorsWithoutProfileHint(t *testing.T) {
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	store, _ := profiles.NewStore()
	require.NoError(t, store.AddProfile(profiles.Profile{
		Name: "stat",
		Configuration: profiles.Configuration{
			ApiUrl:    "https://api.example.com",
			AuthToken: "auth_xxxxxxxxxxxxxxxx",
		},
	}))

	err := runLogout(context.Background(), logoutOptions{Force: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "the active profile is not an OAuth profile")
	require.NotContains(t, err.Error(), "--profile")
}

func TestRunLogin_NoActiveProfile_NoFlags_Errors(t *testing.T) {
	forceInteractive(t)
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	err := runLogin(context.Background(), loginOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no profile is active")
	require.Contains(t, err.Error(), "dash0 config profiles create")
	require.Contains(t, err.Error(), "--profile <name>")
}

// TestRunLogin_StateMismatch_ListenerRejects exercises the CSRF defense in
// the callback listener: a forged (or AS-buggy) redirect that fails to echo
// the armed state is rejected with HTTP 400 before claiming the single-shot
// slot. The login itself times out because no valid callback ever arrives.
func TestRunLogin_StateMismatch_ListenerRejects(t *testing.T) {
	forceInteractive(t)
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	server := newFakeOAuthServer(t)
	defer server.Close()
	server.authorizeStateOver.Store("not-the-armed-state")

	store, _ := profiles.NewStore()
	require.NoError(t, store.AddProfile(profiles.Profile{
		Name: "csrf-prof",
		Configuration: profiles.Configuration{
			ApiUrl: server.URL(),
			OAuth:  &profiles.OAuthState{},
		},
	}))

	defer driveBrowserOnce(t)()

	err := runLogin(context.Background(), loginOptions{
		ProfileName: "csrf-prof",
		Timeout:     500 * time.Millisecond,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out", "expected timeout because the listener rejected the bad-state callback")

	// Profile must remain OAuth-empty: no tokens written.
	all, _ := store.GetProfiles()
	require.Len(t, all, 1)
	require.Empty(t, all[0].Configuration.AuthToken)
	require.NotNil(t, all[0].Configuration.OAuth)
	require.Empty(t, all[0].Configuration.OAuth.RefreshToken)
}

// TestRunLogin_InvalidClient_PurgesCache asserts that a 400 invalid_client
// response from the token endpoint invalidates the dynamic client
// registration cache so the next login re-registers cleanly. The first
// login fails; a second login on a fresh server succeeds because the
// stale cache entry is gone.
func TestRunLogin_InvalidClient_PurgesCache(t *testing.T) {
	forceInteractive(t)
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	server := newFakeOAuthServer(t)
	defer server.Close()
	server.tokenErrorCode.Store("invalid_client")

	store, _ := profiles.NewStore()
	require.NoError(t, store.AddProfile(profiles.Profile{
		Name: "ic-prof",
		Configuration: profiles.Configuration{
			ApiUrl: server.URL(),
			OAuth:  &profiles.OAuthState{},
		},
	}))

	defer driveBrowserOnce(t)()

	err := runLogin(context.Background(), loginOptions{
		ProfileName: "ic-prof",
		Timeout:     5 * time.Second,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token exchange failed")

	// Cache must have been deleted so a follow-up login does not reuse the
	// stale client_id. Peek at the on-disk store.
	clientStore, err := profiles.NewOAuthClientStore()
	require.NoError(t, err)
	_, ok, err := clientStore.Get(server.URL())
	require.NoError(t, err)
	require.False(t, ok, "invalid_client must purge the DCR cache for the offending apiURL")

	// Profile must be unchanged (still OAuth-empty).
	all, _ := store.GetProfiles()
	require.Empty(t, all[0].Configuration.AuthToken)
	require.Empty(t, all[0].Configuration.OAuth.RefreshToken)
}

// TestRunLogin_MissingRefreshToken_Aborts asserts that the CLI refuses to
// persist a profile when the AS returns a 200 OK token response without a
// refresh_token field — running commands afterwards would fail on every
// token refresh, so failing up front is the only safe behavior.
func TestRunLogin_MissingRefreshToken_Aborts(t *testing.T) {
	forceInteractive(t)
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	server := newFakeOAuthServer(t)
	defer server.Close()
	server.tokenOmitRefresh.Store(true)

	store, _ := profiles.NewStore()
	require.NoError(t, store.AddProfile(profiles.Profile{
		Name: "norefresh-prof",
		Configuration: profiles.Configuration{
			ApiUrl: server.URL(),
			OAuth:  &profiles.OAuthState{},
		},
	}))

	defer driveBrowserOnce(t)()

	err := runLogin(context.Background(), loginOptions{
		ProfileName: "norefresh-prof",
		Timeout:     5 * time.Second,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refresh token")

	all, _ := store.GetProfiles()
	require.Empty(t, all[0].Configuration.AuthToken, "no access token should be persisted when refresh_token is missing")
}

// TestRunLogin_AuthorizationServerError_Surfaces asserts that an OAuth
// error redirect (access_denied, etc.) surfaces verbatim to the user with
// both error code and description.
func TestRunLogin_AuthorizationServerError_Surfaces(t *testing.T) {
	forceInteractive(t)
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	server := newFakeOAuthServer(t)
	defer server.Close()
	server.authorizeErrorCode.Store("access_denied")

	store, _ := profiles.NewStore()
	require.NoError(t, store.AddProfile(profiles.Profile{
		Name: "denied-prof",
		Configuration: profiles.Configuration{
			ApiUrl: server.URL(),
			OAuth:  &profiles.OAuthState{},
		},
	}))

	defer driveBrowserOnce(t)()

	err := runLogin(context.Background(), loginOptions{
		ProfileName: "denied-prof",
		Timeout:     5 * time.Second,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access_denied")
	assert.Contains(t, err.Error(), "fake server injected error")

	all, _ := store.GetProfiles()
	require.Empty(t, all[0].Configuration.AuthToken)
}

func TestRunLogout_OnStaticNonActive_ErrorsWithProfileHint(t *testing.T) {
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	store, _ := profiles.NewStore()
	require.NoError(t, store.AddProfile(profiles.Profile{
		Name: "active-one",
		Configuration: profiles.Configuration{
			ApiUrl: "https://api.example.com",
			OAuth:  &profiles.OAuthState{},
		},
	}))
	require.NoError(t, store.AddProfile(profiles.Profile{
		Name: "other-static",
		Configuration: profiles.Configuration{
			ApiUrl:    "https://api.example.com",
			AuthToken: "auth_yyy",
		},
	}))
	require.NoError(t, store.SetActiveProfile("active-one"))

	err := runLogout(context.Background(), logoutOptions{ProfileName: "other-static", Force: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), `profile "other-static"`)
	// `profiles delete` takes the name positionally, so the hint is
	// `dash0 config profiles delete other-static`, not `--profile other-static`.
	require.Contains(t, err.Error(), "profiles delete other-static")
}

// TestRunLogin_ExpiresInCappedAt24h drives a fake AS that returns
// expires_in larger than the cap and asserts the persisted ExpiresAt is
// bounded to roughly now+24h. Without the cap, time.Duration overflow
// would persist an ExpiresAt in the past for very large expires_in values
// (~292 years from Unix nanoseconds).
func TestRunLogin_ExpiresInCappedAt24h(t *testing.T) {
	forceInteractive(t)
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	server := newFakeOAuthServer(t)
	defer server.Close()
	server.tokenExpiresIn.Store(86_400 * 365) // a year — far past the 24h cap

	store, _ := profiles.NewStore()
	require.NoError(t, store.AddProfile(profiles.Profile{
		Name: "cap-prof",
		Configuration: profiles.Configuration{
			ApiUrl: server.URL(),
			OAuth:  &profiles.OAuthState{},
		},
	}))

	defer driveBrowserOnce(t)()

	before := time.Now()
	err := runLogin(context.Background(), loginOptions{
		ProfileName: "cap-prof",
		Timeout:     5 * time.Second,
	})
	require.NoError(t, err)

	all, _ := store.GetProfiles()
	require.Len(t, all, 1)
	require.NotNil(t, all[0].Configuration.OAuth)

	persisted := all[0].Configuration.OAuth.ExpiresAt
	maxAllowed := before.Add(24*time.Hour + 5*time.Second) // tolerance for clock skew
	minAllowed := before.Add(23 * time.Hour)               // sanity floor
	assert.True(t, persisted.Before(maxAllowed),
		"persisted ExpiresAt %s exceeds 24h cap from %s", persisted, before)
	assert.True(t, persisted.After(minAllowed),
		"persisted ExpiresAt %s is suspiciously short (< 23h after %s)", persisted, before)
}
