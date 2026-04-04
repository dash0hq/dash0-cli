package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ImportDashboard creates or updates a dashboard via the standard CRUD APIs.
// When the input has a user-defined ID, UPDATE is always used — PUT has
// create-or-replace semantics, so this is idempotent regardless of whether the
// dashboard already exists.
// When the input has no ID, CREATE is used and the server assigns an ID.
func ImportDashboard(ctx context.Context, apiClient dash0api.Client, dashboard *dash0api.DashboardDefinition, dataset *string) (ImportResult, error) {
	dash0api.StripDashboardServerFields(dashboard)

	action := ActionCreated
	var before any
	id := ""
	if dashboard.Metadata.Dash0Extensions != nil && dashboard.Metadata.Dash0Extensions.Id != nil {
		id = *dashboard.Metadata.Dash0Extensions.Id
	}
	if id != "" {
		existing, err := apiClient.GetDashboard(ctx, id, dataset)
		if err == nil {
			action = ActionUpdated
			before = existing
		}
	}

	var result *dash0api.DashboardDefinition
	var err error
	if id != "" {
		dash0api.ClearDashboardID(dashboard)
		result, err = apiClient.UpdateDashboard(ctx, id, dashboard, dataset)
	} else {
		result, err = apiClient.CreateDashboard(ctx, dashboard, dataset)
	}
	if err != nil {
		return ImportResult{}, err
	}

	resultID := dash0api.GetDashboardID(result)
	if resultID != "" {
		id = resultID
	}

	name := dash0api.GetDashboardName(result)
	if name == "" {
		name = result.Metadata.Name
	}
	return ImportResult{Name: name, ID: id, Action: action, Before: before, After: result}, nil
}

