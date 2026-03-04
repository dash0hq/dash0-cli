package asset

import (
	"encoding/json"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func TestExtractDashboardID(t *testing.T) {
	tests := []struct {
		name      string
		dashboard *dash0api.DashboardDefinition
		want      string
	}{
		{
			name:      "nil dash0Extensions",
			dashboard: &dash0api.DashboardDefinition{},
			want:      "",
		},
		{
			name: "nil id",
			dashboard: &dash0api.DashboardDefinition{
				Metadata: dash0api.DashboardMetadata{
					Dash0Extensions: &dash0api.DashboardMetadataExtensions{},
				},
			},
			want: "",
		},
		{
			name: "valid id",
			dashboard: &dash0api.DashboardDefinition{
				Metadata: dash0api.DashboardMetadata{
					Dash0Extensions: &dash0api.DashboardMetadataExtensions{
						Id: strPtr("a1b2c3d4-5678-90ab-cdef-1234567890ab"),
					},
				},
			},
			want: "a1b2c3d4-5678-90ab-cdef-1234567890ab",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractDashboardID(tt.dashboard)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestYAMLRoundTrip_Dashboard(t *testing.T) {
	fixtureData := readFixture(t, testutil.FixtureDashboardsGetSuccess)

	var original dash0api.DashboardDefinition
	require.NoError(t, json.Unmarshal(fixtureData, &original))

	yamlData, err := yaml.Marshal(&original)
	require.NoError(t, err)

	var roundTripped dash0api.DashboardDefinition
	require.NoError(t, yaml.Unmarshal(yamlData, &roundTripped))

	assertJSONEqual(t, &original, &roundTripped)
}
