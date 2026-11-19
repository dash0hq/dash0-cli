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
	// A failed preflight does not block the write: the PUT reports the same error.
	plan, _ := PlanNotificationChannel(ctx, apiClient, channel)

	var result *dash0api.NotificationChannelDefinition
	var err error
	if plan.Key != "" {
		result, err = apiClient.UpdateNotificationChannel(ctx, plan.Key, channel)
	} else {
		result, err = apiClient.CreateNotificationChannel(ctx, channel)
	}
	if err != nil {
		return ImportResult{}, err
	}

	id := dash0api.GetNotificationChannelID(result)
	return ImportResult{Name: dash0api.GetNotificationChannelName(result), ID: id, Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanNotificationChannel reads the stored channel that
// ImportNotificationChannel would write, keyed by origin. It returns any
// preflight error other than "not found".
func PlanNotificationChannel(ctx context.Context, apiClient dash0api.Client, channel *dash0api.NotificationChannelDefinition) (ImportPlan, error) {
	dash0api.StripNotificationChannelServerFields(channel)
	origin := dash0api.GetNotificationChannelOrigin(channel)
	plan := ImportPlan{Key: origin, Action: ActionCreated, Proposed: channel}
	if origin == "" {
		return plan, nil
	}
	existing, err := apiClient.GetNotificationChannel(ctx, origin)
	if err != nil {
		return plan, ignoreNotFound(err)
	}
	plan.Action, plan.Before = ActionUpdated, existing
	return plan, nil
}
