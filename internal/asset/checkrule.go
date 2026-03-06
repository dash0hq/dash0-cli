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

// ImportCheckRule checks existence by rule ID, strips the ID when the asset
// is not found, and calls the import API.
func ImportCheckRule(ctx context.Context, apiClient dash0api.Client, rule *dash0api.PrometheusAlertRule, dataset *string) (ImportResult, error) {
	StripCheckRuleServerFields(rule)

	// Check if check rule exists
	action := ActionCreated
	var before any
	if rule.Id != nil && *rule.Id != "" {
		existing, err := apiClient.GetCheckRule(ctx, *rule.Id, dataset)
		if err == nil {
			action = ActionUpdated
			before = existing
		} else {
			// Asset not found — strip ID so the API creates a fresh asset.
			rule.Id = nil
		}
	}

	result, err := apiClient.ImportCheckRule(ctx, rule, dataset)
	if err != nil {
		return ImportResult{}, err
	}

	id := ""
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
