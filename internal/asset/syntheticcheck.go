package asset

import (
	"context"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ImportSyntheticCheck creates or updates a synthetic check via the standard CRUD APIs.
// When the input has a user-defined ID, UPDATE is always used — PUT has
// create-or-replace semantics, so this is idempotent regardless of whether the
// check already exists.
// When the input has no ID, CREATE is used and the server assigns an ID.
// A PUT to the ID of a deleted synthetic check brings it back, which is
// reported as ActionRestored.
func ImportSyntheticCheck(ctx context.Context, apiClient dash0api.Client, check *dash0api.SyntheticCheckDefinition, dataset *string) (ImportResult, error) {
	dash0api.StripSyntheticCheckServerFields(check)

	action := ActionCreated
	var before any
	id := ""
	if check.Metadata.Labels != nil && check.Metadata.Labels.Dash0Comid != nil && *check.Metadata.Labels.Dash0Comid != "" {
		id = *check.Metadata.Labels.Dash0Comid
		existing, err := apiClient.GetSyntheticCheck(ctx, id, dataset)
		if err == nil {
			action, before = actionForExisting(SyntheticCheckDeletedAt(existing)), existing
		}
	}

	var (
		result *dash0api.SyntheticCheckDefinition
		err    error
	)
	if id != "" {
		result, err = apiClient.UpdateSyntheticCheck(ctx, id, check, dataset)
	} else {
		result, err = apiClient.CreateSyntheticCheck(ctx, check, dataset)
	}
	if err != nil {
		return ImportResult{}, err
	}
	if err := RequireNotDeleted("synthetic check", dash0api.GetSyntheticCheckName(result), SyntheticCheckDeletedAt(result)); err != nil {
		return ImportResult{}, err
	}

	if result.Metadata.Labels != nil && result.Metadata.Labels.Dash0Comid != nil {
		id = *result.Metadata.Labels.Dash0Comid
	}
	return ImportResult{Name: dash0api.GetSyntheticCheckName(result), ID: id, Action: action, Before: before, After: result}, nil
}

// SyntheticCheckDeletedAt returns when the synthetic check was deleted, or nil
// when it is not deleted.
func SyntheticCheckDeletedAt(check *dash0api.SyntheticCheckDefinition) *time.Time {
	if check == nil || check.Metadata.Annotations == nil {
		return nil
	}
	return check.Metadata.Annotations.Dash0ComdeletedAt
}

// ClearSyntheticCheckDeletedAt removes the deletion timestamp from a synthetic
// check, for example from a file exported with `synthetic-checks get -o yaml`
// while the check was deleted.
func ClearSyntheticCheckDeletedAt(check *dash0api.SyntheticCheckDefinition) {
	if check == nil || check.Metadata.Annotations == nil {
		return
	}
	check.Metadata.Annotations.Dash0ComdeletedAt = nil
}
