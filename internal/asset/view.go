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

// ImportView checks existence by Dash0Comid, strips server-generated fields,
// and calls the import API.
func ImportView(ctx context.Context, apiClient dash0api.Client, view *dash0api.ViewDefinition, dataset *string) (ImportResult, error) {
	StripViewServerFields(view)

	// Check if view exists using its ID
	action := ActionCreated
	var before any
	if view.Metadata.Labels.Dash0Comid != nil && *view.Metadata.Labels.Dash0Comid != "" {
		existing, err := apiClient.GetView(ctx, *view.Metadata.Labels.Dash0Comid, dataset)
		if err == nil {
			action = ActionUpdated
			before = existing
		} else {
			// Asset not found — strip the ID so the API creates a fresh asset.
			view.Metadata.Labels.Dash0Comid = nil
		}
	}

	result, err := apiClient.ImportView(ctx, view, dataset)
	if err != nil {
		return ImportResult{}, err
	}

	id := ""
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
