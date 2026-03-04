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

func TestExtractSyntheticCheckID(t *testing.T) {
	tests := []struct {
		name  string
		check *dash0api.SyntheticCheckDefinition
		want  string
	}{
		{
			name:  "nil labels",
			check: &dash0api.SyntheticCheckDefinition{},
			want:  "",
		},
		{
			name: "nil id",
			check: &dash0api.SyntheticCheckDefinition{
				Metadata: dash0api.SyntheticCheckMetadata{
					Labels: &dash0api.SyntheticCheckLabels{},
				},
			},
			want: "",
		},
		{
			name: "valid id",
			check: &dash0api.SyntheticCheckDefinition{
				Metadata: dash0api.SyntheticCheckMetadata{
					Labels: &dash0api.SyntheticCheckLabels{
						Dash0Comid: strPtr("d4e5f6a7-8901-23de-f012-4567890abcde"),
					},
				},
			},
			want: "d4e5f6a7-8901-23de-f012-4567890abcde",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractSyntheticCheckID(tt.check)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestYAMLRoundTrip_SyntheticCheck(t *testing.T) {
	fixtureData := readFixture(t, testutil.FixtureSyntheticChecksGetSuccess)

	var original dash0api.SyntheticCheckDefinition
	require.NoError(t, json.Unmarshal(fixtureData, &original))

	yamlData, err := yaml.Marshal(&original)
	require.NoError(t, err)

	var roundTripped dash0api.SyntheticCheckDefinition
	require.NoError(t, yaml.Unmarshal(yamlData, &roundTripped))

	assertJSONEqual(t, &original, &roundTripped)
}
