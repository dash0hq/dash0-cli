package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/client"
)

// ImportCheckRule creates or updates a check rule via the standard CRUD APIs.
// When the input has a user-defined ID, UPDATE is always used — PUT has
// create-or-replace semantics, so this is idempotent regardless of whether the
// rule already exists.
// When the input has no ID, CREATE is used and the server assigns an ID.
func ImportCheckRule(ctx context.Context, apiClient dash0api.Client, rule *dash0api.PrometheusAlertRule, dataset *string) (ImportResult, error) {
	dash0api.StripCheckRuleServerFields(rule)

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

	result, err := client.RetryOnConflict(ctx, func() (*dash0api.PrometheusAlertRule, error) {
		if id != "" {
			return apiClient.UpdateCheckRule(ctx, id, rule, dataset)
		}
		return apiClient.CreateCheckRule(ctx, rule, dataset)
	})
	if err != nil {
		return ImportResult{}, err
	}

	if result.Id != nil {
		id = *result.Id
	}
	return ImportResult{Name: result.Name, ID: id, Action: action, Before: before, After: result}, nil
}
