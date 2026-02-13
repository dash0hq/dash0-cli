package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ImportCheckRule checks existence by rule ID, strips the ID when the asset
// is not found, and calls the import API.
func ImportCheckRule(ctx context.Context, apiClient dash0api.Client, rule *dash0api.PrometheusAlertRule, dataset *string) (ImportResult, error) {
	// Strip server-managed fields — the import API manages these and rejects
	// requests that include them (e.g. dataset).
	rule.Dataset = nil
	if rule.Labels != nil {
		delete(*rule.Labels, "dash0.com/origin")
	}

	// Check if check rule exists
	action := ActionCreated
	if rule.Id != nil && *rule.Id != "" {
		_, err := apiClient.GetCheckRule(ctx, *rule.Id, dataset)
		if err == nil {
			action = ActionUpdated
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
	return ImportResult{Name: result.Name, ID: id, Action: action}, nil
}
