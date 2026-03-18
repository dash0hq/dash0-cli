package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// StripViewServerFields removes server-generated fields from a view definition.
// Used by both Import (to avoid sending rejected fields to the API) and diff
// rendering (to suppress noise).
func StripViewServerFields(v *dash0api.ViewDefinition) {
	v.Metadata.Annotations = nil
	if v.Metadata.Labels == nil {
		v.Metadata.Labels = &dash0api.ViewLabels{}
	}
	v.Metadata.Labels.Dash0Comversion = nil
	v.Metadata.Labels.Dash0Comsource = nil
	v.Metadata.Labels.Dash0Comdataset = nil
	v.Metadata.Labels.Dash0Comorigin = nil
	v.Spec.Permissions = nil
}

// ImportView creates or updates a view via the standard CRUD APIs.
// When the input has a user-defined ID, UPDATE is always used — PUT has
// create-or-replace semantics, so this is idempotent regardless of whether the
// view already exists.
// When the input has no ID, CREATE is used and the server assigns an ID.
func ImportView(ctx context.Context, apiClient dash0api.Client, view *dash0api.ViewDefinition, dataset *string) (ImportResult, error) {
	StripViewServerFields(view)

	action := ActionCreated
	var before any
	id := ""
	if view.Metadata.Labels.Dash0Comid != nil && *view.Metadata.Labels.Dash0Comid != "" {
		id = *view.Metadata.Labels.Dash0Comid
		existing, err := apiClient.GetView(ctx, id, dataset)
		if err == nil {
			action = ActionUpdated
			before = existing
		}
	}

	var result *dash0api.ViewDefinition
	var err error
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
	return ImportResult{Name: ExtractViewName(result), ID: id, Action: action, Before: before, After: result}, nil
}

// ExtractViewID extracts the ID from a view definition.
func ExtractViewID(view *dash0api.ViewDefinition) string {
	if view.Metadata.Labels != nil && view.Metadata.Labels.Dash0Comid != nil {
		return *view.Metadata.Labels.Dash0Comid
	}
	return ""
}

// ExtractViewName extracts the display name from a view definition, falling
// back to metadata.name if the display name is empty.
func ExtractViewName(view *dash0api.ViewDefinition) string {
	if view.Spec.Display.Name != "" {
		return view.Spec.Display.Name
	}
	return view.Metadata.Name
}
