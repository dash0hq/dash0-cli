package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// StripCheckRuleServerFields removes server-generated fields from a check rule
// definition. Used by both Import (to avoid sending rejected fields to the API)
// and diff rendering (to suppress noise).
func StripCheckRuleServerFields(r *dash0api.PrometheusAlertRule) {
	r.Dataset = nil
	if r.Labels != nil {
		delete(*r.Labels, "dash0.com/origin")
	}
}

// ImportCheckRule creates or updates a check rule via the standard CRUD APIs.
// When the input has a user-defined ID, UPDATE is always used — PUT has
// create-or-replace semantics, so this is idempotent regardless of whether the
// rule already exists.
// When the input has no ID, CREATE is used and the server assigns an ID.
func ImportCheckRule(ctx context.Context, apiClient dash0api.Client, rule *dash0api.PrometheusAlertRule, dataset *string) (ImportResult, error) {
	StripCheckRuleServerFields(rule)

	action := ActionCreated
	var before any
	id := ""
	if rule.Id != nil && *rule.Id != "" {
		id = *rule.Id
		existing, err := apiClient.GetCheckRule(ctx, id, dataset)
		if err == nil {
			action = ActionUpdated
			before = existing
		}
	}

	var result *dash0api.PrometheusAlertRule
	var err error
	if id != "" {
		result, err = apiClient.UpdateCheckRule(ctx, id, rule, dataset)
	} else {
		result, err = apiClient.CreateCheckRule(ctx, rule, dataset)
	}
	if err != nil {
		return ImportResult{}, err
	}

	if result.Id != nil {
		id = *result.Id
	}
	return ImportResult{Name: result.Name, ID: id, Action: action, Before: before, After: result}, nil
}

// ExtractCheckRuleID extracts the ID from a check rule definition.
func ExtractCheckRuleID(rule *dash0api.PrometheusAlertRule) string {
	if rule.Id != nil {
		return *rule.Id
	}
	return ""
}

// ExtractCheckRuleName extracts the name from a check rule definition.
func ExtractCheckRuleName(rule *dash0api.PrometheusAlertRule) string {
	return rule.Name
}
