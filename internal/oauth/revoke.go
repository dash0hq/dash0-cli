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
	"os"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/version"
)

// revokeTimeout bounds the revocation HTTP call. A slow or unresponsive
// revocation endpoint must never block the calling command past this budget
// — the local state has already been updated by the time the revoke runs.
const revokeTimeout = 5 * time.Second

// RevokeRequest is the input to [Revoke]. Named fields keep API URL, client
// ID, and refresh token from being swapped at the six call sites.
//
// ClientID is used verbatim. [Revoke] deliberately does not fall back to the
// DCR client cache when it is empty: that cache is keyed by API URL alone, so
// on the re-login path a fresh registration has already overwritten the entry
// by the time the old token is revoked, and the revoke would go out under a
// client that never issued it — the AS answers 200 without revoking anything.
// Callers that read a stored profile resolve the client ID at read time
// through profiles.ResolveOAuthClientID, before any registration runs.
type RevokeRequest struct {
	APIURL       string
	ClientID     string
	RefreshToken string
}

// Revoke posts a revocation request for req.RefreshToken, as req.ClientID,
// against req.APIURL. Errors are logged to stderr with a `warning:` prefix;
// the function returns true on success (or no-op) and false on failure so
// callers can optionally append a note to their success message. The
// function performs one bounded network call and no disk I/O, so callers can
// rely on it returning promptly regardless of outcome.
// No-ops (and returns true) when RefreshToken or APIURL is empty.
func Revoke(req RevokeRequest) (ok bool) {
	if req.RefreshToken == "" || req.APIURL == "" {
		return true
	}
	client, err := dash0api.NewOAuthClient(
		dash0api.WithApiUrl(req.APIURL),
		dash0api.WithUserAgent(version.UserAgent()),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to construct OAuth client to revoke refresh token: %v\n", err)
		return false
	}
	defer func() { _ = client.Close(context.Background()) }()
	ctx, cancel := context.WithTimeout(context.Background(), revokeTimeout)
	defer cancel()
	hint := dash0api.OAuthTokenTypeRefreshToken
	revokeReq := &dash0api.OAuthRevocationRequest{
		Token:         req.RefreshToken,
		TokenTypeHint: &hint,
	}
	if req.ClientID != "" {
		revokeReq.ClientId = req.ClientID
	}
	if err := client.RevokeToken(ctx, revokeReq); err != nil {
		fmt.Fprintf(os.Stderr, "warning: refresh token revocation failed (it may already be invalid): %v\n", err)
		return false
	}
	return true
}
