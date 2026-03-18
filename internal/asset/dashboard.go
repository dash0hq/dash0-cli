package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// StripDashboardServerFields removes server-generated metadata fields from a
// dashboard definition. Used by both Import (to avoid sending stale values to
// the API) and diff rendering (to suppress noise like timestamps and version).
func StripDashboardServerFields(d *dash0api.DashboardDefinition) {
	d.Metadata.Annotations = nil
	d.Metadata.CreatedAt = nil
	d.Metadata.UpdatedAt = nil
	d.Metadata.Version = nil
	if d.Metadata.Dash0Extensions != nil {
		d.Metadata.Dash0Extensions.Dataset = nil
	}
}

// ImportDashboard creates or updates a dashboard via the standard CRUD APIs.
// When the input has a user-defined ID, UPDATE is always used — PUT has
// create-or-replace semantics, so this is idempotent regardless of whether the
// dashboard already exists.
// When the input has no ID, CREATE is used and the server assigns an ID.
func ImportDashboard(ctx context.Context, apiClient dash0api.Client, dashboard *dash0api.DashboardDefinition, dataset *string) (ImportResult, error) {
	StripDashboardServerFields(dashboard)

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
		result, err = apiClient.UpdateDashboard(ctx, id, dashboard, dataset)
	} else {
		result, err = apiClient.CreateDashboard(ctx, dashboard, dataset)
	}
	if err != nil {
		return ImportResult{}, err
	}

	resultID := ExtractDashboardID(result)
	if resultID != "" {
		id = resultID
	}

	name := ExtractDashboardDisplayName(result)
	if name == "" {
		name = result.Metadata.Name
	}
	return ImportResult{Name: name, ID: id, Action: action, Before: before, After: result}, nil
}

// ExtractDashboardID extracts the ID from a dashboard definition.
func ExtractDashboardID(dashboard *dash0api.DashboardDefinition) string {
	if dashboard.Metadata.Dash0Extensions != nil && dashboard.Metadata.Dash0Extensions.Id != nil && *dashboard.Metadata.Dash0Extensions.Id != "" {
		return *dashboard.Metadata.Dash0Extensions.Id
	}
	return ""
}

// ExtractDashboardDisplayName extracts the display name from a dashboard definition.
func ExtractDashboardDisplayName(dashboard *dash0api.DashboardDefinition) string {
	if dashboard == nil || dashboard.Spec == nil {
		return ""
	}

	display, ok := dashboard.Spec["display"].(map[string]interface{})
	if !ok {
		return ""
	}

	name, ok := display["name"].(string)
	if !ok {
		return ""
	}

	return name
}
