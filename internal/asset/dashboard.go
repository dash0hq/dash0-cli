package asset

import (
	"context"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ImportDashboard creates or updates a dashboard via the standard CRUD APIs.
// When the input has a user-defined ID, UPDATE is always used — PUT has
// create-or-replace semantics, so this is idempotent regardless of whether the
// dashboard already exists.
// When the input has no ID, CREATE is used and the server assigns an ID.
// A PUT to the ID of a deleted dashboard brings it back, which is reported as
// ActionRestored.
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
			action, before = actionForExisting(DashboardDeletedAt(existing)), existing
		}
	}

	var (
		result *dash0api.DashboardDefinition
		err    error
	)
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
	if err := RequireNotDeleted("dashboard", name, DashboardDeletedAt(result)); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Name: name, ID: id, Action: action, Before: before, After: result}, nil
}

// DashboardDeletedAt returns when the dashboard was deleted, or nil when it is
// not deleted.
func DashboardDeletedAt(dashboard *dash0api.DashboardDefinition) *time.Time {
	if dashboard == nil || dashboard.Metadata.Annotations == nil {
		return nil
	}
	return dashboard.Metadata.Annotations.Dash0ComdeletedAt
}

// ClearDashboardDeletedAt removes the deletion timestamp from a dashboard, for
// example from a file exported with `dashboards get -o yaml` while the
// dashboard was deleted.
func ClearDashboardDeletedAt(dashboard *dash0api.DashboardDefinition) {
	if dashboard == nil || dashboard.Metadata.Annotations == nil {
		return
	}
	dashboard.Metadata.Annotations.Dash0ComdeletedAt = nil
}
