//go:build integration

package logging

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oauthQueryServer is a mock Dash0 API that serves a cursor-paginated log query
// alongside the OAuth token endpoint, and records the bearer token presented on
// every page.
//
// It exists to pin the behavior reported in #258: a long export must keep
// working after its 15-minute access token would have expired.
type oauthQueryServer struct {
	*httptest.Server

	mu sync.Mutex
	// bearers records the Authorization header of each /api/logs request, in
	// order, so a test can assert which token carried which page.
	bearers []string
	// tokenCalls counts refresh-grant exchanges.
	tokenCalls int

	// pages is the number of pages to serve before dropping the cursor.
	pages int
	// unauthorizedPage, when positive, makes that 1-based page reply 401 the
	// first time it is requested.
	unauthorizedPage int
	unauthorizedDone bool
	// refreshStatus and refreshBody override the token endpoint's reply.
	refreshStatus int
	refreshBody   map[string]any
	// issuedTokens is the sequence of access tokens the token endpoint hands
	// out, in order.
	issuedTokens []string
}

func newOAuthQueryServer(t *testing.T, srv *oauthQueryServer) *oauthQueryServer {
	t.Helper()
	if srv.pages == 0 {
		srv.pages = 1
	}
	if len(srv.issuedTokens) == 0 {
		srv.issuedTokens = []string{"dash0_at_refreshed-1", "dash0_at_refreshed-2"}
	}
	srv.Server = httptest.NewServer(http.HandlerFunc(srv.handle))
	t.Cleanup(srv.Close)
	return srv
}

func (s *oauthQueryServer) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.URL.Path == "/oauth/token" {
		s.tokenCalls++
		if s.refreshStatus != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(s.refreshStatus)
			_ = json.NewEncoder(w).Encode(s.refreshBody)
			return
		}
		issued := s.issuedTokens[min(s.tokenCalls-1, len(s.issuedTokens)-1)]
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  issued,
			"refresh_token": fmt.Sprintf("rt-%d", s.tokenCalls),
			"token_type":    "Bearer",
			// The real authorization server issues 15-minute access tokens.
			"expires_in": int64((15 * time.Minute).Seconds()),
		})
		return
	}

	if r.URL.Path != "/api/logs" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	s.bearers = append(s.bearers, r.Header.Get("Authorization"))
	page := len(s.bearers)

	if s.unauthorizedPage > 0 && page == s.unauthorizedPage && !s.unauthorizedDone {
		s.unauthorizedDone = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "token expired"},
		})
		return
	}

	// The log-query endpoint replies in OTLP JSON shape, with pagination cursors
	// alongside the payload.
	body := map[string]any{
		"resourceLogs": []any{
			map[string]any{
				"resource": map[string]any{
					"attributes": []any{
						map[string]any{
							"key":   "service.name",
							"value": map[string]any{"stringValue": "my-service"},
						},
					},
				},
				"scopeLogs": []any{
					map[string]any{
						"scope": map[string]any{"name": "dash0-cli"},
						"logRecords": []any{
							map[string]any{
								"timeUnixNano":   "1768471200000000000",
								"severityNumber": 9,
								"severityText":   "INFO",
								"body": map[string]any{
									"stringValue": fmt.Sprintf("record from page %d", page),
								},
							},
						},
					},
				},
			},
		},
	}
	// Hand out a cursor until the last page, so the CLI keeps paginating.
	if page < s.pages {
		body["cursors"] = map[string]any{"after": fmt.Sprintf("cursor-%d", page)}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func (s *oauthQueryServer) snapshot() (bearers []string, tokenCalls int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.bearers...), s.tokenCalls
}

// seedOAuthProfile writes an active OAuth profile whose access token is inside
// the refresh threshold, mirroring the state a CLI invocation finds on disk a
// few minutes after `dash0 login`.
func seedOAuthProfile(t *testing.T, apiUrl string, expiresIn time.Duration) {
	t.Helper()
	configDir := os.Getenv("DASH0_CONFIG_DIR")
	require.NotEmpty(t, configDir, "call testutil.SetupTestEnv first")

	profileFile := profiles.ProfilesFile{
		Profiles: []profiles.Profile{{
			Name: "oauth-test",
			Configuration: profiles.Configuration{
				ApiUrl:    apiUrl,
				AuthToken: "dash0_at_stale",
				Dataset:   "default",
				OAuth: &profiles.OAuthState{
					ClientID:     "test-client",
					RefreshToken: "rt-0",
					ExpiresAt:    time.Now().Add(expiresIn),
				},
			},
		}},
	}
	data, err := json.MarshalIndent(profileFile, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, profiles.ProfilesFileName), data, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, profiles.ActiveProfileFileName), []byte("oauth-test"), 0o600))
}

