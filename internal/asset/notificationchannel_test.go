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

func TestRoutingAssetsChangedWarning(t *testing.T) {
	asset := dash0api.NotificationChannelRoutingAsset{
		Kind: dash0api.CheckRule,
		Id:   "462a0f31-28fd-4a20-b610-b75c6868b141",
		Name: "some check rule",
	}
	otherAsset := dash0api.NotificationChannelRoutingAsset{
		Kind: dash0api.CheckRule,
		Id:   "9c1f13ff-11f2-4d17-b45e-a4c8825f7e56",
		Name: "another check rule",
	}
	withAssets := func(assets ...dash0api.NotificationChannelRoutingAsset) *dash0api.NotificationChannelDefinition {
		return &dash0api.NotificationChannelDefinition{
			Spec: dash0api.NotificationChannelSpec{
				Routing: &dash0api.NotificationChannelRouting{Assets: assets},
			},
		}
	}

	tests := []struct {
		name        string
		channel     *dash0api.NotificationChannelDefinition
		before      *dash0api.NotificationChannelDefinition
		wantWarning bool
	}{
		{
			name:        "no assets in file",
			channel:     &dash0api.NotificationChannelDefinition{},
			before:      withAssets(asset),
			wantWarning: false,
		},
		{
			name:        "file assets match server: get-edit-update roundtrip stays silent",
			channel:     withAssets(asset),
			before:      withAssets(asset),
			wantWarning: false,
		},
		{
			name:        "file assets differ from server",
			channel:     withAssets(asset, otherAsset),
			before:      withAssets(asset),
			wantWarning: true,
		},
		{
			name:        "server has no routing",
			channel:     withAssets(asset),
			before:      &dash0api.NotificationChannelDefinition{},
			wantWarning: true,
		},
		{
			name:        "nil before",
			channel:     withAssets(asset),
			before:      nil,
			wantWarning: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			warning := RoutingAssetsChangedWarning(tc.channel, tc.before)
			if tc.wantWarning {
				assert.Contains(t, warning, "API-managed and ignored on write")
			} else {
				assert.Empty(t, warning)
			}
		})
	}
}
