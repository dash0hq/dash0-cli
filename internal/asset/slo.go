package asset

import (
	"context"
	"fmt"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	sigsyaml "sigs.k8s.io/yaml"
)

// SLOUsesOrigin reports whether an SLO document carries a non-empty
// dash0.com/origin label.
//
// --since uses this to warn when an SLO is about to be deleted by
// dash0.com/id alone. SLO ids are server-assigned, so an id-only document
// that was first applied against an organization not holding that id took
// ImportSLO's POST fallback, and the live SLO is sitting at a fresh id the
// document never learned. Deleting by the id recorded in git history then
// either 404s (hard-failing without --force) or counts as already-deleted
// (with --force), leaving the real SLO orphaned either way. There is no
// local, API-free way to recover the live id from git history; origin is the
// only identifier the server never reassigns.
func SLOUsesOrigin(data []byte) (bool, error) {
	var slo dash0api.SloDefinition
	if err := sigsyaml.Unmarshal(data, &slo); err != nil {
		return false, fmt.Errorf("failed to decode SLO: %w", err)
	}
	return dash0api.GetSLOOrigin(&slo) != "", nil
}

// ImportSLO creates or upserts an SLO via the standard CRUD APIs.
//
// Upsert key selection mirrors ImportTeam (origin-first, preflight-driven):
//
//   - If the input has a user-defined origin (label `dash0.com/origin`), a
//     preflight GetSLO runs against that origin. On hit, PUT is used to update
//     in place. On a genuine 404, PUT is still used — the API treats an origin
//     PUT as create-or-replace, so the SLO materializes at the requested
//     origin. Any other preflight error is surfaced.
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
		// The route is PUT either way, so the preflight only decides "created"
		// vs "updated" — but swallowing a 5xx there reports a replace as a
		// create with no diff, claiming a write the run did not make.
		existing, err := apiClient.GetSLO(ctx, origin, dataset)
		switch {
		case err == nil:
			action = ActionUpdated
			before = existing
		case dash0api.IsNotFound(err):
			// Nothing at this origin yet; the PUT below creates it.
		default:
			return ImportResult{}, err
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
