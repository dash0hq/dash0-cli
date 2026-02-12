package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ImportSyntheticCheck checks existence by Dash0Comid, strips server-generated
// fields, and calls the import API.
func ImportSyntheticCheck(ctx context.Context, apiClient dash0api.Client, check *dash0api.SyntheticCheckDefinition, dataset *string) (ImportResult, error) {
	// Strip server-generated metadata fields — the import API manages these.
	check.Metadata.Annotations = nil
	if check.Metadata.Labels == nil {
		check.Metadata.Labels = &dash0api.SyntheticCheckLabels{}
	}
	check.Metadata.Labels.Dash0Comversion = nil
	check.Metadata.Labels.Custom = nil
	check.Metadata.Labels.Dash0Comdataset = nil
	check.Spec.Permissions = nil
	// Strip origin — the import API manages it server-side and rejects any
	// value submitted in the request body.
	check.Metadata.Labels.Dash0Comorigin = nil

	// Check if synthetic check exists using its ID
	action := ActionCreated
	if check.Metadata.Labels.Dash0Comid != nil && *check.Metadata.Labels.Dash0Comid != "" {
		_, err := apiClient.GetSyntheticCheck(ctx, *check.Metadata.Labels.Dash0Comid, dataset)
		if err == nil {
			action = ActionUpdated
		} else {
			// Asset not found — strip the ID so the API creates a fresh asset.
			check.Metadata.Labels.Dash0Comid = nil
		}
	}

	result, err := apiClient.ImportSyntheticCheck(ctx, check, dataset)
	if err != nil {
		return ImportResult{}, err
	}

	id := ""
	if result.Metadata.Labels != nil && result.Metadata.Labels.Dash0Comid != nil {
		id = *result.Metadata.Labels.Dash0Comid
	}
	return ImportResult{Name: result.Metadata.Name, ID: id, Action: action}, nil
}
