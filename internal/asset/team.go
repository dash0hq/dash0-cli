package asset

import (
	"context"
	"fmt"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
)

// TeamUsesOrigin reports whether a team document carries a non-empty
// dash0.com/origin label. --since uses it the same way as SLOUsesOrigin: an
// id-only team whose id was not found took ImportTeam's POST fallback, so the
// live team sits at a server-assigned id the document never learned.
func TeamUsesOrigin(data []byte) (bool, error) {
	var team dash0api.TeamDefinitionV1Alpha1
	if err := dash0yaml.Unmarshal(data, &team); err != nil {
		return false, fmt.Errorf("failed to decode team: %w", err)
	}
	return dash0api.GetTeamOrigin(&team) != "", nil
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
	hasOrigin := dash0api.GetTeamOrigin(team) != ""
	plan, err := PlanTeam(ctx, apiClient, team)
	// Only the id path's preflight decides the route, so only it can block the write.
	if err != nil && !hasOrigin {
		return ImportResult{}, err
	}

	var result *dash0api.TeamDefinitionV1Alpha1
	if plan.Key != "" {
		result, err = apiClient.UpsertTeam(ctx, plan.Key, team)
	} else {
		result, err = apiClient.CreateTeam(ctx, team)
	}
	if err != nil {
		return ImportResult{}, err
	}

	// Emails render membership changes as "-bob@example.com" rather than
	// opaque ids. A failed lookup leaves the raw ids in place.
	if before, ok := plan.Before.(*dash0api.TeamDefinitionV1Alpha1); ok {
		_ = dash0api.ResolveTeamMembersToEmails(ctx, apiClient, before)
	}
	_ = dash0api.ResolveTeamMembersToEmails(ctx, apiClient, result)

	name := dash0api.GetTeamDisplayName(result)
	if name == "" {
		name = dash0api.GetTeamName(result)
	}
	return ImportResult{Name: name, ID: dash0api.GetTeamID(result), Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanTeam picks the team's upsert key and reads the stored team that
// ImportTeam would write. Origin wins over id, unless the origin is not found
// and the id is, then the team is updated at the id. On the id path a 404
// leaves Key empty, so the write falls back to POST. It returns any preflight
// error other than "not found".
//
// The server stores member ids, so spec.members on Before holds ids. Callers
// resolve them to emails and decide whether a failed lookup matters.
func PlanTeam(ctx context.Context, apiClient dash0api.Client, team *dash0api.TeamDefinitionV1Alpha1) (ImportPlan, error) {
	// StripTeamServerFields clears the dash0.com/id label, so read it first.
	origin := dash0api.GetTeamOrigin(team)
	id := dash0api.GetTeamID(team)
	lookupKey := PreferredKey(team)
	dash0api.StripTeamServerFields(team)

	plan := ImportPlan{Key: origin, Action: ActionCreated, Proposed: team}
	if lookupKey == "" {
		return plan, nil
	}
	existing, err := apiClient.GetTeam(ctx, lookupKey)
	// An identity-provider origin such as "saml://..." is not addressable,
	// while the exported id of the same team is, so updating at the id keeps
	// apply from creating a second team.
	if dash0api.IsNotFound(err) && lookupKey == origin && id != "" {
		lookupKey = id
		existing, err = apiClient.GetTeam(ctx, id)
	}
	if err != nil {
		return plan, ignoreNotFound(err)
	}
	plan.Key, plan.Action, plan.Before = lookupKey, ActionUpdated, existing
	return plan, nil
}
