package asset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

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

// assertJSONEqual marshals both values to JSON and compares them
// semantically. This is the right comparison for YAML round-trip tests
// because union types (json.RawMessage) may have different key ordering
// or whitespace after the roundtrip while still being semantically equal.
func assertJSONEqual(t *testing.T, expected, actual interface{}) {
	t.Helper()
	expectedJSON, err := json.Marshal(expected)
	require.NoError(t, err)
	actualJSON, err := json.Marshal(actual)
	require.NoError(t, err)
	assert.JSONEq(t, string(expectedJSON), string(actualJSON))
}

func readFixture(t *testing.T, relativePath string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testutil.FixturesDir(), relativePath))
	require.NoError(t, err)
	return data
}
