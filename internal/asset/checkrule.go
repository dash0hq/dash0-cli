package asset

import (
	"context"
	"fmt"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ImportCheckRule creates or updates a check rule via the standard CRUD APIs.
// When the input has a user-defined ID, UPDATE is always used — PUT has
// create-or-replace semantics, so this is idempotent regardless of whether the
// rule already exists.
// When the input has no ID, CREATE is used and the server assigns an ID.
// A PUT to the ID of a deleted check rule brings it back, which is reported as
// ActionRestored. Before then holds the deleted rule without its deletion
// timestamp, so a diff against After shows only the other changes.
func ImportCheckRule(ctx context.Context, apiClient dash0api.Client, rule *dash0api.PrometheusAlertRule, dataset *string) (ImportResult, error) {
	dash0api.StripCheckRuleServerFields(rule)
	ClearCheckRuleDeletedAt(rule)

	action := ActionCreated
	var before any
	id := ""
	if rule.Id != nil && *rule.Id != "" {
		id = *rule.Id
		existing, err := apiClient.GetCheckRule(ctx, id, dataset)
		if err == nil {
			action = ActionUpdated
			if CheckRuleDeletedAt(existing) != nil {
				action = ActionRestored
				ClearCheckRuleDeletedAt(existing)
			}
			before = existing
		}
	}

	var (
		result *dash0api.PrometheusAlertRule
		err    error
	)
	if id != "" {
		result, err = apiClient.UpdateCheckRule(ctx, id, rule, dataset)
	} else {
		result, err = apiClient.CreateCheckRule(ctx, rule, dataset)
	}
	if err != nil {
		return ImportResult{}, err
	}
	if err := RequireCheckRuleNotDeleted(result); err != nil {
		return ImportResult{}, err
	}

	if result.Id != nil {
		id = *result.Id
	}
	return ImportResult{Name: result.Name, ID: id, Action: action, Before: before, After: result}, nil
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

// RequireCheckRuleNotDeleted returns an error when the API response for a
// create or update still marks the check rule as deleted.
func RequireCheckRuleNotDeleted(rule *dash0api.PrometheusAlertRule) error {
	if CheckRuleDeletedAt(rule) == nil {
		return nil
	}
	return fmt.Errorf("check rule %q is still deleted after the update", rule.Name)
}
