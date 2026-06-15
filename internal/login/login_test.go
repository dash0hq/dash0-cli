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
