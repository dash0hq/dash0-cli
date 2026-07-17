package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ImportTeam creates or upserts a team via the CRD Teams API. When the input
// carries a dash0.com/origin label, PUT is used (create-or-replace by origin).
// Otherwise, POST is used and the server assigns both id and origin.
//
// Before returning, spec.members on both the before and after states is
// translated from internal IDs (what the server echoes) to email addresses,
// so the apply diff renderer prints legible membership changes.
func ImportTeam(ctx context.Context, apiClient dash0api.Client, team *dash0api.TeamDefinitionV1Alpha1) (ImportResult, error) {
	dash0api.StripTeamServerFields(team)

	action := ActionCreated
	var before *dash0api.TeamDefinitionV1Alpha1
	origin := dash0api.GetTeamOrigin(team)
	if origin != "" {
		existing, err := apiClient.GetTeam(ctx, origin)
		if err == nil {
			action = ActionUpdated
			before = existing
		}
	}

	var result *dash0api.TeamDefinitionV1Alpha1
	var err error
	if origin != "" {
		result, err = apiClient.UpsertTeam(ctx, origin, team)
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

	id := dash0api.GetTeamID(result)
	name := dash0api.GetTeamDisplayName(result)
	if name == "" {
		name = dash0api.GetTeamName(result)
	}
	var beforeAny any
	if before != nil {
		beforeAny = before
	}
	return ImportResult{Name: name, ID: id, Action: action, Before: beforeAny, After: result}, nil
}

