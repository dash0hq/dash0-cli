package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ImportSLO creates or upserts an SLO via the standard CRUD APIs.
//
// Upsert key selection mirrors ImportTeam (origin-first, preflight-driven):
//
//   - If the input has a user-defined origin (label `dash0.com/origin`), a
//     preflight GetSLO runs against that origin. On hit, PUT is used to update
//     in place. On miss, PUT is still used — the API treats an origin PUT as
//     create-or-replace, so the SLO materializes at the requested origin.
//   - If the input has a user-defined ID (label `dash0.com/id`) but no origin,
//     a preflight GetSLO gates the choice: on hit, PUT (idempotent update); on
//     a genuine 404, POST (create fresh with a server-assigned id). The miss
//     path matters for cross-environment apply: a YAML downloaded from one
//     Dash0 org carries an id that does not exist in a different org's backend,
//     and PUT-to-unknown-id returns 404. Falling back to POST keeps `apply`
//     idempotent — the identifier in the file becomes advisory when it cannot
//     be honored. Any other preflight error (5xx, auth failure, network blip)
//     is surfaced rather than silently POSTed, so a transient hiccup never
//     spawns a duplicate.
//   - Otherwise, POST is used and the server assigns both id and origin.
//
// PUT is create-or-replace, so upserting on either key is idempotent across
// repeated applies. Both labels are captured before StripSLOServerFields runs,
// because that helper clears dash0.com/origin and dash0.com/id.
func ImportSLO(ctx context.Context, apiClient dash0api.Client, slo *dash0api.SloDefinition, dataset *string) (ImportResult, error) {
	// Capture identifiers before stripping — StripSLOServerFields clears both
	// the dash0.com/origin and dash0.com/id labels, so origin- and id-based
	// routing must observe the input first.
	origin := dash0api.GetSLOOrigin(slo)
	id := dash0api.GetSLOID(slo)
	dash0api.StripSLOServerFields(slo)

	action := ActionCreated
	var before any
	var upsertKey string
	switch {
	case origin != "":
		upsertKey = origin
		if existing, err := apiClient.GetSLO(ctx, origin, dataset); err == nil {
			action = ActionUpdated
			before = existing
		}
	case id != "":
		// The preflight GET's outcome decides the route, so the kind of error
		// matters. Only a genuine 404 permits POST fallback (cross-environment
		// apply — the id belongs to another org). Any other error (5xx, auth
		// failure, network blip) must surface — silently POSTing would create a
		// duplicate on the very failure mode this path exists to prevent.
		existing, err := apiClient.GetSLO(ctx, id, dataset)
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

	var result *dash0api.SloDefinition
	var err error
	if upsertKey != "" {
		result, err = apiClient.UpdateSLO(ctx, upsertKey, slo, dataset)
	} else {
		// No explicit ClearSLOID here: StripSLOServerFields above already
		// removed dash0.com/id, so the cross-environment POST fallback cannot
		// carry an identifier belonging to the source organization.
		result, err = apiClient.CreateSLO(ctx, slo, dataset)
	}
	if err != nil {
		return ImportResult{}, err
	}

	if resultID := dash0api.GetSLOID(result); resultID != "" {
		id = resultID
	}
	return ImportResult{Name: dash0api.GetSLOName(result), ID: id, Action: action, Before: before, After: result}, nil
}
