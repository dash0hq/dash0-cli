package dashboards

import (
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/stretchr/testify/assert"
)

func TestExtractDisplayName(t *testing.T) {
	tests := []struct {
		name      string
		dashboard *dash0api.DashboardDefinition
		want      string
	}{
		{
			name:      "nil dashboard",
			dashboard: nil,
			want:      "",
		},
		{
			name: "nil spec",
			dashboard: &dash0api.DashboardDefinition{
				Spec: nil,
			},
			want: "",
		},
		{
			name: "empty spec",
			dashboard: &dash0api.DashboardDefinition{
				Spec: map[string]interface{}{},
			},
			want: "",
		},
		{
			name: "no display field",
			dashboard: &dash0api.DashboardDefinition{
				Spec: map[string]interface{}{
					"other": "value",
				},
			},
			want: "",
		},
		{
			name: "display is not a map",
			dashboard: &dash0api.DashboardDefinition{
				Spec: map[string]interface{}{
					"display": "not a map",
				},
			},
			want: "",
		},
		{
			name: "display has no name",
			dashboard: &dash0api.DashboardDefinition{
				Spec: map[string]interface{}{
					"display": map[string]interface{}{
						"description": "some description",
					},
				},
			},
			want: "",
		},
		{
			name: "display name is not a string",
			dashboard: &dash0api.DashboardDefinition{
				Spec: map[string]interface{}{
					"display": map[string]interface{}{
						"name": 123,
					},
				},
			},
			want: "",
		},
		{
			name: "valid display name",
			dashboard: &dash0api.DashboardDefinition{
				Spec: map[string]interface{}{
					"display": map[string]interface{}{
						"name":        "My Dashboard",
						"description": "A test dashboard",
					},
				},
			},
			want: "My Dashboard",
		},
		{
			name: "empty display name",
			dashboard: &dash0api.DashboardDefinition{
				Spec: map[string]interface{}{
					"display": map[string]interface{}{
						"name": "",
					},
				},
			},
			want: "",
		},
		{
			name: "display name with special characters",
			dashboard: &dash0api.DashboardDefinition{
				Spec: map[string]interface{}{
					"display": map[string]interface{}{
						"name": "Dash0 - Services (Production) 🚀",
					},
				},
			},
			want: "Dash0 - Services (Production) 🚀",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asset.ExtractDashboardDisplayName(tt.dashboard)
			assert.Equal(t, tt.want, got)
		})
	}
}
