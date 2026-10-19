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
	dash0api.StripViewServerFields(view)

	action := ActionCreated
	var before any
	id := ""
	if view.Metadata.Labels != nil && view.Metadata.Labels.Dash0Comid != nil && *view.Metadata.Labels.Dash0Comid != "" {
		id = *view.Metadata.Labels.Dash0Comid
		existing, err := apiClient.GetView(ctx, id, dataset)
		if err == nil {
			action, before = actionForExisting(existing, ViewDeletedAt(existing))
		}
	}

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
	return ImportResult{Name: dash0api.GetViewName(result), ID: id, Action: action, Before: before, After: result}, nil
}

// ViewDeletedAt returns when the view was deleted, or nil when it is not
// deleted.
func ViewDeletedAt(view *dash0api.ViewDefinition) *time.Time {
	if view == nil || view.Metadata.Annotations == nil {
		return nil
	}
	return view.Metadata.Annotations.Dash0ComdeletedAt
}