// TestQueryLogs_OAuthTokenRefreshedAcrossPages asserts that every page of a
// paginated export carries a valid token and that walking three pages triggers
// exactly one refresh-grant exchange, guarding against a per-request refresh
// storm.
//
// Note this test also passes before the fix, because the startup resolution
// refreshes a token this close to expiry and the whole test then runs inside the
// new token's validity. Wall-clock expiry mid-run cannot be reproduced in a unit
// test: the authorization server's floor on expires_in is ten minutes
// (profiles.OAuthRefreshMinExpiresIn). The two tests that do fail before the fix
// are TestQueryLogs_OAuthRecoversFromMidQueryUnauthorized below, and
// TestAuthTransport/asks_the_provider_for_a_token_on_every_request in
// dash0-api-client-go, which pins the per-request mechanism directly.
func TestQueryLogs_OAuthTokenRefreshedAcrossPages(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newOAuthQueryServer(t, &oauthQueryServer{pages: 3})
	// One minute of validity left: inside the five-minute refresh threshold, so
	// the very first request must already carry a refreshed token.
	seedOAuthProfile(t, server.URL, 1*time.Minute)

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{"logs", "query", "--api-url", server.URL, "--limit", "3"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	bearers, tokenCalls := server.snapshot()
	require.Len(t, bearers, 3, "expected the query to walk all three cursor pages")
	assert.Equal(t, 1, tokenCalls, "expected exactly one refresh-grant exchange")
	for i, bearer := range bearers {
		assert.Equal(t, "Bearer dash0_at_refreshed-1", bearer,
			"page %d carried a stale token; the client must ask its provider per request", i+1)
		assert.NotContains(t, bearer, "dash0_at_stale", "page %d carried the expiring token", i+1)
	}
	assert.Contains(t, output, "record from page 1")
}

// TestQueryLogs_OAuthRecoversFromMidQueryUnauthorized covers the server
// rejecting a token the client still considered valid -- clock skew, or a
// revocation performed elsewhere.
//
// TODO: move to the API client, which already covers the replay in
// TestAuthTransportUnauthorizedRecovery.
func TestQueryLogs_OAuthRecoversFromMidQueryUnauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newOAuthQueryServer(t, &oauthQueryServer{
		pages:            2,
		unauthorizedPage: 1,
	})
	// Plenty of validity left, so nothing is refreshed proactively and the 401
	// is the only thing that can trigger a refresh.
	seedOAuthProfile(t, server.URL, 1*time.Hour)

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{"logs", "query", "--api-url", server.URL, "--limit", "2"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err, "the 401 should have been recovered from, not surfaced")

	bearers, tokenCalls := server.snapshot()
	require.GreaterOrEqual(t, len(bearers), 2, "expected the rejected request to be replayed")
	assert.Equal(t, "Bearer dash0_at_stale", bearers[0], "the first attempt should use the stored token")
	assert.Equal(t, "Bearer dash0_at_refreshed-1", bearers[1], "the replay should use a refreshed token")
	assert.Equal(t, 1, tokenCalls, "expected exactly one forced refresh")
	assert.Contains(t, output, "record from page")
}

// TestQueryLogs_OAuthRefreshRejectedMidQuery pins the failure path: when the
// refresh token is dead the user gets the actionable re-authentication message,
// a non-zero exit, and no partial output on stdout that a caller could parse as
// success.
func TestQueryLogs_OAuthRefreshRejectedMidQuery(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newOAuthQueryServer(t, &oauthQueryServer{
		pages:         1,
		refreshStatus: http.StatusBadRequest,
		refreshBody:   map[string]any{"error": "invalid_grant"},
	})
	seedOAuthProfile(t, server.URL, 1*time.Minute)

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{"logs", "query", "--api-url", server.URL})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.Error(t, err, "a dead refresh token must fail the command")
	assert.Contains(t, err.Error(), "session has expired or was revoked")
	assert.Contains(t, err.Error(), "dash0 login")
	assert.Empty(t, output, "no partial output may reach stdout on failure")
}

// TestQueryLogs_OAuthRefreshRejectedMidQueryAgentMode covers the same failure in
// agent mode, where `dash0 login` cannot run and the hint must name the
// static-token escape hatches instead.
func TestQueryLogs_OAuthRefreshRejectedMidQueryAgentMode(t *testing.T) {
	testutil.SetupTestEnv(t)
	// Set the flag directly rather than via agentmode.Init: Init(false) re-runs
	// agent detection, which leaves the flag set whenever the test itself runs
	// under an agent, leaking into every later test in the package.
	prevAgentMode := agentmode.Enabled
	agentmode.Enabled = true
	t.Cleanup(func() { agentmode.Enabled = prevAgentMode })

	server := newOAuthQueryServer(t, &oauthQueryServer{
		pages:         1,
		refreshStatus: http.StatusBadRequest,
		refreshBody:   map[string]any{"error": "invalid_grant"},
	})
	seedOAuthProfile(t, server.URL, 1*time.Minute)

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{"logs", "query", "--api-url", server.URL})

	var err error
	_ = testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DASH0_AUTH_TOKEN")
	assert.NotContains(t, err.Error(), "Run `dash0 login",
		"agent mode must not point at a command that refuses to run there")
}

// TestQueryLogs_StaticTokenIsNotRefreshed guards the escape hatch the customer
// is using as a workaround: a static token must be sent as-is, with no refresh
// attempt.
func TestQueryLogs_StaticTokenIsNotRefreshed(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newOAuthQueryServer(t, &oauthQueryServer{pages: 2})
	seedOAuthProfile(t, server.URL, 1*time.Minute)
	t.Setenv("DASH0_AUTH_TOKEN", "auth_static-override")

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{"logs", "query", "--api-url", server.URL, "--limit", "2"})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	bearers, tokenCalls := server.snapshot()
	assert.Equal(t, 0, tokenCalls, "a static token must never trigger a refresh")
	require.NotEmpty(t, bearers)
	for i, bearer := range bearers {
		assert.Equal(t, "Bearer auth_static-override", bearer, "request %d", i+1)
	}
}
