package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

const labelVersion = "dash0.com/version"

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

// ImportRecordingRule creates or updates a recording rule via the standard CRUD APIs.
// When the input has a user-defined ID, UPDATE is always used — PUT has
// create-or-replace semantics, so this is idempotent regardless of whether the
// rule already exists.
// When the input has no ID, CREATE is used and the server assigns an ID.
func ImportRecordingRule(ctx context.Context, apiClient dash0api.Client, rule *dash0api.RecordingRule, dataset *string) (ImportResult, error) {
	// Capture the user-defined ID before stripping; StripRecordingRuleServerFields
	// clears the dash0.com/id label, so a later read would always return "".
	id := dash0api.GetRecordingRuleID(rule)
	dash0api.StripRecordingRuleServerFields(rule)

	action := ActionCreated
	var before any
	if id != "" {
		existing, err := apiClient.GetRecordingRule(ctx, id, dataset)
		if err == nil {
			action = ActionUpdated
			before = existing
		}
	}

	if before != nil {
		CarryRecordingRuleVersion(before.(*dash0api.RecordingRule), rule)
	}

	var result *dash0api.RecordingRule
	var err error
	if id != "" {
		result, err = apiClient.UpdateRecordingRule(ctx, id, rule, dataset)
	} else {
		result, err = apiClient.CreateRecordingRule(ctx, rule, dataset)
	}
	if err != nil {
		return ImportResult{}, err
	}

	resultID := dash0api.GetRecordingRuleID(result)
	return ImportResult{Name: dash0api.GetRecordingRuleName(result), ID: resultID, Action: action, Before: before, After: result}, nil
}
