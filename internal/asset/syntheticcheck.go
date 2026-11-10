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
	// A failed preflight does not block the write: the PUT reports the same error.
	plan, _ := PlanSyntheticCheck(ctx, apiClient, check, dataset)
	id := plan.Key

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

	if result.Metadata.Labels != nil && result.Metadata.Labels.Dash0Comid != nil {
		id = *result.Metadata.Labels.Dash0Comid
	}
	if err := RequireNotDeleted("Synthetic check", dash0api.GetSyntheticCheckName(result), id, SyntheticCheckDeletedAt(result)); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Name: dash0api.GetSyntheticCheckName(result), ID: id, Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanSyntheticCheck reads the stored synthetic check that ImportSyntheticCheck
// would write. It returns any preflight error other than "not found".
func PlanSyntheticCheck(ctx context.Context, apiClient dash0api.Client, check *dash0api.SyntheticCheckDefinition, dataset *string) (ImportPlan, error) {
	dash0api.StripSyntheticCheckServerFields(check)
	plan := ImportPlan{Action: ActionCreated, Proposed: check}
	if check.Metadata.Labels == nil || check.Metadata.Labels.Dash0Comid == nil || *check.Metadata.Labels.Dash0Comid == "" {
		return plan, nil
	}
	plan.Key = *check.Metadata.Labels.Dash0Comid
	existing, err := apiClient.GetSyntheticCheck(ctx, plan.Key, dataset)
	if err != nil {
		return plan, ignoreNotFound(err)
	}
	plan.Action, plan.Before = actionForExisting(SyntheticCheckDeletedAt(existing)), existing
	return plan, nil
}

// SyntheticCheckDeletedAt returns when the synthetic check was deleted, or nil
// when it is not deleted.
func SyntheticCheckDeletedAt(check *dash0api.SyntheticCheckDefinition) *time.Time {
	if check == nil || check.Metadata.Annotations == nil {
		return nil
	}
	return check.Metadata.Annotations.Dash0ComdeletedAt
}
