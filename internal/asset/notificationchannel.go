package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ImportNotificationChannel creates or updates a notification channel via the standard CRUD APIs.
// When the input has a user-defined ID (via dash0.com/origin label), UPDATE is always used — PUT has
// create-or-replace semantics, so this is idempotent regardless of whether the
// channel already exists.
// When the input has no origin, CREATE is used and the server assigns an ID.
func ImportNotificationChannel(ctx context.Context, apiClient dash0api.Client, channel *dash0api.NotificationChannelDefinition) (ImportResult, error) {
	dash0api.StripNotificationChannelServerFields(channel)

	action := ActionCreated
	var before any
	origin := dash0api.GetNotificationChannelOrigin(channel)
	if origin != "" {
		existing, err := apiClient.GetNotificationChannel(ctx, origin)
		if err == nil {
			action = ActionUpdated
			before = existing
		}
	}

	var result *dash0api.NotificationChannelDefinition
	var err error
	if origin != "" {
		result, err = apiClient.UpdateNotificationChannel(ctx, origin, channel)
	} else {
		result, err = apiClient.CreateNotificationChannel(ctx, channel)
	}
	if err != nil {
		return ImportResult{}, err
	}

	id := dash0api.GetNotificationChannelID(result)
	return ImportResult{Name: dash0api.GetNotificationChannelName(result), ID: id, Action: action, Before: before, After: result}, nil
}
