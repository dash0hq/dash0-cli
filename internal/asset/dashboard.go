package asset

import (
	"context"
	"fmt"
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
	if err := ValidateDashboard(dashboard); err != nil {
		return ImportResult{}, err
	}
	// A failed preflight does not block the write: the PUT reports the same error.
	plan, _ := PlanDashboard(ctx, apiClient, dashboard, dataset)
	id := plan.Key

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

	resultID := DashboardKey(result)
	if resultID != "" {
		id = resultID
	}

	name := dash0api.GetDashboardName(result)
	if name == "" {
		name = result.Metadata.Name
	}
	if err := RequireNotDeleted("Dashboard", name, id, DashboardDeletedAt(result)); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Name: name, ID: id, Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanDashboard reads the stored dashboard that ImportDashboard would write.
// It returns any preflight error other than "not found".
func PlanDashboard(ctx context.Context, apiClient dash0api.Client, dashboard *dash0api.DashboardDefinition, dataset *string) (ImportPlan, error) {
	if err := ValidateDashboard(dashboard); err != nil {
		return ImportPlan{}, err
	}
	// Read the key first: the strip clears the dash0.com/id label.
	key := DashboardKey(dashboard)
	dash0api.StripDashboardServerFields(dashboard)
	plan := ImportPlan{Key: key, Action: ActionCreated, Proposed: dashboard}
	if key == "" {
		return plan, nil
	}
	existing, err := apiClient.GetDashboard(ctx, plan.Key, dataset)
	if err != nil {
		return plan, ignoreNotFound(err)
	}
	plan.Action, plan.Before = actionForExisting(DashboardDeletedAt(existing)), existing
	return plan, nil
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

// DashboardKeys lets the shared key rules resolve a dashboard's identifiers in
// the order apply upserts by: the dash0.com/origin label, then
// metadata.dash0Extensions.id, then the dash0.com/id label. The client's own
// GetID reads the dash0.com/id label first, which would put the server UUID
// ahead of the id the user chose. Read it before StripDashboardServerFields,
// which clears the dash0.com/id label.
func DashboardKeys(dashboard *dash0api.DashboardDefinition) dash0api.AssetMetadata {
	id := dashboardExtensionsID(dashboard)
	if id == "" {
		id = dash0api.GetDashboardID(dashboard)
	}
	return dashboardKeys{origin: dashboard.GetOrigin(), id: id, name: dash0api.GetDashboardName(dashboard)}
}

type dashboardKeys struct{ origin, id, name string }

func (k dashboardKeys) GetOrigin() string               { return k.origin }
func (k dashboardKeys) GetID() string                   { return k.id }
func (k dashboardKeys) GetName() string                 { return k.name }
func (dashboardKeys) UpsertKeys() (byOrigin, byID bool) { return true, true }

// DashboardKey returns the identifier ImportDashboard upserts by, or "".
func DashboardKey(dashboard *dash0api.DashboardDefinition) string {
	return PreferredKey(DashboardKeys(dashboard))
}

// ValidateDashboard rejects a dashboard whose dash0.com/origin label and
// metadata.dash0Extensions.id differ: the API refuses the create, and an
// update would address only one of the two.
func ValidateDashboard(dashboard *dash0api.DashboardDefinition) error {
	origin, id := dashboard.GetOrigin(), dashboardExtensionsID(dashboard)
	if origin != "" && id != "" && origin != id {
		return fmt.Errorf(
			"%s (%q) and metadata.dash0Extensions.id (%q) must be equal; set only %s",
			OriginLabel, origin, id, OriginLabel,
		)
	}
	return nil
}

func dashboardExtensionsID(dashboard *dash0api.DashboardDefinition) string {
	if dashboard == nil || dashboard.Metadata.Dash0Extensions == nil || dashboard.Metadata.Dash0Extensions.Id == nil {
		return ""
	}
	return *dashboard.Metadata.Dash0Extensions.Id
}
