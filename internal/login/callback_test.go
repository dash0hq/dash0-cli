package login

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeriveAppHost(t *testing.T) {
	cases := []struct {
		name, apiURL, want string
	}{
		{"eu-west-1 prod", "https://api.eu-west-1.aws.dash0.com", "app.dash0.com"},
		{"prod root", "https://api.dash0.com", "app.dash0.com"},
		{"dev tenant", "https://api.dash0-dev.com", "app.dash0-dev.com"},
		{"dev regional", "https://api.eu-west-1.aws.dash0-dev.com", "app.dash0-dev.com"},
		{"non-api host", "https://example.com", ""},
		{"localhost", "http://127.0.0.1:8080", ""},
		{"invalid URL", "not a url", ""},
		{"empty", "", ""},
		{"lookalike domain", "https://api.foo.attacker.com", ""},
		{"lookalike dash0", "https://api.attacker-dash0.com", ""},
		// Hostile or unrecognized sibling subdomains under dash0.com itself
		// must not derive an app host. The matcher accepts only the root
		// (api.dash0.com) and the documented regional shape
		// (api.<region>.aws.dash0.com).
		{"ad-hoc dash0 subdomain", "https://api.staging.dash0.com", ""},
		{"empty middle segment", "https://api..dash0.com", ""},
		{"sibling of api in dash0", "https://api.attacker.dash0.com", ""},
		{"wrong aws segment shape", "https://api.foo.bar.aws.dash0.com", ""},
		// DNS hostnames are case-insensitive — Api.Dash0.com must derive
		// the same app host as api.dash0.com.
		{"mixed-case prod", "https://Api.Dash0.com", "app.dash0.com"},
		{"mixed-case regional", "https://API.eu-west-1.AWS.dash0.com", "app.dash0.com"},
		// Hostnames with explicit ports must still match — `u.Hostname()`
		// strips the port before regex matching.
		{"prod with port", "https://api.dash0.com:443", "app.dash0.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := deriveAppHost(c.apiURL); got != c.want {
				t.Errorf("deriveAppHost(%q) = %q, want %q", c.apiURL, got, c.want)
			}
		})
	}
}

func TestBuildPostLoginRedirect_Success(t *testing.T) {
	got := buildPostLoginRedirect(
		"https://api.eu-west-1.aws.dash0.com",
		callbackResult{Code: "code123", State: "state456"},
	)
	want := "https://app.dash0.com/oauth/handoff-success"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildPostLoginRedirect_FailureWithDescription(t *testing.T) {
	got := buildPostLoginRedirect(
		"https://api.dash0-dev.com",
		callbackResult{ErrorCode: "access_denied", ErrorDescription: "user declined"},
	)
	if !strings.HasPrefix(got, "https://app.dash0-dev.com/oauth/handoff-failed?") {
		t.Errorf("unexpected redirect base: %q", got)
	}
	if !strings.Contains(got, "error=access_denied") {
		t.Errorf("expected error code in URL, got %q", got)
	}
	if !strings.Contains(got, "error_description=user+declined") {
		t.Errorf("expected url-encoded description, got %q", got)
	}
}

func TestBuildPostLoginRedirect_FailureWithoutDescription(t *testing.T) {
	got := buildPostLoginRedirect(
		"https://api.dash0-dev.com",
		callbackResult{ErrorCode: "server_error"},
	)
	if got != "https://app.dash0-dev.com/oauth/handoff-failed?error=server_error" {
		t.Errorf("unexpected URL: %q", got)
	}
}

func TestBuildPostLoginRedirect_FallbackOnNonStandardHost(t *testing.T) {
	got := buildPostLoginRedirect("http://127.0.0.1:8080", callbackResult{Code: "x"})
	if got != "" {
		t.Errorf("expected empty redirect (fallback) for localhost API URL, got %q", got)
	}
}

// newTestListener builds a callbackListener wired up just enough to drive
// its handlers via httptest. It does NOT bind a TCP port, so it cannot be
// used to test the bind path — only the in-handler logic.
func newTestListener(expectedState string) *callbackListener {
	return &callbackListener{
		apiURL:        "http://localhost",
		result:        make(chan callbackResult, 1),
		serveErr:      make(chan error, 1),
		expectedState: expectedState,
	}
}

// TestHandleCallback_RejectsNonGET asserts the OAuth redirect handler
// refuses any method other than GET. RFC 6749 §4.1.2 mandates that the AS
// redirects via 302, so the browser only ever issues GET; anything else
// is suspicious and must not even reach state validation or the single-
// shot slot.
func TestHandleCallback_RejectsNonGET(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			cl := newTestListener("expected-state")
			req := httptest.NewRequest(method, "/callback?state=expected-state&code=legit", nil)
			rec := httptest.NewRecorder()

			cl.handleCallback(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
			}
			if got := rec.Header().Get("Allow"); got != http.MethodGet {
				t.Errorf("Allow header = %q, want %q", got, http.MethodGet)
			}
			// The slot must NOT have been claimed by a non-GET request.
			if cl.done {
				t.Errorf("done was set by a %s; single-shot slot was consumed", method)
			}
			select {
			case r := <-cl.result:
				t.Errorf("unexpected result delivered: %+v", r)
			default:
			}
		})
	}
}

// TestHandleCallback_AllowsGET sanity-checks that the method gate does
// not regress the legitimate GET path.
func TestHandleCallback_AllowsGET(t *testing.T) {
	cl := newTestListener("expected-state")
	req := httptest.NewRequest(http.MethodGet, "/callback?state=expected-state&code=legit", nil)
	rec := httptest.NewRecorder()

	cl.handleCallback(rec, req)

	// localhost apiURL → fallback plain-text body (no Dash0 app redirect).
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !cl.done {
		t.Errorf("done not set after a legitimate GET callback")
	}
	select {
	case r := <-cl.result:
		if r.Code != "legit" || r.State != "expected-state" {
			t.Errorf("result = %+v, want code=legit state=expected-state", r)
		}
	default:
		t.Errorf("no result delivered after a legitimate GET callback")
	}
}

// TestHandleRoot_OnlyServesRoot asserts that the listener does NOT serve
// the friendly explanatory message for arbitrary paths. A catch-all that
// returns 200 for `/admin`, `/.git/HEAD`, etc. would be misleading to a
// curious local scanner.
func TestHandleRoot_OnlyServesRoot(t *testing.T) {
	cl := newTestListener("expected-state")

	t.Run("serves /", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		cl.handleRoot(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), "Dash0 CLI OAuth callback listener") {
			t.Errorf("missing friendly body, got %q", rec.Body.String())
		}
	})

	for _, p := range []string{"/admin", "/.git/HEAD", "/login", "/callback/extra", "/foo/bar"} {
		t.Run("rejects "+p, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, p, nil)
			rec := httptest.NewRecorder()
			cl.handleRoot(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Errorf("path %q status = %d, want %d", p, rec.Code, http.StatusNotFound)
			}
		})
	}
}

// TestHandleRoot_RejectsNonGET mirrors handleCallback's method gate.
func TestHandleRoot_RejectsNonGET(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			cl := newTestListener("expected-state")
			req := httptest.NewRequest(method, "/", nil)
			rec := httptest.NewRecorder()
			cl.handleRoot(rec, req)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
			}
			if got := rec.Header().Get("Allow"); got != http.MethodGet {
				t.Errorf("Allow header = %q, want %q", got, http.MethodGet)
			}
		})
	}
}
