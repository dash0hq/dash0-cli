package asset

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/google/uuid"
)

// ImportDashboard creates or updates a dashboard through RouteDocument. A PUT
// to a deleted dashboard brings it back, which is reported as ActionRestored.
func ImportDashboard(ctx context.Context, apiClient dash0api.Client, dashboard *dash0api.DashboardDefinition, dataset *string) (ImportResult, error) {
	if err := ValidateDashboard(dashboard); err != nil {
		return ImportResult{}, err
	}
	plan, err := planDashboard(ctx, apiClient, dashboard, dataset)
	// A failed origin read does not block the write: the PUT reports the same
	// error. A kind mismatch does, because the API rejects that write.
	if _, ok := errors.AsType[*DashboardKindMismatchError](err); ok || plan.Key == "" {
		return ImportResult{}, err
	}
	id := plan.Key

	dash0api.ClearDashboardID(dashboard)
	if plan.Action != ActionCreated {
		setDashboardOrigin(dashboard, "")
	}
	result, err := apiClient.UpdateDashboard(ctx, id, dashboard, dataset)
	if err != nil {
		return ImportResult{}, err
	}

	if resultID := DashboardKey(result); resultID != "" {
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
	// Read the identity first: the strip clears the dash0.com/id label.
	identity := DashboardIdentity(dashboard, dataset)
	dash0api.StripDashboardServerFields(dashboard)
	get := func(ctx context.Context, key string) (*dash0api.DashboardDefinition, error) {
		return apiClient.GetDashboard(ctx, key, dataset)
	}
	route, err := RouteDocument(ctx, identity, get, (*dash0api.DashboardDefinition).GetOrigin)
	plan := ImportPlan{Key: route.Key, Action: ActionCreated, Proposed: dashboard}
	if route.Key == "" {
		return plan, err
	}
	if route.Create {
		// The API stores a UUID key as the dashboard's id, not its origin.
		if _, notUUID := uuid.Parse(route.Key); notUUID != nil {
			setDashboardOrigin(dashboard, route.Key)
		}
		return plan, err
	}
	if err := CheckDashboardKind(route.Key, route.Stored, dashboard); err != nil {
		return plan, err
	}
	setDashboardOrigin(dashboard, route.Stored.GetOrigin())
	plan.Action, plan.Before = actionForExisting(DashboardDeletedAt(route.Stored)), route.Stored
	return plan, nil
}

// DashboardIdentity returns what RouteDocument routes a dashboard by. The id is
// the dash0.com/id label; metadata.dash0Extensions.id names the origin, which
// ValidateDashboard keeps equal to the dash0.com/origin label.
func DashboardIdentity(dashboard *dash0api.DashboardDefinition, dataset *string) Identity {
	identity := Identity{
		Origin:   dashboard.GetOrigin(),
		Inferred: InferOrigin(string(dashboard.Kind), dashboard.Metadata.Name, DatasetName(dataset)),
	}
	if identity.Origin == "" {
		identity.Origin = dashboardExtensionsID(dashboard)
	}
	if labels := dashboard.Metadata.Labels; labels != nil && labels.Dash0Comid != nil {
		identity.ID = *labels.Dash0Comid
	}
	return identity
}

func setDashboardOrigin(dashboard *dash0api.DashboardDefinition, origin string) {
	if dashboard.Metadata.Labels == nil {
		if origin == "" {
			return
		}
		dashboard.Metadata.Labels = &dash0api.DashboardLabels{}
	}
	dashboard.Metadata.Labels.Dash0Comorigin = optionalString(origin)
}

// CheckDashboardKind returns a DashboardKindMismatchError when proposed would
// change the stored dashboard at id between ephemeral and permanent. A deleted
// dashboard can come back as either kind.
func CheckDashboardKind(id string, stored, proposed *dash0api.DashboardDefinition) error {
	if DashboardDeletedAt(stored) != nil || IsEphemeralDashboard(stored) == IsEphemeralDashboard(proposed) {
		return nil
	}
	return &DashboardKindMismatchError{ID: id, Stored: DashboardDisplayKind(stored), Proposed: DashboardDisplayKind(proposed)}
}

// DashboardKindMismatchError reports a write that would change a dashboard
// between ephemeral and permanent, which the API rejects.
type DashboardKindMismatchError struct {
	ID       string
	Stored   string
	Proposed string
}

func (e *DashboardKindMismatchError) Error() string {
	return fmt.Sprintf("dashboard %q is stored as %s, and the API does not change the kind of an existing dashboard to %s; use another identifier, or delete the stored dashboard first", e.ID, strings.ToLower(e.Stored), strings.ToLower(e.Proposed))
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
		dashboard.Spec = map[string]any{}
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
