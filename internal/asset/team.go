package asset

import (
	"context"
	"fmt"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// validateUpsertLabel rejects values that would be mangled by URL-path
// normalization when passed through GetTeam / UpsertTeam. Empty and
// whitespace-only values are also rejected — GetTeamOrigin/GetTeamID return
// raw label strings without trimming, and an empty upsert key silently
// routes to the create-only path.
func validateUpsertLabel(label, value string) error {
	if value == "" {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "." || trimmed == ".." || strings.ContainsAny(trimmed, "/\\") {
		return fmt.Errorf("invalid %s label value %q — must not be empty, whitespace, \".\", \"..\", or contain \"/\" or \"\\\\\"", label, value)
	}
	return nil
}

// ImportTeam creates or upserts a team via the CRD Teams API.
//
// Upsert key selection:
//
//   - If the input has a user-defined origin (label `dash0.com/origin`), a
//     preflight GetTeam runs against that origin. On hit, PUT is used to
//     update in place. On miss, PUT is still used — the API treats origin
//     PUT as create-or-replace, so the team materializes at the requested
//     origin.
//   - If the input has a user-defined ID (label `dash0.com/id`) but no
//     origin, a preflight GetTeam gates the choice: on hit, PUT (idempotent
//     update); on miss, POST (create fresh with a server-assigned id). The
//     miss path matters for cross-environment apply: a YAML downloaded from
//     one Dash0 org carries an id that does not exist in a different org's
//     backend, and PUT-to-unknown-id returns 404. Falling back to POST keeps
//     `apply` idempotent — the identifier in the file becomes advisory when
//     it cannot be honored.
//   - Otherwise, POST is used and the server assigns both id and origin.
//
// dash0.com/id is captured before StripTeamServerFields runs because that
// helper clears the id label along with the server source label.
//
// Before returning, spec.members on both the before and after states is
// translated from internal IDs (what the server echoes) to email addresses,
// so the apply diff renderer prints legible membership changes.
func ImportTeam(ctx context.Context, apiClient dash0api.Client, team *dash0api.TeamDefinitionV1Alpha1) (ImportResult, error) {
	// Capture identifiers before stripping — StripTeamServerFields clears the
	// dash0.com/id label, so id-based routing must observe the input first.
	origin := dash0api.GetTeamOrigin(team)
	id := dash0api.GetTeamID(team)
	dash0api.StripTeamServerFields(team)

	// Reject label values that would be mangled by URL-path normalization
	// before they reach the API client. url.PathEscape leaves ".", "..", and
	// "/" unescaped, and net/url collapses "." / ".." segments per RFC 3986 —
	// so an id or origin equal to ".", "..", "/", or containing "/" resolves
	// to a completely different endpoint than intended. Fail fast with a
	// legible error rather than silently hitting the list endpoint or an
	// unrelated resource.
	if err := validateUpsertLabel("dash0.com/origin", origin); err != nil {
		return ImportResult{}, err
	}
	if err := validateUpsertLabel("dash0.com/id", id); err != nil {
		return ImportResult{}, err
	}

	action := ActionCreated
	var before *dash0api.TeamDefinitionV1Alpha1
	var upsertKey string
	switch {
	case origin != "":
		upsertKey = origin
		// PUT-by-origin is create-or-replace, so this preflight only affects
		// whether we can render a "before" snapshot in the apply diff — the
		// routing decision is fixed regardless of GET outcome. Any error
		// (404 or transient) is therefore safe to swallow. The id branch
		// below is different: there the GET outcome decides POST vs PUT, so
		// error kind must be inspected.
		if existing, err := apiClient.GetTeam(ctx, origin); err == nil {
			action = ActionUpdated
			before = existing
		}
	case id != "":
		// The preflight GET's outcome decides the route, so the kind of
		// error matters. Only a genuine 404 permits POST fallback (cross-
		// environment apply — the id belongs to another org). Any other
		// error (5xx, auth failure, network blip) must surface — silently
		// POSTing would create a duplicate on the very failure mode this
		// path exists to prevent.
		existing, err := apiClient.GetTeam(ctx, id)
		switch {
		case err == nil:
			upsertKey = id
			action = ActionUpdated
			before = existing
		case dash0api.IsNotFound(err):
			// Fall through to POST.
		default:
			return ImportResult{}, err
		}
	}

	var result *dash0api.TeamDefinitionV1Alpha1
	var err error
	if upsertKey != "" {
		result, err = apiClient.UpsertTeam(ctx, upsertKey, team)
	} else {
		result, err = apiClient.CreateTeam(ctx, team)
	}
	if err != nil {
		return ImportResult{}, err
	}

	// Translate spec.members from IDs to emails on both sides of the diff so
	// membership changes render as "-bob@example.com" rather than opaque UUIDs.
	// Failures are non-fatal — leave raw IDs in place if the members lookup
	// fails for any reason.
	_ = dash0api.ResolveTeamMembersToEmails(ctx, apiClient, before)
	_ = dash0api.ResolveTeamMembersToEmails(ctx, apiClient, result)

	resultID := dash0api.GetTeamID(result)
	name := dash0api.GetTeamDisplayName(result)
	if name == "" {
		name = dash0api.GetTeamName(result)
	}
	var beforeAny any
	if before != nil {
		beforeAny = before
	}
	return ImportResult{Name: name, ID: resultID, Action: action, Before: beforeAny, After: result}, nil
}

