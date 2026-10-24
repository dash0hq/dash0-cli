package login

import "testing"

// TestExpiresInCap guards the maxAccessTokenLifetimeSeconds constant from
// silent drift. The CAP LOGIC itself is exercised by the integration test
// TestRunLogin_ExpiresInCappedAt24h (which drives a fake AS returning a
// year-long expires_in and asserts the persisted ExpiresAt is bounded to
// ~24h). Both tests must pass: this one catches a renamed/deleted const,
// the integration test catches a deleted cap branch.
func TestExpiresInCap(t *testing.T) {
	if maxAccessTokenLifetimeSeconds != 86400 {
		t.Fatalf("maxAccessTokenLifetimeSeconds drifted from 24h; bump tests if intentional")
	}
}

// TestEnsureRegisteredClient_Dash0HostedUsesFirstPartyClient checks that a
// Dash0-hosted API URL never reaches dynamic client registration: the nil
// OAuth client would panic if it did.
func TestEnsureRegisteredClient_Dash0HostedUsesFirstPartyClient(t *testing.T) {
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())
	const redirectURI = "http://127.0.0.1:54321/callback"

	for _, apiURL := range []string{
		"https://api.eu-west-1.aws.dash0.com",
		"https://api.dash0.com",
		"https://api.us-west-2.aws.dash0-dev.com",
	} {
		t.Run(apiURL, func(t *testing.T) {
			rec, err := ensureRegisteredClient(t.Context(), nil, apiURL, redirectURI)
			if err != nil {
				t.Fatalf("ensureRegisteredClient(%q) returned an error: %v", apiURL, err)
			}
			if rec.ClientID != firstPartyClientID {
				t.Errorf("ClientID = %q, want %q", rec.ClientID, firstPartyClientID)
			}
			if rec.RedirectURI != redirectURI {
				t.Errorf("RedirectURI = %q, want %q", rec.RedirectURI, redirectURI)
			}
		})
	}
}
