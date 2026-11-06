package asset

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func transformationFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(testutil.FixturesDir() + "/telemetrytransformationrules/get_success.json")
	require.NoError(t, err)
	return raw
}

func TestParseTelemetryTransformationRule(t *testing.T) {
	raw := transformationFixture(t)
	rule, err := ParseTelemetryTransformationRule(raw)
	require.NoError(t, err)
	assert.False(t, rule.Spec.Enabled)
	require.NotNil(t, rule.Spec.Priority)
	assert.Zero(t, *rule.Spec.Priority)
	assert.Equal(t, "z-normalize", rule.Spec.Transformations[0].Name)
	assert.Equal(t, "set(attributes[\"original\"], body)", rule.Spec.Transformations[0].Statements[0])
	require.NotNil(t, rule.Spec.Examples)
	assert.Equal(t, "Synthetic log", (*rule.Spec.Examples)[0].Name)

	for _, tc := range []struct{ from, to, message string }{
		{`"enabled": false,`, ``, "spec.enabled is required"},
		{`"enabled": false`, `"enabled": null`, "spec.enabled is required"},
		{`"context": "log"`, `"context": "span"`, "not valid for signal"},
		{`"priority": 0`, `"priority": 50`, "spec.priority"},
		{`"priority": 0`, `"priority": 500`, "spec.priority"},
		{`"signal": "logs"`, `"signal": "unknown"`, "invalid telemetry transformation signal"},
		{`"name": "z-normalize",`, ``, "spec.transformations[0].name is required"},
		{`"name": "Synthetic log",`, ``, "spec.examples[0].name is required"},
		{`"priority": 0`, `"statements": []`, `unknown field "statements"`},
		{`"conditions": [`, `"conditions": [null,`, "spec.conditions[0] must be a string"},
		{`"statements": [`, `"statements": [null,`, "spec.transformations[0].statements[0] must be a string"},
	} {
		t.Run(tc.message+tc.to, func(t *testing.T) {
			_, err := ParseTelemetryTransformationRule(bytes.Replace(raw, []byte(tc.from), []byte(tc.to), 1))
			require.ErrorContains(t, err, tc.message)
		})
	}

	t.Run("large integers and YAML boolean-looking keys", func(t *testing.T) {
		rule, err := ParseTelemetryTransformationRule([]byte(`kind: Dash0TelemetryTransformationRule
metadata:
  name: precision
spec:
  enabled: false
  signal: logs
  context: log
  transformations:
    - name: ""
      statements: []
  examples:
    - name: ""
      otlp:
        y: 9007199254740993
        nested:
          on: false
`))
		require.NoError(t, err)
		payload := (*rule.Spec.Examples)[0].Otlp
		assert.Equal(t, json.Number("9007199254740993"), payload["y"])
		assert.Equal(t, false, payload["nested"].(map[string]interface{})["on"])
	})
}

func TestTelemetryTransformationRuleDiff(t *testing.T) {
	before, err := ParseTelemetryTransformationRule(transformationFixture(t))
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		change func(*dash0api.TelemetryTransformationRuleDefinition)
		want   string
	}{
		{"metadata only", func(r *dash0api.TelemetryTransformationRuleDefinition) {
			r.Metadata.Labels = nil
			r.Metadata.Annotations = nil
		}, "no changes"},
		{"transformations order", func(r *dash0api.TelemetryTransformationRuleDefinition) { slices.Reverse(r.Spec.Transformations) }, "+  - name: a-tag"},
		{"statements order", func(r *dash0api.TelemetryTransformationRuleDefinition) {
			slices.Reverse(r.Spec.Transformations[0].Statements)
		}, "+    - set(body"},
		{"examples removed", func(r *dash0api.TelemetryTransformationRuleDefinition) { r.Spec.Examples = nil }, "-  examples:"},
		{"examples edited", func(r *dash0api.TelemetryTransformationRuleDefinition) {
			(*r.Spec.Examples)[0].Name = "Changed example"
		}, "+  - name: Changed example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			after, err := ParseTelemetryTransformationRule(transformationFixture(t))
			require.NoError(t, err)
			tc.change(after)
			var out bytes.Buffer
			require.NoError(t, PrintDiff(&out, "Telemetry transformation rule", "Normalize logs", before, after))
			assert.Contains(t, out.String(), tc.want)
		})
	}
	assert.NotNil(t, before.Metadata.Annotations.Dash0ComupdatedAt)
	assert.Equal(t, "z-normalize", before.Spec.Transformations[0].Name)
}
