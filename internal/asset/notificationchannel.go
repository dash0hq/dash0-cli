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

// ImportNotificationChannel creates or updates a notification channel through
// RouteDocument.
func ImportNotificationChannel(ctx context.Context, apiClient dash0api.Client, channel *dash0api.NotificationChannelDefinition) (ImportResult, error) {
	plan, err := PlanNotificationChannel(ctx, apiClient, channel)
	// A failed origin read does not block the write: the PUT reports the same error.
	if plan.Key == "" {
		return ImportResult{}, err
	}
	if plan.Action != ActionCreated {
		setNotificationChannelOrigin(channel, "")
	}
	result, err := apiClient.UpdateNotificationChannel(ctx, plan.Key, channel)
	if err != nil {
		return ImportResult{}, err
	}

	id := dash0api.GetNotificationChannelID(result)
	return ImportResult{Name: dash0api.GetNotificationChannelName(result), ID: id, Action: plan.Action, Before: plan.Before, After: result}, nil
}

// PlanNotificationChannel routes the channel with RouteDocument and reads the
// stored channel that ImportNotificationChannel would write. It returns any
// preflight error other than "not found"; an empty Key means the write has no
// target.
func PlanNotificationChannel(ctx context.Context, apiClient dash0api.Client, channel *dash0api.NotificationChannelDefinition) (ImportPlan, error) {
	// The strip clears the dash0.com/id label, so read it first.
	identity := NotificationChannelIdentity(channel)
	dash0api.StripNotificationChannelServerFields(channel)
	route, err := RouteDocument(ctx, identity, apiClient.GetNotificationChannel, (*dash0api.NotificationChannelDefinition).GetOrigin)
	plan := ImportPlan{Key: route.Key, Action: ActionCreated, Proposed: channel}
	if route.Key == "" || route.Create {
		setNotificationChannelOrigin(channel, route.Key)
		return plan, err
	}
	setNotificationChannelOrigin(channel, route.Stored.GetOrigin())
	plan.Action, plan.Before = ActionUpdated, route.Stored
	return plan, err
}

// NotificationChannelIdentity returns what RouteDocument routes a notification
// channel by. Channels are organization-level, so the inferred origin has no
// dataset.
func NotificationChannelIdentity(channel *dash0api.NotificationChannelDefinition) Identity {
	return identityOf("notificationchannel", channel.Metadata.Name, "", channel)
}

func setNotificationChannelOrigin(channel *dash0api.NotificationChannelDefinition, origin string) {
	if channel.Metadata.Labels == nil {
		if origin == "" {
			return
		}
		channel.Metadata.Labels = &dash0api.NotificationChannelLabels{}
	}
	channel.Metadata.Labels.Dash0Comorigin = optionalString(origin)
}
