package asset

import (
	"context"
	"fmt"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
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
	if err := dash0yaml.Unmarshal(data, &slo); err != nil {
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
//     and PUT-to-unknown-id returns 404. Falling back to POST lets that YAML
//     apply at all, but is a one-way create, not an idempotent one: the server
//     assigns an id the document does not carry, so the next apply preflights
//     the same unknown id, 404s again, and POSTs a second SLO. Only
//     dash0.com/origin makes a repeated apply converge. Any other preflight
//     error (5xx, auth failure, network blip) is surfaced rather than silently
//     POSTed, so a transient hiccup never spawns a duplicate.
//   - Otherwise, POST is used and the server assigns both id and origin.
//
// PUT is create-or-replace, so upserting on either key is idempotent across
// repeated applies. A PUT to a deleted SLO brings it back, which is reported as
// ActionRestored. Both labels are captured before StripSLOServerFields runs,
// because that helper clears dash0.com/origin and dash0.com/id.
func ImportSLO(ctx context.Context, apiClient dash0api.Client, slo *dash0api.SloDefinition, dataset *string) (ImportResult, error) {
	id := dash0api.GetSLOID(slo)
	plan, err := PlanSLO(ctx, apiClient, slo, dataset)
	if err != nil {
		return ImportResult{}, err
	}

	var result *dash0api.SloDefinition
	if plan.Key != "" {
		result, err = apiClient.UpdateSLO(ctx, plan.Key, slo, dataset)
	} else {
		// StripSLOServerFields in PlanSLO already removed dash0.com/id, so the
		// cross-environment POST fallback cannot carry an identifier belonging
		// to the source organization.
		result, err = apiClient.CreateSLO(ctx, slo, dataset)
	}
	if err != nil {
		return ImportResult{}, err
	}

	if resultID := dash0api.GetSLOID(result); resultID != "" {
		id = resultID
	}
	if err := RequireNotDeleted("SLO", dash0api.GetSLOName(result), id, SLODeletedAt(result)); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Name: dash0api.GetSLOName(result), ID: id, Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanSLO picks the SLO's upsert key and reads the stored SLO that ImportSLO
// would write. Origin wins over id. On the id path a 404 leaves Key empty, so
// the write falls back to POST. Any other preflight error is returned: on the
// id path it decides the route, and on the origin path swallowing it would
// report a replace as a create.
func PlanSLO(ctx context.Context, apiClient dash0api.Client, slo *dash0api.SloDefinition, dataset *string) (ImportPlan, error) {
	// StripSLOServerFields clears both labels, so read them first.
	origin := dash0api.GetSLOOrigin(slo)
	id := dash0api.GetSLOID(slo)
	dash0api.StripSLOServerFields(slo)

	plan := ImportPlan{Action: ActionCreated, Proposed: slo}
	lookupKey := origin
	if lookupKey == "" {
		lookupKey = id
	}
	if lookupKey == "" {
		return plan, nil
	}
	if origin != "" {
		plan.Key = origin
	}
	existing, err := apiClient.GetSLO(ctx, lookupKey, dataset)
	if err != nil {
		return plan, ignoreNotFound(err)
	}
	plan.Key = lookupKey
	plan.Action, plan.Before = actionForExisting(SLODeletedAt(existing)), existing
	return plan, nil
}

// SLODeletedAt returns when the SLO was deleted, or nil when it is not deleted.
func SLODeletedAt(slo *dash0api.SloDefinition) *time.Time {
	if slo == nil || slo.Metadata.Annotations == nil {
		return nil
	}
	return slo.Metadata.Annotations.Dash0ComdeletedAt
}
