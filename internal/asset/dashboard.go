package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/google/uuid"
)

// ImportDashboard checks existence by dash0Extensions.id,
// strips server-generated fields when the asset is not found, and calls the
// import API. The import API uses dash0Extensions.id as the upsert key; when
// the input has no id, a fresh UUID is generated so re-applying the exported
// YAML updates the dashboard instead of creating a duplicate.
func ImportDashboard(ctx context.Context, apiClient dash0api.Client, dashboard *dash0api.DashboardDefinition, dataset *string) (ImportResult, error) {
	// Use dash0Extensions.id for the existence check — this is the upsert key
	// used by the import API. Note: metadata.name for dashboards is the display
	// name, not a UUID, so it cannot be used for lookups.

	// Strip server-generated metadata fields — the import API manages these
	// and rejects requests that include stale values (e.g. an outdated version).
	dashboard.Metadata.Annotations = nil
	dashboard.Metadata.CreatedAt = nil
	dashboard.Metadata.UpdatedAt = nil
	dashboard.Metadata.Version = nil

	action := ActionCreated
	id := ""
	if dashboard.Metadata.Dash0Extensions != nil && dashboard.Metadata.Dash0Extensions.Id != nil {
		id = *dashboard.Metadata.Dash0Extensions.Id
	}
	if id != "" {
		_, err := apiClient.GetDashboard(ctx, id, dataset)
		if err == nil {
			action = ActionUpdated
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
	return ImportResult{Name: name, ID: id, Action: action}, nil
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
