package asset

import (
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
)

func TestRoutingAssetsWarning(t *testing.T) {
	tests := []struct {
		name        string
		channel     *dash0api.NotificationChannelDefinition
		wantWarning bool
	}{
		{
			name:        "nil channel",
			channel:     nil,
			wantWarning: false,
		},
		{
			name:        "no routing",
			channel:     &dash0api.NotificationChannelDefinition{},
			wantWarning: false,
		},
		{
			name: "routing without assets",
			channel: &dash0api.NotificationChannelDefinition{
				Spec: dash0api.NotificationChannelSpec{
					Routing: &dash0api.NotificationChannelRouting{
						Filters: []dash0api.FilterCriteria{{}},
					},
				},
			},
			wantWarning: false,
		},
		{
			name: "empty assets list",
			channel: &dash0api.NotificationChannelDefinition{
				Spec: dash0api.NotificationChannelSpec{
					Routing: &dash0api.NotificationChannelRouting{
						Assets: []dash0api.NotificationChannelRoutingAsset{},
					},
				},
			},
			wantWarning: false,
		},
		{
			name: "non-empty assets list",
			channel: &dash0api.NotificationChannelDefinition{
				Spec: dash0api.NotificationChannelSpec{
					Routing: &dash0api.NotificationChannelRouting{
						Assets: []dash0api.NotificationChannelRoutingAsset{{
							Kind: dash0api.CheckRule,
							Id:   "462a0f31-28fd-4a20-b610-b75c6868b141",
							Name: "some check rule",
						}},
					},
				},
			},
			wantWarning: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			warning := RoutingAssetsWarning(tc.channel)
			if tc.wantWarning {
				assert.Contains(t, warning, "API-managed and ignored on write")
				assert.Contains(t, warning, "dash0.com/notification-channel-ids")
				assert.Contains(t, warning, "spec.notifications.channels")
			} else {
				assert.Empty(t, warning)
			}
		})
	}
}
