package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/google/uuid"
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

// ImportDashboard checks existence by dash0Extensions.id,
// strips server-generated fields when the asset is not found, and calls the
// import API. The import API uses dash0Extensions.id as the upsert key; when
// the input has no id, a fresh UUID is generated so re-applying the exported
// YAML updates the dashboard instead of creating a duplicate.
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
		} else {
			// Asset not found — strip the ID so the API creates a fresh asset
			// instead of colliding with a soft-deleted record.
			dashboard.Metadata.Dash0Extensions.Id = nil
		}
	}

	// Ensure dash0Extensions.id is set so the import API can perform upserts.
	// When the input has no id (e.g. fresh YAML), generate a unique one. This
	// makes re-applying the exported output update the existing dashboard rather
	// than creating a duplicate.
	if dashboard.Metadata.Dash0Extensions == nil {
		dashboard.Metadata.Dash0Extensions = &dash0api.DashboardMetadataExtensions{}
	}
	if dashboard.Metadata.Dash0Extensions.Id == nil || *dashboard.Metadata.Dash0Extensions.Id == "" {
		id = uuid.New().String()
		dashboard.Metadata.Dash0Extensions.Id = &id
	}

	result, err := apiClient.ImportDashboard(ctx, dashboard, dataset)
	if err != nil {
		return ImportResult{}, err
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
