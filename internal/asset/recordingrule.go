package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

const labelVersion = "dash0.com/version"

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
	dash0api.StripRecordingRuleServerFields(rule)

	action := ActionCreated
	var before any
	id := dash0api.GetRecordingRuleID(rule)
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
