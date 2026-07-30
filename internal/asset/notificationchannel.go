package asset

import (
	"context"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// RoutingAssetsWarning returns a user-facing warning when the notification channel definition
// carries a non-empty spec.routing.assets, and an empty string otherwise. The Dash0 API treats
// spec.routing.assets as an API-managed back-reference (populated when a check rule or synthetic
// check binds to the channel) and silently ignores any value supplied on write — without a
// warning, users believe they attached the channel when nothing happened. The wording matches the
// Terraform provider's warnIfRoutingAssetsSet so both IaC clients speak the same language.
func RoutingAssetsWarning(channel *dash0api.NotificationChannelDefinition) string {
	if channel == nil || channel.Spec.Routing == nil || len(channel.Spec.Routing.Assets) == 0 {
		return ""
	}
	return "spec.routing.assets is API-managed and ignored on write: the Dash0 API populates it as a " +
		"back-reference when a check rule or synthetic check binds to this channel. Existing bindings " +
		"are unaffected. To bind a check rule, set the dash0.com/notification-channel-ids " +
		"annotation on the check rule; to bind a synthetic check, set spec.notifications.channels on " +
		"the synthetic check."
}

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
