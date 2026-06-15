package login

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// callbackResult is whatever the OAuth authorization endpoint redirected back
// to us. Exactly one of (Code) or (ErrorCode) will be non-empty for a real
// callback; both are empty only for the synthetic "listener closed" case.
type callbackResult struct {
	Code             string
	State            string
	ErrorCode        string
	ErrorDescription string
}

// callbackListener is a single-shot HTTP server bound to 127.0.0.1 that
// captures the OAuth redirect and hands the user off to the Dash0 web app
// via a 302 redirect.
type callbackListener struct {
	server   *http.Server
	listener net.Listener
	port     int
	apiURL   string

	mu            sync.Mutex
	result        chan callbackResult
	serveErr      chan error
	done          bool
	expectedState string
}

// startCallbackListener binds a TCP listener on 127.0.0.1:port (port=0 picks
// an ephemeral port) and serves a single OAuth callback. apiURL is the
// Dash0 API URL the user is authenticating against; the listener uses it to
// derive the Dash0 web-app host the user is redirected to once the callback
// arrives. expectedState is the OAuth `state` parameter that callbacks must
// echo — it is set before Serve starts so there is no window during which
// a forged /callback could claim the single-shot slot with an unchecked
// state. Pass a non-empty value; an empty state disables the CSRF check
// (intended only for tests that drive the redirect synchronously).
func startCallbackListener(port int, apiURL, expectedState string) (*callbackListener, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return nil, fmt.Errorf("failed to bind callback listener on 127.0.0.1:%d: %w", port, err)
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		return nil, fmt.Errorf("unexpected listener address type %T", ln.Addr())
	}

	cl := &callbackListener{
		listener:      ln,
		port:          addr.Port,
		apiURL:        apiURL,
		result:        make(chan callbackResult, 1),
		serveErr:      make(chan error, 1),
		expectedState: expectedState,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", cl.handleCallback)
	mux.HandleFunc("/", cl.handleRoot)

	// Idle/Read/Write timeouts bound how long a misbehaving (or hung) HTTP
	// client can occupy a goroutine. Localhost-only and a bounded login
	// window already cap real exposure, but defense-in-depth is cheap.
	cl.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	go func() {
		// Serve always returns a non-nil error; the graceful Close() path
		// returns http.ErrServerClosed, which is expected and is not
		// surfaced. Anything else (listener died, bind closed externally)
		// is delivered to Wait so the user sees a real diagnostic instead
		// of a misleading "timed out waiting for callback".
		if err := cl.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			cl.serveErr <- err
		}
	}()

	return cl, nil
}

// RedirectURI returns the redirect URI the OAuth flow should be configured
// with — http://127.0.0.1:<port>/callback per RFC 8252 §7.3.
func (c *callbackListener) RedirectURI() string {
	return fmt.Sprintf("http://127.0.0.1:%d/callback", c.port)
}

// Wait blocks until a callback arrives, ctx is cancelled / times out, or
// the underlying HTTP server dies unexpectedly. The serve-error case
// returns a wrapped error so the user sees what actually broke instead of
// the misleading "timed out waiting for callback".
func (c *callbackListener) Wait(ctx context.Context) (callbackResult, error) {
	select {
	case r := <-c.result:
		return r, nil
	case err := <-c.serveErr:
		return callbackResult{}, fmt.Errorf("OAuth callback listener died: %w", err)
	case <-ctx.Done():
		return callbackResult{}, ctx.Err()
	}
}

// Close shuts the HTTP server down. Safe to call multiple times.
func (c *callbackListener) Close() {
	if c.server == nil {
		return
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = c.server.Shutdown(shutdownCtx)
}

func (c *callbackListener) handleCallback(w http.ResponseWriter, r *http.Request) {
	// Restrict to GET. The OAuth authorization-code redirect (RFC 6749
	// §4.1.2) is a 302 that the browser follows with GET; any other
	// method is either a buggy client or a probe. Reject before touching
	// state validation or the single-shot slot so a forged POST/PUT
	// cannot tie up the listener even with an unguessable state.
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	res := callbackResult{
		Code:             q.Get("code"),
		State:            q.Get("state"),
		ErrorCode:        q.Get("error"),
		ErrorDescription: q.Get("error_description"),
	}

	// Validate state BEFORE claiming the single-shot slot. A forged early
	// /callback hit with a wrong (or missing) state would otherwise consume
	// the slot and DoS the legitimate browser callback within the timeout
	// window. RFC 6749 §10.12 mandates CSRF protection via `state`; treat
	// callbacks that fail to echo the armed value as someone else's request.
	// Refusing the empty-expected case is defense-in-depth: production
	// callers always pass a non-empty expectedState (the listener
	// constructor receives it), so an empty value would indicate a bug.
	c.mu.Lock()
	expected := c.expectedState
	if expected == "" || res.State != expected {
		c.mu.Unlock()
		http.Error(w, "OAuth state mismatch", http.StatusBadRequest)
		return
	}
	already := c.done
	c.done = true
	c.mu.Unlock()

	if already {
		http.Error(w, "OAuth callback already received", http.StatusGone)
		return
	}

	if target := buildPostLoginRedirect(c.apiURL, res); target != "" {
		http.Redirect(w, r, target, http.StatusFound)
	} else {
		writePostLoginFallback(w, res)
	}

	// Non-blocking send: the channel has buffer 1 and we just claimed the
	// "done" slot, so this can never block. Use a select-with-default just to
	// be defensive.
	select {
	case c.result <- res:
	default:
	}
}

// handleRoot serves a friendly explanatory message at exactly `/`, and
// returns 404 for everything else. Without the path check, the registered
// catch-all (`/` in http.ServeMux semantics) would respond with the same
// friendly 200 for any path — e.g. `/admin`, `/.git/HEAD`, `/login` — which
// is misleading at best and could nudge a curious scanner into believing
// the process is something it isn't. The listener also restricts to GET
// for the same reason as handleCallback: nothing legitimate hits this
// listener over POST/PUT/etc.
func (c *callbackListener) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Dash0 CLI OAuth callback listener. This URL exists only to receive the OAuth redirect from your browser.\n"))
}

