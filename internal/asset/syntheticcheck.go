package asset

import (
	"context"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ImportSyntheticCheck creates or updates a synthetic check through
// RouteDocument. A PUT to a deleted synthetic check brings it back, which is
// reported as ActionRestored.
func ImportSyntheticCheck(ctx context.Context, apiClient dash0api.Client, check *dash0api.SyntheticCheckDefinition, dataset *string) (ImportResult, error) {
	plan, err := PlanSyntheticCheck(ctx, apiClient, check, dataset)
	// A failed origin read does not block the write: the PUT reports the same error.
	if plan.Key == "" {
		return ImportResult{}, err
	}
	if plan.Action != ActionCreated {
		setSyntheticCheckOrigin(check, "")
	}
	result, err := apiClient.UpdateSyntheticCheck(ctx, plan.Key, check, dataset)
	if err != nil {
		return ImportResult{}, err
	}
	id := dash0api.GetSyntheticCheckID(result)
	if err := RequireNotDeleted("Synthetic check", dash0api.GetSyntheticCheckName(result), id, SyntheticCheckDeletedAt(result)); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Name: dash0api.GetSyntheticCheckName(result), ID: id, Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanSyntheticCheck routes the synthetic check with RouteDocument and reads
// the stored check that ImportSyntheticCheck would write. It returns any
// preflight error other than "not found"; Key is empty when the id read
// failed, because the write has no target.
func PlanSyntheticCheck(ctx context.Context, apiClient dash0api.Client, check *dash0api.SyntheticCheckDefinition, dataset *string) (ImportPlan, error) {
	// StripSyntheticCheckServerFields clears the origin, so read the identity first.
	identity := SyntheticCheckIdentity(check, dataset)
	dash0api.StripSyntheticCheckServerFields(check)
	get := func(ctx context.Context, key string) (*dash0api.SyntheticCheckDefinition, error) {
		return apiClient.GetSyntheticCheck(ctx, key, dataset)
	}
	route, err := RouteDocument(ctx, identity, get, (*dash0api.SyntheticCheckDefinition).GetOrigin)
	plan := ImportPlan{Key: route.Key, Action: ActionCreated, Proposed: check}
	// An id in the body that is not the path's key can only disagree with it.
	if route.Key != identity.ID {
		dash0api.ClearSyntheticCheckID(check)
	}
	if route.Key == "" || route.Create {
		setSyntheticCheckOrigin(check, route.Key)
		return plan, err
	}
	// The stored origin keeps the diff to what changes; ImportSyntheticCheck
	// clears it before the write.
	setSyntheticCheckOrigin(check, route.Stored.GetOrigin())
	plan.Action, plan.Before = actionForExisting(SyntheticCheckDeletedAt(route.Stored)), route.Stored
	return plan, nil
}

// SyntheticCheckIdentity returns what RouteDocument routes a synthetic check by.
func SyntheticCheckIdentity(check *dash0api.SyntheticCheckDefinition, dataset *string) Identity {
	return identityOf("syntheticcheck", check.Metadata.Name, dataset, check)
}

func setSyntheticCheckOrigin(check *dash0api.SyntheticCheckDefinition, origin string) {
	if check.Metadata.Labels == nil {
		if origin == "" {
			return
		}
		check.Metadata.Labels = &dash0api.SyntheticCheckLabels{}
	}
	check.Metadata.Labels.Dash0Comorigin = optionalString(origin)
}

// SyntheticCheckDeletedAt returns when the synthetic check was deleted, or nil
// when it is not deleted.
func SyntheticCheckDeletedAt(check *dash0api.SyntheticCheckDefinition) *time.Time {
	if check == nil || check.Metadata.Annotations == nil {
		return nil
	}
	return check.Metadata.Annotations.Dash0ComdeletedAt
}
