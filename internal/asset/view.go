package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ImportView checks existence by Dash0Comid, strips server-generated fields,
// and calls the import API.
func ImportView(ctx context.Context, apiClient dash0api.Client, view *dash0api.ViewDefinition, dataset *string) (ImportResult, error) {
	// Strip server-generated metadata and user-specific fields — the import
	// API manages these and rejects requests that include them.
	view.Metadata.Annotations = nil
	if view.Metadata.Labels == nil {
		view.Metadata.Labels = &dash0api.ViewLabels{}
	}
	view.Metadata.Labels.Dash0Comversion = nil
	view.Metadata.Labels.Dash0Comsource = nil
	view.Metadata.Labels.Dash0Comdataset = nil
	view.Spec.Permissions = nil

	// Strip origin — the import API manages it server-side and rejects any
	// value submitted in the request body.
	view.Metadata.Labels.Dash0Comorigin = nil

	// Check if view exists using its ID
	action := ActionCreated
	if view.Metadata.Labels.Dash0Comid != nil && *view.Metadata.Labels.Dash0Comid != "" {
		_, err := apiClient.GetView(ctx, *view.Metadata.Labels.Dash0Comid, dataset)
		if err == nil {
			action = ActionUpdated
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
	return ImportResult{Name: result.Metadata.Name, ID: id, Action: action}, nil
}