// buildPostLoginRedirect returns the Dash0 web-app URL to redirect a
// successful or failed OAuth callback to. Returns "" when the API URL does
// not follow the `api.<rest>` convention (local dev, custom deployments),
// in which case the caller falls back to a plain-text response.
//
// TODO(dash0-cli #27): once the post-login pages on the Dash0 web app are
// confirmed to live at these paths and accept these query params, drop this
// comment. Until then, the Dash0 page contract (per the prompt sent to the
// dash0 web-app team) treats every query param as optional and degrades
// gracefully when params are missing.
func buildPostLoginRedirect(apiURL string, res callbackResult) string {
	appHost := deriveAppHost(apiURL)
	if appHost == "" {
		return ""
	}
	u := &url.URL{Scheme: "https", Host: appHost}
	if res.ErrorCode != "" {
		u.Path = "/oauth/handoff-failed"
		q := url.Values{}
		q.Set("error", res.ErrorCode)
		if res.ErrorDescription != "" {
			q.Set("error_description", res.ErrorDescription)
		}
		// TODO(dash0-cli #27): forward `error_uri` once we surface it from
		// the OAuth callback (currently unused in callbackResult).
		u.RawQuery = q.Encode()
	} else {
		u.Path = "/oauth/handoff-success"
		// TODO(dash0-cli #27): pass `expires_in` and `client_name` once we
		// delay the redirect response until after token exchange completes
		// (currently fires before we know the access-token lifetime).
	}
	return u.String()
}

// knownAppHosts maps the Dash0-controlled API DNS suffix to the single
// web-app host that serves users for that environment. A naive
// `api.<rest>` -> `app.<rest>` prefix substitution would send the browser
// to attacker-controlled `app.foo.attacker.com` when `--api-url
// https://api.foo.attacker.com` is passed, so the mapping is bounded to
// suffixes the CLI can vouch for. The production Dash0 web app is a single
// host (`app.dash0.com`) that handles regional routing internally;
// `dash0-dev.com` is the staging deployment with its own single web-app
// host. New environments require an explicit entry here before they will
// receive the post-login redirect.
// Production and staging Dash0 API hosts. Both shapes match:
//
//   - `api.dash0.com` / `api.dash0-dev.com` (the root)
//   - `api.<region>.aws.dash0.com` / `api.<region>.aws.dash0-dev.com`
//
// Anything else — including arbitrary sibling subdomains like
// `api.staging.dash0.com` — is rejected so a future ad-hoc subdomain is
// not silently treated as a redirect target.
var (
	prodAPIHostRE = regexp.MustCompile(`^api(\.[a-z0-9-]+\.aws)?\.dash0\.com$`)
	devAPIHostRE  = regexp.MustCompile(`^api(\.[a-z0-9-]+\.aws)?\.dash0-dev\.com$`)
)

// deriveAppHost returns the Dash0 web-app host the OAuth post-login redirect
// should target for the given API URL. Returns "" when the API URL does not
// belong to a Dash0-controlled DNS suffix (local dev, custom deployments,
// hostile URLs); the caller falls back to a plain-text response in that
// case. The host must match one of the regexes above — the CLI does not
// derive an app host for ad-hoc subdomains under the same suffix.
// DNS hostnames are case-insensitive (RFC 4343), so `Api.Dash0.com` must
// be treated the same as `api.dash0.com`; lowercase before matching.
func deriveAppHost(apiURL string) string {
	u, err := url.Parse(apiURL)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Hostname()) // strip any :port and normalize case
	switch {
	case prodAPIHostRE.MatchString(host):
		return "app.dash0.com"
	case devAPIHostRE.MatchString(host):
		return "app.dash0-dev.com"
	}
	return ""
}

// writePostLoginFallback handles the rare case where we cannot derive a
// Dash0 web-app host from the API URL (local dev, tests, custom
// deployments). The browser sees a one-line plain-text page; the CLI still
// transitions normally because the redirect target is purely cosmetic.
func writePostLoginFallback(w http.ResponseWriter, res callbackResult) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if res.ErrorCode != "" {
		_, _ = fmt.Fprintf(w, "Dash0 login failed: %s.\nReturn to your terminal for details.\n", res.ErrorCode)
	} else {
		_, _ = w.Write([]byte("Dash0 login complete. You can close this tab.\n"))
	}
}
