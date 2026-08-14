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

// The CRD below is trimmed to only what the assertions below exercise:
// annotation merge and override. It omits labels, `for`, and PromQL detail
// that no assertion touches.
func TestParseCheckRules_TopLevelAnnotationsMergeIntoRules(t *testing.T) {
	crd := []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: checkout-check-rules
  namespace: monitoring
  annotations:
    dash0.com/notification-channel-ids: "3fa42d0c-6b8e-4c1a-9f2d-111111111111,3fa42d0c-6b8e-4c1a-9f2d-222222222222"
spec:
  groups:
    - name: Alerting
      interval: 1m
      rules:
        - alert: CheckoutHighLatency
          expr: up == 0
        - alert: CheckoutHighErrorRate
          expr: up == 0
          annotations:
            summary: "Checkout error rate is elevated"
            dash0.com/notification-channel-ids: "3fa42d0c-6b8e-4c1a-9f2d-333333333333"
            runbook_url: "https://runbooks.example.com/checkout-error-rate"
`)

	rules, err := ParseCheckRules(crd)
	require.NoError(t, err)
	require.Len(t, rules, 2)

	// Rule 1 declared no annotations of its own, so it must inherit the
	// top-level dash0.com/notification-channel-ids value verbatim.
	latencyRule := rules[0]
	require.NotNil(t, latencyRule.Annotations)
	require.NotNil(t, latencyRule.Annotations.AdditionalProperties)
	assert.Equal(t,
		"3fa42d0c-6b8e-4c1a-9f2d-111111111111,3fa42d0c-6b8e-4c1a-9f2d-222222222222",
		latencyRule.Annotations.AdditionalProperties["dash0.com/notification-channel-ids"],
	)

	// Rule 2's own dash0.com/notification-channel-ids overrides the
	// top-level value (rule-level wins on conflict) and its unique
	// runbook_url annotation is preserved.
	errorRateRule := rules[1]
	require.NotNil(t, errorRateRule.Annotations)
	require.NotNil(t, errorRateRule.Annotations.AdditionalProperties)
	assert.Equal(t,
		"3fa42d0c-6b8e-4c1a-9f2d-333333333333",
		errorRateRule.Annotations.AdditionalProperties["dash0.com/notification-channel-ids"],
	)
	assert.Equal(t,
		"https://runbooks.example.com/checkout-error-rate",
		errorRateRule.Annotations.AdditionalProperties["runbook_url"],
	)
	// The rule's own value must win outright, not be appended alongside the
	// top-level one: assert the map has exactly the two additional keys
	// expected (notification-channel-ids, runbook_url); summary is extracted
	// onto its own dedicated struct field instead, asserted below.
	assert.Len(t, errorRateRule.Annotations.AdditionalProperties, 2)
	require.NotNil(t, errorRateRule.Annotations.Summary)
	assert.Equal(t, "Checkout error rate is elevated", *errorRateRule.Annotations.Summary)
}

// The critical and degraded thresholds are two independent annotations, so a
// document may set one at the top level and the other on the rule. They do not
// collide on a key, and the merge is per key rather than a map replacement, so
// both must survive onto the same check rule. A rule that restates a threshold
// still wins for that threshold alone.
func TestParseCheckRules_ThresholdsMergeFromBothLevels(t *testing.T) {
	crd := []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: checkout-thresholds
  annotations:
    dash0-threshold-critical: "1000"
spec:
  groups:
    - name: Alerting
      rules:
        - alert: InheritsCriticalOnly
          expr: up > $__threshold
        - alert: AddsDegradedToInheritedCritical
          expr: up > $__threshold
          annotations:
            dash0-threshold-degraded: "500"
        - alert: OverridesBothThresholds
          expr: up > $__threshold
          annotations:
            dash0-threshold-critical: "42"
            dash0-threshold-degraded: "7"
`)

	rules, err := ParseCheckRules(crd)
	require.NoError(t, err)
	require.Len(t, rules, 3)

	// Declaring no annotations, this rule inherits the top-level critical
	// threshold and has no degraded threshold at all.
	inherited := rules[0]
	require.NotNil(t, inherited.Thresholds)
	require.NotNil(t, inherited.Thresholds.Failed)
	assert.Equal(t, float64(1000), *inherited.Thresholds.Failed)
	assert.Nil(t, inherited.Thresholds.Degraded)

	// The rule sets only the degraded threshold. Setting it must not discard
	// the critical threshold coming from the top level: both apply.
	split := rules[1]
	require.NotNil(t, split.Thresholds)
	require.NotNil(t, split.Thresholds.Failed)
	require.NotNil(t, split.Thresholds.Degraded)
	assert.Equal(t, float64(1000), *split.Thresholds.Failed)
	assert.Equal(t, float64(500), *split.Thresholds.Degraded)

	// The rule restates the critical threshold, so its own value wins.
	overridden := rules[2]
	require.NotNil(t, overridden.Thresholds)
	require.NotNil(t, overridden.Thresholds.Failed)
	require.NotNil(t, overridden.Thresholds.Degraded)
	assert.Equal(t, float64(42), *overridden.Thresholds.Failed)
	assert.Equal(t, float64(7), *overridden.Thresholds.Degraded)
}
