package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ImportSLO creates or updates an SLO via the standard CRUD APIs.
// When the input has a user-defined ID, UPDATE is always used — PUT has
// create-or-replace semantics, so this is idempotent regardless of whether the
// SLO already exists.
// When the input has no ID, CREATE is used and the server assigns an ID.
func ImportSLO(ctx context.Context, apiClient dash0api.Client, slo *dash0api.SloDefinition, dataset *string) (ImportResult, error) {
	dash0api.StripSLOServerFields(slo)

	action := ActionCreated
	var before any
	id := dash0api.GetSLOID(slo)
	if id != "" {
		existing, err := apiClient.GetSLO(ctx, id, dataset)
		if err == nil {
			action = ActionUpdated
			before = existing
		}
	}

	var result *dash0api.SloDefinition
	var err error
	if id != "" {
		result, err = apiClient.UpdateSLO(ctx, id, slo, dataset)
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
