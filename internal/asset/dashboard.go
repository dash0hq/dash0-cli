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
	plan, _ := planDashboard(ctx, apiClient, dashboard, dataset)
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
	return planDashboard(ctx, apiClient, dashboard, dataset)
}

func planDashboard(ctx context.Context, apiClient dash0api.Client, dashboard *dash0api.DashboardDefinition, dataset *string) (ImportPlan, error) {
	// Read the keys first: the strip clears the dash0.com/id label.
	key := DashboardKey(dashboard)
	alias := DashboardAlias(dashboard)
	dash0api.StripDashboardServerFields(dashboard)
	plan := ImportPlan{Key: key, Action: ActionCreated, Proposed: dashboard}
	if key == "" {
		return plan, nil
	}
	lookupKey := key
	existing, err := apiClient.GetDashboard(ctx, lookupKey, dataset)
	// An exported dashboard that gains an origin still names its server id in
	// dash0.com/id, so updating at that id keeps it from becoming a second
	// dashboard. The API ignores the origin on an update, so the dashboard
	// keeps no origin, and apply --since deletes it at the id.
	if dash0api.IsNotFound(err) && alias != "" {
		lookupKey = alias
		existing, err = apiClient.GetDashboard(ctx, lookupKey, dataset)
	}
	if err != nil {
		return plan, ignoreNotFound(err)
	}
	plan.Key, plan.Action, plan.Before = lookupKey, actionForExisting(DashboardDeletedAt(existing)), existing
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

// DashboardAlias returns the dash0.com/id label when the dashboard upserts by
// another key, or "". It names the same dashboard: ImportDashboard falls back
// to it, and apply --since deletes at it when the key is not found.
func DashboardAlias(dashboard *dash0api.DashboardDefinition) string {
	if dashboard == nil || dashboard.Metadata.Labels == nil || dashboard.Metadata.Labels.Dash0Comid == nil {
		return ""
	}
	if label := *dashboard.Metadata.Labels.Dash0Comid; label != DashboardKey(dashboard) {
		return label
	}
	return ""
}

// ValidateDashboard rejects a dashboard whose dash0.com/origin label and
// metadata.dash0Extensions.id differ: the API refuses the create, and an
// update would address only one of the two.
func ValidateDashboard(dashboard *dash0api.DashboardDefinition) error {
	origin, id := dashboard.GetOrigin(), dashboardExtensionsID(dashboard)
	if origin != "" && id != "" && origin != id {
		return fmt.Errorf(
			"%s (%q) and metadata.dash0Extensions.id (%q) must be equal; set only %s, and move %q to metadata.labels[\"dash0.com/id\"] if it is the id of an existing dashboard",
			OriginLabel, origin, id, OriginLabel, id,
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
