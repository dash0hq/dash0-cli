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

func TestExtractViewID(t *testing.T) {
	tests := []struct {
		name string
		view *dash0api.ViewDefinition
		want string
	}{
		{
			name: "nil labels",
			view: &dash0api.ViewDefinition{},
			want: "",
		},
		{
			name: "nil id",
			view: &dash0api.ViewDefinition{
				Metadata: dash0api.ViewMetadata{
					Labels: &dash0api.ViewLabels{},
				},
			},
			want: "",
		},
		{
			name: "valid id",
			view: &dash0api.ViewDefinition{
				Metadata: dash0api.ViewMetadata{
					Labels: &dash0api.ViewLabels{
						Dash0Comid: strPtr("c3d4e5f6-7890-12cd-ef01-34567890abcd"),
					},
				},
			},
			want: "c3d4e5f6-7890-12cd-ef01-34567890abcd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractViewID(tt.view)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestYAMLRoundTrip_View(t *testing.T) {
	fixtureData := readFixture(t, testutil.FixtureViewsGetSuccess)

	var original dash0api.ViewDefinition
	require.NoError(t, json.Unmarshal(fixtureData, &original))

	// Verify the fixture exercises the union types in filter values —
	// this is the exact pattern that broke with yaml.v3 serialization.
	require.NotNil(t, original.Spec.Filter)
	require.NotEmpty(t, *original.Spec.Filter)
	require.NotNil(t, (*original.Spec.Filter)[0].Values)

	yamlData, err := yaml.Marshal(&original)
	require.NoError(t, err)

	var roundTripped dash0api.ViewDefinition
	require.NoError(t, yaml.Unmarshal(yamlData, &roundTripped))

	assertJSONEqual(t, &original, &roundTripped)
}
