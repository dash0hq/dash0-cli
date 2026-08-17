package asset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCheckRules_PrometheusRuleComposesName(t *testing.T) {
	// Matches the reported case: the check-rule name must be
	// "<group name> - <alert name>", as the operator and Terraform provider produce.
	crd := []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: ticketprovider-sapi-password-sweep-disabled
  labels:
    dash0.com/id: 1e9e4743-2701-4082-a402-8047dc4c78d0
spec:
  groups:
    - name: ticketprovider-sapi-password-sweep-disabled
      interval: 1m
      rules:
        - alert: ticketprovider / ticketprovider-sb / Password Sweep Disabled
          expr: up == 0
          for: 5m
`)

	rules, err := ParseCheckRules(crd)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "ticketprovider-sapi-password-sweep-disabled - ticketprovider / ticketprovider-sb / Password Sweep Disabled", rules[0].Name)
}

func TestParseCheckRules_MultiGroupSkipsRecordingRules(t *testing.T) {
	crd := []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: mixed
spec:
  groups:
    - name: group-a
      rules:
        - alert: HighErrorRate
          expr: errors > 0
        - record: instance:cpu:avg
          expr: avg(cpu)
    - name: group-b
      rules:
        - alert: DiskFull
          expr: disk > 0
`)

	rules, err := ParseCheckRules(crd)
	require.NoError(t, err)
	require.Len(t, rules, 2)
	// Names zip onto the alerting rules in document order; the recording rule is skipped.
	assert.Equal(t, "group-a - HighErrorRate", rules[0].Name)
	assert.Equal(t, "group-b - DiskFull", rules[1].Name)
}

// TestParseCheckRules_BooleanLiteralAlertNamePreserved is a regression test
// for a bug where an alert name that is a YAML boolean literal (Y, N, yes,
// no, on, off, true, false, and case variants), written unquoted, was
// silently corrupted to "true"/"false": sigs.k8s.io/yaml's YAML->JSON->struct
// unmarshal path resolves the literal to a real JSON boolean, then coerces
// it into the destination *string field instead of erroring. Confirmed
// directly, unmarshaling `alert: Y` into a struct with an `Alert *string`
// field sets Alert to "true", not "Y".
func TestParseCheckRules_BooleanLiteralAlertNamePreserved(t *testing.T) {
	cases := []string{"Y", "N", "yes", "No", "ON", "off", "true", "False"}
	for _, alertName := range cases {
		crd := []byte("apiVersion: monitoring.coreos.com/v1\n" +
			"kind: PrometheusRule\n" +
			"metadata:\n" +
			"  name: boolean-literal-test\n" +
			"spec:\n" +
			"  groups:\n" +
			"    - name: g\n" +
			"      rules:\n" +
			"        - alert: " + alertName + "\n" +
			"          expr: up == 0\n")

		rules, err := ParseCheckRules(crd)
		require.NoError(t, err, "alert name %q", alertName)
		require.Len(t, rules, 1, "alert name %q", alertName)
		assert.Equal(t, "g - "+alertName, rules[0].Name, "alert name %q must be preserved verbatim, not coerced into a boolean and re-stringified", alertName)
	}
}

func TestExtractPrometheusAlertNames_BooleanLiteralPreserved(t *testing.T) {
	crd := []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: boolean-literal-test
spec:
  groups:
    - name: g
      rules:
        - alert: Y
          expr: up == 0
        - alert: N
          expr: up == 1
`)
	names, err := ExtractPrometheusAlertNames(crd)
	require.NoError(t, err)
	require.Len(t, names, 2)
	assert.Equal(t, "g - Y", names[0].CheckRuleName())
	assert.Equal(t, "g - N", names[1].CheckRuleName())
}

func TestPrometheusRuleEndpoints_AlertingOnly(t *testing.T) {
	crd := []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: alerting-only
spec:
  groups:
    - name: group-a
      rules:
        - alert: HighErrorRate
          expr: errors > 0
`)
	hasAlerts, hasRecords, err := PrometheusRuleEndpoints(crd)
	require.NoError(t, err)
	assert.True(t, hasAlerts)
	assert.False(t, hasRecords)
}

func TestPrometheusRuleEndpoints_RecordingOnly(t *testing.T) {
	crd := []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: recording-only
spec:
  groups:
    - name: group-a
      rules:
        - record: instance:cpu:avg
          expr: avg(cpu)
`)
	hasAlerts, hasRecords, err := PrometheusRuleEndpoints(crd)
	require.NoError(t, err)
	assert.False(t, hasAlerts)
	assert.True(t, hasRecords)
}

func TestPrometheusRuleEndpoints_Mixed(t *testing.T) {
	crd := []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: mixed
spec:
  groups:
    - name: group-a
      rules:
        - alert: HighErrorRate
          expr: errors > 0
        - record: instance:cpu:avg
          expr: avg(cpu)
`)
	hasAlerts, hasRecords, err := PrometheusRuleEndpoints(crd)
	require.NoError(t, err)
	assert.True(t, hasAlerts)
	assert.True(t, hasRecords)
}

func TestPrometheusRuleEndpoints_NonPrometheusRuleKind(t *testing.T) {
	doc := []byte(`kind: CheckRule
id: some-id
name: High Error Rate
expression: up == 0
`)
	hasAlerts, hasRecords, err := PrometheusRuleEndpoints(doc)
	require.NoError(t, err)
	assert.False(t, hasAlerts)
	assert.False(t, hasRecords)
}

func TestParseCheckRules_PlainCheckRuleKeepsName(t *testing.T) {
	doc := []byte(`kind: CheckRule
id: b2c3d4e5-6789-01bc-def0-234567890abc
name: High Error Rate
expression: up == 0
`)

	rules, err := ParseCheckRules(doc)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "High Error Rate", rules[0].Name)
}
