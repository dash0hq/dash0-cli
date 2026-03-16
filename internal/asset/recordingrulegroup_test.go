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

func TestExtractRecordingRuleGroupID(t *testing.T) {
	tests := []struct {
		name  string
		group *dash0api.RecordingRuleGroupDefinition
		want  string
	}{
		{
			name:  "nil labels",
			group: &dash0api.RecordingRuleGroupDefinition{},
			want:  "",
		},
		{
			name: "nil origin and nil id",
			group: &dash0api.RecordingRuleGroupDefinition{
				Metadata: dash0api.RecordingRuleGroupMetadata{
					Labels: &dash0api.RecordingRuleGroupLabels{},
				},
			},
			want: "",
		},
		{
			name: "origin takes precedence over id",
			group: &dash0api.RecordingRuleGroupDefinition{
				Metadata: dash0api.RecordingRuleGroupMetadata{
					Labels: &dash0api.RecordingRuleGroupLabels{
						Dash0Comid:     strPtr("aaaa1111-1234-5678-abcd-000000000001"),
						Dash0Comorigin: strPtr("tf_aaaa1111-1234-5678-abcd-000000000001"),
					},
				},
			},
			want: "tf_aaaa1111-1234-5678-abcd-000000000001",
		},
		{
			name: "falls back to id when origin is empty",
			group: &dash0api.RecordingRuleGroupDefinition{
				Metadata: dash0api.RecordingRuleGroupMetadata{
					Labels: &dash0api.RecordingRuleGroupLabels{
						Dash0Comid:     strPtr("aaaa1111-1234-5678-abcd-000000000001"),
						Dash0Comorigin: strPtr(""),
					},
				},
			},
			want: "aaaa1111-1234-5678-abcd-000000000001",
		},
		{
			name: "only id set",
			group: &dash0api.RecordingRuleGroupDefinition{
				Metadata: dash0api.RecordingRuleGroupMetadata{
					Labels: &dash0api.RecordingRuleGroupLabels{
						Dash0Comid: strPtr("bbbb2222-1234-5678-abcd-000000000002"),
					},
				},
			},
			want: "bbbb2222-1234-5678-abcd-000000000002",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractRecordingRuleGroupID(tt.group)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractRecordingRuleGroupName(t *testing.T) {
	tests := []struct {
		name  string
		group *dash0api.RecordingRuleGroupDefinition
		want  string
	}{
		{
			name: "display name takes precedence",
			group: &dash0api.RecordingRuleGroupDefinition{
				Metadata: dash0api.RecordingRuleGroupMetadata{Name: "metadata-name"},
				Spec:     dash0api.RecordingRuleGroupSpec{Display: dash0api.RecordingRuleGroupDisplay{Name: "Display Name"}},
			},
			want: "Display Name",
		},
		{
			name: "falls back to metadata name",
			group: &dash0api.RecordingRuleGroupDefinition{
				Metadata: dash0api.RecordingRuleGroupMetadata{Name: "metadata-name"},
			},
			want: "metadata-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractRecordingRuleGroupName(tt.group)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestYAMLRoundTrip_RecordingRuleGroup(t *testing.T) {
	fixtureData := readFixture(t, testutil.FixtureRecordingRuleGroupsGetSuccess)

	var original dash0api.RecordingRuleGroupDefinition
	require.NoError(t, json.Unmarshal(fixtureData, &original))

	yamlData, err := yaml.Marshal(&original)
	require.NoError(t, err)

	var roundTripped dash0api.RecordingRuleGroupDefinition
	require.NoError(t, yaml.Unmarshal(yamlData, &roundTripped))

	assertJSONEqual(t, &original, &roundTripped)
}
