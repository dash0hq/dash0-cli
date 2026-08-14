// Package oauth holds OAuth helpers shared across the CLI: refresh-token
// revocation used by `login`, `logout`, and `config profiles update
// --oauth=false`; and sanitization of OAuth-server-supplied strings before
// they reach a terminal.
//
// Why a separate package: `internal/login` already imports `internal/config`
// for profile-hint helpers, so neither package can host shared OAuth code
// without an import cycle. A neutral package keeps both callers honest.
package oauth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/version"
)

// revokeTimeout bounds the revocation HTTP call. A slow or unresponsive
// revocation endpoint must never block the calling command past this budget
// — the local state has already been updated by the time the revoke runs.
const revokeTimeout = 5 * time.Second

// Revoke posts a revocation request for refreshToken, as clientID, against
// apiURL. Errors are logged to stderr with a `warning:` prefix; the function
// returns true on success (or no-op) and false on failure so callers can
// optionally append a note to their success message. Callers rely on
// this function returning promptly regardless of outcome.
// No-ops (and returns true) when refreshToken or apiURL is empty.
//
// Uses the generated client's raw-body form-data method instead of the
// typed [dash0api.OAuthClient.RevokeToken] because dash0-api-client-go's
// OAuthRevocationRequest has no ClientId field yet; switch back to the
// typed call once it does.
func Revoke(apiURL, clientID, refreshToken string) (ok bool) {
	if refreshToken == "" || apiURL == "" {
		return true
	}
	userAgentEditor := func(_ context.Context, req *http.Request) error {
		req.Header.Set("User-Agent", version.UserAgent())
		return nil
	}
	inner, err := dash0api.NewClientWithResponses(apiURL, dash0api.WithRequestEditorFn(userAgentEditor))
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to construct OAuth client to revoke refresh token: %v\n", err)
		return false
	}
	form := url.Values{
		"token":           {refreshToken},
		"token_type_hint": {"refresh_token"},
		"client_id":       {clientID},
	}
	ctx, cancel := context.WithTimeout(context.Background(), revokeTimeout)
	defer cancel()
	resp, err := inner.PostOauthRevokeWithBodyWithResponse(ctx, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: refresh token revocation failed (it may already be invalid): %v\n", err)
		return false
	}
	if status := resp.StatusCode(); status < 200 || status >= 300 {
		fmt.Fprintf(os.Stderr, "warning: refresh token revocation failed (it may already be invalid): unexpected status %s\n", resp.Status())
		return false
	}
	return true
}
