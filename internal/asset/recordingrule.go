package asset

import (
	"context"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

const (
	labelVersion        = "dash0.com/version"
	annotationDeletedAt = "dash0.com/deleted-at"
)

// RecordingOnlyPrometheusRule returns a copy of the input PrometheusRule CRD
// stripped of alerting rules so it can be sent to the recording-rules API.
// Returns nil if the input contains no recording rules. Empty groups (after
// filtering) are dropped.
func RecordingOnlyPrometheusRule(crd *dash0api.RecordingRule) *dash0api.RecordingRule {
	if crd == nil {
		return nil
	}
	out := *crd
	out.Spec = dash0api.PrometheusRuleSpec{}
	for _, group := range crd.Spec.Groups {
		var kept []dash0api.PrometheusRuleDefinition
		for _, rule := range group.Rules {
			if rule.Record != nil && *rule.Record != "" {
				kept = append(kept, rule)
			}
		}
		if len(kept) == 0 {
			continue
		}
		groupCopy := group
		groupCopy.Rules = kept
		out.Spec.Groups = append(out.Spec.Groups, groupCopy)
	}
	if len(out.Spec.Groups) == 0 {
		return nil
	}
	return &out
}

// PrometheusRuleHasAlerts reports whether any rule in the CRD is an alerting
// rule (has `alert:` set). Used by apply to decide whether to dispatch to the
// check-rule import path.
func PrometheusRuleHasAlerts(crd *dash0api.RecordingRule) bool {
	if crd == nil {
		return false
	}
	for _, group := range crd.Spec.Groups {
		for _, rule := range group.Rules {
			if rule.Alert != nil && *rule.Alert != "" {
				return true
			}
		}
	}
	return false
}

// CarryRecordingRuleVersion copies the version label from src to dst so that
// update requests include the optimistic-locking version the API currently requires.
func CarryRecordingRuleVersion(src, dst *dash0api.RecordingRule) {
	if src == nil || src.Metadata.Labels == nil {
		return
	}
	version := (*src.Metadata.Labels)[labelVersion]
	if version == "" {
		return
	}
	if dst.Metadata.Labels == nil {
		labels := map[string]string{labelVersion: version}
		dst.Metadata.Labels = &labels
	} else {
		(*dst.Metadata.Labels)[labelVersion] = version
	}
}

// ImportRecordingRule creates or updates a recording rule through
// RouteDocument. A PUT to a deleted recording rule brings it back, which is
// reported as ActionRestored.
func ImportRecordingRule(ctx context.Context, apiClient dash0api.Client, rule *dash0api.RecordingRule, dataset *string) (ImportResult, error) {
	plan, err := PlanRecordingRule(ctx, apiClient, rule, dataset)
	// A failed origin read does not block the write: the PUT reports the same error.
	if plan.Key == "" {
		return ImportResult{}, err
	}
	if plan.Action != ActionCreated {
		SetRecordingRuleOrigin(rule, UpdateOrigin(rule.GetOrigin(), plan.Key))
	}
	result, err := apiClient.UpdateRecordingRule(ctx, plan.Key, rule, dataset)
	if err != nil {
		return ImportResult{}, err
	}

	resultID := dash0api.GetRecordingRuleID(result)
	if err := RequireNotDeleted("Recording rule", dash0api.GetRecordingRuleName(result), resultID, RecordingRuleDeletedAt(result)); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Name: dash0api.GetRecordingRuleName(result), ID: resultID, Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanRecordingRule routes the recording rule with RouteDocument, reads the
// stored rule that ImportRecordingRule would write, and carries its version
// onto rule. It returns any preflight error other than "not found"; Key is
// empty when the id read failed, because the write has no target.
func PlanRecordingRule(ctx context.Context, apiClient dash0api.Client, rule *dash0api.RecordingRule, dataset *string) (ImportPlan, error) {
	// StripRecordingRuleServerFields clears the id and origin, so read the identity first.
	identity := RecordingRuleIdentity(rule, dataset)
	dash0api.StripRecordingRuleServerFields(rule)
	get := func(ctx context.Context, key string) (*dash0api.RecordingRule, error) {
		return apiClient.GetRecordingRule(ctx, key, dataset)
	}
	route, err := RouteDocument(ctx, identity, get, (*dash0api.RecordingRule).GetOrigin)
	plan := ImportPlan{Key: route.Key, Action: ActionCreated, Proposed: rule}
	if route.Key == "" || route.Create {
		SetRecordingRuleOrigin(rule, route.Key)
		return plan, err
	}
	// The stored origin keeps the diff to what changes; the import clears it
	// before a write at the id.
	SetRecordingRuleOrigin(rule, route.Stored.GetOrigin())
	plan.Action, plan.Before = actionForExisting(RecordingRuleDeletedAt(route.Stored)), route.Stored
	CarryRecordingRuleVersion(route.Stored, rule)
	return plan, nil
}

// RecordingRuleIdentity returns what RouteDocument routes the recording rules
// of a PrometheusRule CRD by. They are one asset, so the origin is inferred
// from the CRD's metadata.name alone.
func RecordingRuleIdentity(rule *dash0api.RecordingRule, dataset *string) Identity {
	return identityOf("recordingrule", rule.Metadata.Name, dataset, rule)
}

// SetRecordingRuleOrigin sets the dash0.com/origin label, and removes it when origin is empty.
func SetRecordingRuleOrigin(rule *dash0api.RecordingRule, origin string) {
	setRecordingRuleLabel(rule, dash0api.LabelOrigin, origin)
}

func setRecordingRuleLabel(rule *dash0api.RecordingRule, key, value string) {
	if rule.Metadata.Labels == nil {
		if value == "" {
			return
		}
		rule.Metadata.Labels = &map[string]string{}
	}
	if value == "" {
		delete(*rule.Metadata.Labels, key)
		return
	}
	(*rule.Metadata.Labels)[key] = value
}

// RecordingRuleDeletedAt returns when the recording rule was deleted, or nil
// when it is not deleted. The PrometheusRule type keeps annotations as plain
// strings, so the timestamp is parsed here.
func RecordingRuleDeletedAt(rule *dash0api.RecordingRule) *time.Time {
	if rule == nil || rule.Metadata.Annotations == nil {
		return nil
	}
	value := (*rule.Metadata.Annotations)[annotationDeletedAt]
	if value == "" {
		return nil
	}
	deletedAt, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &deletedAt
}

// ClearRecordingRuleDeletedAt removes the deletion timestamp from a recording
// rule, for example from a file exported with `recording-rules get -o yaml`
// while the rule was deleted.
func ClearRecordingRuleDeletedAt(rule *dash0api.RecordingRule) {
	if rule == nil || rule.Metadata.Annotations == nil {
		return
	}
	delete(*rule.Metadata.Annotations, annotationDeletedAt)
}
