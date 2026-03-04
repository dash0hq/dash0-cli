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

func TestExtractCheckRuleID(t *testing.T) {
	tests := []struct {
		name string
		rule *dash0api.PrometheusAlertRule
		want string
	}{
		{
			name: "nil id",
			rule: &dash0api.PrometheusAlertRule{},
			want: "",
		},
		{
			name: "valid id",
			rule: &dash0api.PrometheusAlertRule{
				Id: strPtr("b2c3d4e5-6789-01bc-def0-234567890abc"),
			},
			want: "b2c3d4e5-6789-01bc-def0-234567890abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractCheckRuleID(tt.rule)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestYAMLRoundTrip_CheckRule(t *testing.T) {
	fixtureData := readFixture(t, testutil.FixtureCheckRulesGetSuccess)

	var original dash0api.PrometheusAlertRule
	require.NoError(t, json.Unmarshal(fixtureData, &original))

	yamlData, err := yaml.Marshal(&original)
	require.NoError(t, err)

	var roundTripped dash0api.PrometheusAlertRule
	require.NoError(t, yaml.Unmarshal(yamlData, &roundTripped))

	assertJSONEqual(t, &original, &roundTripped)
}
