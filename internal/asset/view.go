package asset

import (
	"context"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ImportView creates or updates a view via the standard CRUD APIs.
// When the input has a user-defined ID, UPDATE is always used — PUT has
// create-or-replace semantics, so this is idempotent regardless of whether the
// view already exists.
// When the input has no ID, CREATE is used and the server assigns an ID.
// A PUT to the ID of a deleted view brings it back, which is reported as
// ActionRestored.
func ImportView(ctx context.Context, apiClient dash0api.Client, view *dash0api.ViewDefinition, dataset *string) (ImportResult, error) {
	// A failed preflight does not block the write: the PUT reports the same error.
	plan, _ := PlanView(ctx, apiClient, view, dataset)
	id := plan.Key

	var (
		result *dash0api.ViewDefinition
		err    error
	)
	if id != "" {
		result, err = apiClient.UpdateView(ctx, id, view, dataset)
	} else {
		result, err = apiClient.CreateView(ctx, view, dataset)
	}
	if err != nil {
		return ImportResult{}, err
	}

	if result.Metadata.Labels != nil && result.Metadata.Labels.Dash0Comid != nil {
		id = *result.Metadata.Labels.Dash0Comid
	}
	if err := RequireNotDeleted("View", dash0api.GetViewName(result), id, ViewDeletedAt(result)); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Name: dash0api.GetViewName(result), ID: id, Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanView reads the stored view that ImportView would write.
// It returns any preflight error other than "not found".
func PlanView(ctx context.Context, apiClient dash0api.Client, view *dash0api.ViewDefinition, dataset *string) (ImportPlan, error) {
	dash0api.StripViewServerFields(view)
	plan := ImportPlan{Action: ActionCreated, Proposed: view}
	if view.Metadata.Labels == nil || view.Metadata.Labels.Dash0Comid == nil || *view.Metadata.Labels.Dash0Comid == "" {
		return plan, nil
	}
	plan.Key = *view.Metadata.Labels.Dash0Comid
	existing, err := apiClient.GetView(ctx, plan.Key, dataset)
	if err != nil {
		return plan, ignoreNotFound(err)
	}
	plan.Action, plan.Before = actionForExisting(ViewDeletedAt(existing)), existing
	return plan, nil
}

// ViewDeletedAt returns when the view was deleted, or nil when it is not
// deleted.
func ViewDeletedAt(view *dash0api.ViewDefinition) *time.Time {
	if view == nil || view.Metadata.Annotations == nil {
		return nil
	}
	return view.Metadata.Annotations.Dash0ComdeletedAt
}
