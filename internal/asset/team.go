package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

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

	action := ActionCreated
	var before *dash0api.TeamDefinitionV1Alpha1
	var upsertKey string
	switch {
	case origin != "":
		upsertKey = origin
		if existing, err := apiClient.GetTeam(ctx, origin); err == nil {
			action = ActionUpdated
			before = existing
		}
	case id != "":
		// Only route to PUT if the id actually exists in the target env.
		// Cross-environment applies must not 404; on miss we fall through
		// to POST so the file's id becomes advisory rather than fatal.
		if existing, err := apiClient.GetTeam(ctx, id); err == nil {
			upsertKey = id
			action = ActionUpdated
			before = existing
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

