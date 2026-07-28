package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ImportSLO creates or upserts an SLO via the standard CRUD APIs.
//
// Upsert key selection mirrors ImportTeam. The SLO API routes GET, PUT, and
// DELETE by origin-or-id (unlike dashboards, views, check rules, and synthetic
// checks, where the server treats dash0.com/origin as provenance metadata and
// it must not be used as an upsert key). So:
//
//   - Prefer the stable dash0.com/id when present.
//   - Fall back to dash0.com/origin when there is no id. An origin-only
//     document (e.g. a UI CR download) must upsert via PUT on that origin
//     rather than POST a fresh duplicate on every apply — the #227 team bug.
//   - Only POST (server assigns id and origin) when neither is present.
//
// PUT is create-or-replace, so upserting on either key is idempotent across
// repeated applies. The origin label is captured before StripSLOServerFields
// runs because that helper clears dash0.com/origin along with the other
// server-managed labels.
func ImportSLO(ctx context.Context, apiClient dash0api.Client, slo *dash0api.SloDefinition, dataset *string) (ImportResult, error) {
	// Capture identifiers before stripping — StripSLOServerFields clears the
	// dash0.com/origin label, so origin-based routing must observe the input
	// first.
	origin := ""
	if slo.Metadata.Labels != nil && slo.Metadata.Labels.Dash0Comorigin != nil {
		origin = *slo.Metadata.Labels.Dash0Comorigin
	}
	id := dash0api.GetSLOID(slo)
	dash0api.StripSLOServerFields(slo)

	var upsertKey string
	switch {
	case id != "":
		upsertKey = id
	case origin != "":
		upsertKey = origin
	}

	action := ActionCreated
	var before any
	if upsertKey != "" {
		existing, err := apiClient.GetSLO(ctx, upsertKey, dataset)
		if err == nil {
			action = ActionUpdated
			before = existing
		}
	}

	var result *dash0api.SloDefinition
	var err error
	if upsertKey != "" {
		result, err = apiClient.UpdateSLO(ctx, upsertKey, slo, dataset)
	} else {
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
