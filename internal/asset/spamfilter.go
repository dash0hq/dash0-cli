package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ImportSpamFilter creates or updates a spam filter via the standard CRUD APIs.
// When the input has a user-defined ID, UPDATE is always used — PUT has
// create-or-replace semantics, so this is idempotent regardless of whether the
// filter already exists.
// When the input has no ID, CREATE is used and the server assigns an ID.
func ImportSpamFilter(ctx context.Context, apiClient dash0api.Client, filter *dash0api.SpamFilter, dataset *string) (ImportResult, error) {
	dash0api.StripSpamFilterServerFields(filter)

	action := ActionCreated
	var before any
	id := dash0api.GetSpamFilterID(filter)
	if id != "" {
		existing, err := apiClient.GetSpamFilter(ctx, id, dataset)
		if err == nil {
			action = ActionUpdated
			before = existing
		}
	}

	var result *dash0api.SpamFilter
	var err error
	if id != "" {
		result, err = apiClient.UpdateSpamFilter(ctx, id, filter, dataset)
	} else {
		result, err = apiClient.CreateSpamFilter(ctx, filter, dataset)
	}
	if err != nil {
		return ImportResult{}, err
	}

	resultID := dash0api.GetSpamFilterID(result)
	return ImportResult{Name: dash0api.GetSpamFilterName(result), ID: resultID, Action: action, Before: before, After: result}, nil
}
