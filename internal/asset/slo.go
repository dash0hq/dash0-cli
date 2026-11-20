package asset

import (
	"context"
	"fmt"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
)

// SLOUsesOrigin reports whether an SLO document carries a non-empty
// dash0.com/origin label.
//
// --since uses this to warn when an SLO is about to be deleted by
// dash0.com/id alone: an id-only SLO that an earlier CLI version created by
// POST sits at a server-assigned id the document never learned, and the
// delete misses it.
func SLOUsesOrigin(data []byte) (bool, error) {
	var slo dash0api.SloDefinition
	if err := dash0yaml.Unmarshal(data, &slo); err != nil {
		return false, fmt.Errorf("failed to decode SLO: %w", err)
	}
	return dash0api.GetSLOOrigin(&slo) != "", nil
}

// ImportSLO creates or updates an SLO through RouteDocument. The path carries
// the origin, so the body never does. A PUT to a deleted SLO brings it back,
// which is reported as ActionRestored.
func ImportSLO(ctx context.Context, apiClient dash0api.Client, slo *dash0api.SloDefinition, dataset *string) (ImportResult, error) {
	plan, err := PlanSLO(ctx, apiClient, slo, dataset)
	if err != nil {
		return ImportResult{}, err
	}
	result, err := apiClient.UpdateSLO(ctx, plan.Key, slo, dataset)
	if err != nil {
		return ImportResult{}, err
	}
	id := dash0api.GetSLOID(result)
	if err := RequireNotDeleted("SLO", dash0api.GetSLOName(result), id, SLODeletedAt(result)); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Name: dash0api.GetSLOName(result), ID: id, Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanSLO routes the SLO with RouteDocument and reads the stored SLO that
// ImportSLO would write. Any preflight error other than "not found" is
// returned and stops the write.
func PlanSLO(ctx context.Context, apiClient dash0api.Client, slo *dash0api.SloDefinition, dataset *string) (ImportPlan, error) {
	// StripSLOServerFields clears both labels, so read them first.
	identity := SLOIdentity(slo, dataset)
	dash0api.StripSLOServerFields(slo)
	get := func(ctx context.Context, key string) (*dash0api.SloDefinition, error) {
		return apiClient.GetSLO(ctx, key, dataset)
	}
	route, err := RouteDocument(ctx, identity, get, (*dash0api.SloDefinition).GetOrigin)
	plan := ImportPlan{Key: route.Key, Action: ActionCreated, Proposed: slo}
	if err != nil {
		return plan, err
	}
	if !route.Create {
		plan.Action, plan.Before = actionForExisting(SLODeletedAt(route.Stored)), route.Stored
	}
	return plan, nil
}

// SLOIdentity returns what RouteDocument routes an SLO by.
func SLOIdentity(slo *dash0api.SloDefinition, dataset *string) Identity {
	return identityOf("slo", slo.Metadata.Name, dataset, slo)
}

// SLODeletedAt returns when the SLO was deleted, or nil when it is not deleted.
func SLODeletedAt(slo *dash0api.SloDefinition) *time.Time {
	if slo == nil || slo.Metadata.Annotations == nil {
		return nil
	}
	return slo.Metadata.Annotations.Dash0ComdeletedAt
}
