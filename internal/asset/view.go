package asset

import (
	"context"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ImportView creates or updates a view through RouteDocument. A PUT to a
// deleted view brings it back, which is reported as ActionRestored.
func ImportView(ctx context.Context, apiClient dash0api.Client, view *dash0api.ViewDefinition, dataset *string) (ImportResult, error) {
	plan, err := PlanView(ctx, apiClient, view, dataset)
	// A failed origin read does not block the write: the PUT reports the same error.
	if plan.Key == "" {
		return ImportResult{}, err
	}
	// An update at the id never sends the origin, which the API rejects there.
	// An update at the stored origin sends it back: without it, Dash0 leaves
	// the origin out of a later read by id.
	if plan.Action != ActionCreated && view.GetOrigin() != plan.Key {
		setViewOrigin(view, "")
	}
	result, err := apiClient.UpdateView(ctx, plan.Key, view, dataset)
	if err != nil {
		return ImportResult{}, err
	}
	id := dash0api.GetViewID(result)
	if err := RequireNotDeleted("View", dash0api.GetViewName(result), id, ViewDeletedAt(result)); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Name: dash0api.GetViewName(result), ID: id, Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanView routes the view with RouteDocument and reads the stored view that
// ImportView would write. It returns any preflight error other than "not
// found"; Key is empty when the id read failed, because the write has no target.
func PlanView(ctx context.Context, apiClient dash0api.Client, view *dash0api.ViewDefinition, dataset *string) (ImportPlan, error) {
	// StripViewServerFields clears the origin, so read the identity first.
	identity := ViewIdentity(view, dataset)
	dash0api.StripViewServerFields(view)
	get := func(ctx context.Context, key string) (*dash0api.ViewDefinition, error) {
		return apiClient.GetView(ctx, key, dataset)
	}
	route, err := RouteDocument(ctx, identity, get, (*dash0api.ViewDefinition).GetOrigin)
	plan := ImportPlan{Key: route.Key, Action: ActionCreated, Proposed: view}
	// An id in the body that is not the path's key can only disagree with it.
	if route.Key != identity.ID {
		dash0api.ClearViewID(view)
	}
	if route.Key == "" || route.Create {
		setViewOrigin(view, route.Key)
		return plan, err
	}
	// The stored origin keeps the diff to what changes; the import clears it
	// before a write at the id.
	setViewOrigin(view, route.Stored.GetOrigin())
	plan.Action, plan.Before = actionForExisting(ViewDeletedAt(route.Stored)), route.Stored
	return plan, nil
}

// ViewIdentity returns what RouteDocument routes a view by.
func ViewIdentity(view *dash0api.ViewDefinition, dataset *string) Identity {
	return identityOf("view", view.Metadata.Name, dataset, view)
}

func setViewOrigin(view *dash0api.ViewDefinition, origin string) {
	if view.Metadata.Labels == nil {
		if origin == "" {
			return
		}
		view.Metadata.Labels = &dash0api.ViewLabels{}
	}
	view.Metadata.Labels.Dash0Comorigin = optionalString(origin)
}

// ViewDeletedAt returns when the view was deleted, or nil when it is not
// deleted.
func ViewDeletedAt(view *dash0api.ViewDefinition) *time.Time {
	if view == nil || view.Metadata.Annotations == nil {
		return nil
	}
	return view.Metadata.Annotations.Dash0ComdeletedAt
}
