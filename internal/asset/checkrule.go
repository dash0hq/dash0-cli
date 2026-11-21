package asset

import (
	"context"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ImportCheckRule creates or updates a check rule through RouteDocument. A
// PUT to a deleted check rule brings it back, which is reported as
// ActionRestored.
func ImportCheckRule(ctx context.Context, apiClient dash0api.Client, rule CheckRule, dataset *string) (ImportResult, error) {
	plan, err := PlanCheckRule(ctx, apiClient, rule, dataset)
	// A failed origin read does not block the write: the PUT reports the same error.
	if plan.Key == "" {
		return ImportResult{}, err
	}
	if plan.Action != ActionCreated {
		setCheckRuleOrigin(rule.PrometheusAlertRule, "")
	}
	result, err := apiClient.UpdateCheckRule(ctx, plan.Key, rule.PrometheusAlertRule, dataset)
	if err != nil {
		return ImportResult{}, err
	}
	id := plan.Key
	if result.Id != nil {
		id = *result.Id
	}
	if err := RequireNotDeleted("Check rule", result.Name, id, CheckRuleDeletedAt(result)); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Name: result.Name, ID: id, Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanCheckRule routes the check rule with RouteDocument and reads the stored
// rule that ImportCheckRule would write. It returns any preflight error other
// than "not found"; Key is empty when the id read failed, because the write
// has no target.
func PlanCheckRule(ctx context.Context, apiClient dash0api.Client, rule CheckRule, dataset *string) (ImportPlan, error) {
	// StripCheckRuleServerFields clears the origin, so read the identity first.
	identity := rule.Identity(dataset)
	dash0api.StripCheckRuleServerFields(rule.PrometheusAlertRule)
	get := func(ctx context.Context, key string) (*dash0api.PrometheusAlertRule, error) {
		return apiClient.GetCheckRule(ctx, key, dataset)
	}
	route, err := RouteDocument(ctx, identity, get, (*dash0api.PrometheusAlertRule).GetOrigin)
	plan := ImportPlan{Key: route.Key, Action: ActionCreated, Proposed: rule.PrometheusAlertRule}
	// An id in the body that is not the path's key can only disagree with it.
	if route.Key != identity.ID {
		dash0api.ClearCheckRuleID(rule.PrometheusAlertRule)
	}
	if route.Key == "" || route.Create {
		setCheckRuleOrigin(rule.PrometheusAlertRule, route.Key)
		return plan, err
	}
	// The stored origin keeps the diff to what changes; ImportCheckRule clears
	// it before the write.
	setCheckRuleOrigin(rule.PrometheusAlertRule, route.Stored.GetOrigin())
	plan.Action, plan.Before = actionForExisting(CheckRuleDeletedAt(route.Stored)), route.Stored
	return plan, nil
}

// setCheckRuleOrigin sets the origin, which a check rule keeps among its
// Prometheus labels.
func setCheckRuleOrigin(rule *dash0api.PrometheusAlertRule, origin string) {
	if rule.Labels == nil {
		if origin == "" {
			return
		}
		rule.Labels = &map[string]string{}
	}
	if origin == "" {
		delete(*rule.Labels, dash0api.LabelOrigin)
		return
	}
	(*rule.Labels)[dash0api.LabelOrigin] = origin
}

// CheckRuleDeletedAt returns when the check rule was deleted, or nil when it is
// not deleted.
func CheckRuleDeletedAt(rule *dash0api.PrometheusAlertRule) *time.Time {
	if rule == nil || rule.Metadata == nil || rule.Metadata.Annotations == nil {
		return nil
	}
	return rule.Metadata.Annotations.Dash0ComdeletedAt
}

// ClearCheckRuleDeletedAt removes the deletion timestamp from a check rule, for
// example from a file exported with `check-rules get -o yaml` while the rule
// was deleted.
func ClearCheckRuleDeletedAt(rule *dash0api.PrometheusAlertRule) {
	if rule == nil || rule.Metadata == nil {
		return
	}
	if annotations := rule.Metadata.Annotations; annotations != nil {
		annotations.Dash0ComdeletedAt = nil
		if *annotations == (dash0api.PrometheusAlertRuleMetadataAnnotations{}) {
			rule.Metadata.Annotations = nil
		}
	}
	if rule.Metadata.Annotations == nil && rule.Metadata.Labels == nil {
		rule.Metadata = nil
	}
}
