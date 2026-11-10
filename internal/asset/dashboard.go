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

	resultID := dash0api.GetDashboardID(result)
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
	dash0api.StripDashboardServerFields(dashboard)
	plan := ImportPlan{Action: ActionCreated, Proposed: dashboard}
	if dashboard.Metadata.Dash0Extensions == nil || dashboard.Metadata.Dash0Extensions.Id == nil || *dashboard.Metadata.Dash0Extensions.Id == "" {
		return plan, nil
	}
	plan.Key = *dashboard.Metadata.Dash0Extensions.Id
	existing, err := apiClient.GetDashboard(ctx, plan.Key, dataset)
	if err != nil {
		return plan, ignoreNotFound(err)
	}
	plan.Action, plan.Before = actionForExisting(DashboardDeletedAt(existing)), existing
	return plan, nil
}

// IsEphemeralDashboard reports whether dashboard is an EphemeralDashboard.
func IsEphemeralDashboard(dashboard *dash0api.DashboardDefinition) bool {
	return NormalizeKind(string(dashboard.Kind)) == "ephemeraldashboard"
}

// DashboardDisplayKind returns "Ephemeral dashboard" for an
// EphemeralDashboard and "Dashboard" for any other dashboard.
func DashboardDisplayKind(dashboard *dash0api.DashboardDefinition) string {
	if IsEphemeralDashboard(dashboard) {
		return KindDisplayName("ephemeraldashboard")
	}
	return KindDisplayName("dashboard")
}

// ValidateEphemeralDashboard rejects an EphemeralDashboard without spec.ttl
// before any write. The API owns the duration format and its limits.
func ValidateEphemeralDashboard(dashboard *dash0api.DashboardDefinition) error {
	if !IsEphemeralDashboard(dashboard) {
		return nil
	}
	if ttl, _ := dashboard.Spec["ttl"].(string); ttl == "" {
		return fmt.Errorf("an EphemeralDashboard requires spec.ttl, a duration such as 1d or 12h")
	}
	return nil
}

// ToEphemeralDashboard turns dashboard into an EphemeralDashboard that the API
// deletes once ttl has passed since its latest write.
func ToEphemeralDashboard(dashboard *dash0api.DashboardDefinition, ttl string) {
	dashboard.Kind = dash0api.DashboardKindEphemeralDashboard
	if dashboard.Spec == nil {
		dashboard.Spec = map[string]interface{}{}
	}
	dashboard.Spec["ttl"] = ttl
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
