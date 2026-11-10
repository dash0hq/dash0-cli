package asset

import (
	"testing"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func signalToMetricsRuleWithLabels(origin, id string) *dash0api.SignalToMetricsDefinition {
	labels := &dash0api.SignalToMetricsLabels{}
	if origin != "" {
		labels.Dash0Comorigin = &origin
	}
	if id != "" {
		labels.Dash0Comid = &id
	}
	return &dash0api.SignalToMetricsDefinition{
		Metadata: dash0api.SignalToMetricsMetadata{Labels: labels},
	}
}

func TestGetSignalToMetricsRuleOrigin(t *testing.T) {
	tests := []struct {
		name string
		rule *dash0api.SignalToMetricsDefinition
		want string
	}{
		{name: "origin set", rule: signalToMetricsRuleWithLabels("dash0-cli_checkout-errors", ""), want: "dash0-cli_checkout-errors"},
		{name: "labels present, origin absent", rule: signalToMetricsRuleWithLabels("", "some-id"), want: ""},
		{name: "no labels at all", rule: &dash0api.SignalToMetricsDefinition{}, want: ""},
		{name: "nil rule", rule: nil, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, GetSignalToMetricsRuleOrigin(tt.rule))
		})
	}
}

// StripSignalToMetricsServerFields clears the origin label, so the import
// helper must read it first or every origin document would POST a duplicate.
func TestGetSignalToMetricsRuleOrigin_MustBeReadBeforeStrip(t *testing.T) {
	rule := signalToMetricsRuleWithLabels("dash0-cli_checkout-errors", "")

	require.Equal(t, "dash0-cli_checkout-errors", GetSignalToMetricsRuleOrigin(rule))

	dash0api.StripSignalToMetricsServerFields(rule)

	assert.Empty(t, GetSignalToMetricsRuleOrigin(rule),
		"Strip clears the origin label; the import helper must read it first")
}

func TestSignalToMetricsRuleKey(t *testing.T) {
	assert.Equal(t, "my-origin", SignalToMetricsRuleKey(signalToMetricsRuleWithLabels("my-origin", "my-id")), "origin wins over id")
	assert.Equal(t, "my-id", SignalToMetricsRuleKey(signalToMetricsRuleWithLabels("", "my-id")))
	assert.Empty(t, SignalToMetricsRuleKey(signalToMetricsRuleWithLabels("", "")))
	assert.Empty(t, SignalToMetricsRuleKey(nil))
}

func TestCheckSignalToMetricsRuleDataset(t *testing.T) {
	stored := func(t *testing.T, labels string) *dash0api.SignalToMetricsDefinition {
		t.Helper()
		var rule dash0api.SignalToMetricsDefinition
		require.NoError(t, dash0yaml.Unmarshal([]byte("kind: Dash0SignalToMetrics\nmetadata:\n  name: checkout-errors\n  labels:\n"+labels), &rule))
		return &rule
	}
	inDefault := "    dash0.com/dataset: default\n"
	inStaging := "    dash0.com/dataset: staging\n"

	cases := map[string]struct {
		labels  string
		dataset *string
		wantErr string
	}{
		"same dataset":                      {inStaging, dash0api.DatasetPtr("staging"), ""},
		"no dataset means default":          {inDefault, nil, ""},
		"another dataset":                   {inDefault, dash0api.DatasetPtr("staging"), `signal-to-metrics rule "my-origin" belongs to dataset "default", not "staging"`},
		"default target, staging rule":      {inStaging, nil, `belongs to dataset "staging", not "default"`},
		"stored rule without dataset label": {"    team: checkout\n", dash0api.DatasetPtr("staging"), ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := CheckSignalToMetricsRuleDataset(stored(t, c.labels), c.dataset, "my-origin")

			if c.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.wantErr)
			assert.True(t, IsSignalToMetricsRuleWrongDataset(err), "callers pass the explained error through")
		})
	}
}

func TestSignalToMetricsRuleDeletedAt(t *testing.T) {
	deletedAt := time.Date(2026, 10, 22, 11, 30, 0, 0, time.UTC)

	assert.Nil(t, SignalToMetricsRuleDeletedAt(nil))
	assert.Nil(t, SignalToMetricsRuleDeletedAt(&dash0api.SignalToMetricsDefinition{}))
	assert.Equal(t, &deletedAt, SignalToMetricsRuleDeletedAt(&dash0api.SignalToMetricsDefinition{
		Metadata: dash0api.SignalToMetricsMetadata{
			Annotations: &dash0api.SignalToMetricsGroupAnnotations{Dash0ComdeletedAt: &deletedAt},
		},
	}))
}

func TestSignalToMetricsRuleUsesOrigin(t *testing.T) {
	usesOrigin, err := SignalToMetricsRuleUsesOrigin([]byte(`kind: Dash0SignalToMetrics
metadata:
  name: checkout-errors
  labels:
    dash0.com/origin: dash0-cli_checkout-errors
`))
	require.NoError(t, err)
	assert.True(t, usesOrigin)

	usesOrigin, err = SignalToMetricsRuleUsesOrigin([]byte(`kind: Dash0SignalToMetrics
metadata:
  name: checkout-errors
  labels:
    dash0.com/id: signal_to_metrics_01kq3m8x2v7d9f4h6j1n5p0r2t
`))
	require.NoError(t, err)
	assert.False(t, usesOrigin)

	_, err = SignalToMetricsRuleUsesOrigin([]byte("metadata: ["))
	require.Error(t, err)
}

func TestKindDisplayName_SignalToMetricsRule(t *testing.T) {
	assert.Equal(t, "Signal-to-metrics rule", KindDisplayName("Dash0SignalToMetrics"))
}

func TestIsValidKind_SignalToMetricsRule(t *testing.T) {
	assert.True(t, IsValidKind("Dash0SignalToMetrics"))
	assert.False(t, IsValidKind("Dash0SignalToMetricsRule"))
}

func TestExtractIdentifier_SignalToMetricsRule(t *testing.T) {
	tests := []struct {
		name   string
		labels string
		want   string
	}{
		{
			name: "origin wins over id",
			labels: `    dash0.com/origin: dash0-cli_checkout-errors
    dash0.com/id: signal_to_metrics_01kq3m8x2v7d9f4h6j1n5p0r2t
`,
			want: "dash0-cli_checkout-errors",
		},
		{
			name: "id when origin is absent",
			labels: `    dash0.com/id: signal_to_metrics_01kq3m8x2v7d9f4h6j1n5p0r2t
`,
			want: "signal_to_metrics_01kq3m8x2v7d9f4h6j1n5p0r2t",
		},
		{name: "neither label", labels: "    team: checkout\n", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identifier, err := ExtractIdentifier([]byte(`kind: Dash0SignalToMetrics
metadata:
  name: checkout-errors
  labels:
` + tt.labels))
			require.NoError(t, err)
			assert.Equal(t, tt.want, identifier)
		})
	}
}
