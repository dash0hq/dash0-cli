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

// TestParseCheckRules_SingleAlertKeepsSharedID pins that a single-alert CRD's
// one check rule keeps the CRD's own dash0.com/id verbatim -- there is only
// ever one alert to upsert, so the shared id unambiguously names it, and
// existing single-alert users' check rules must keep resolving to the same
// id they've always had.
func TestParseCheckRules_SingleAlertKeepsSharedID(t *testing.T) {
	crd := []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: single
  labels:
    dash0.com/id: shared-id
spec:
  groups:
    - name: g
      rules:
        - alert: HighErrorRate
          expr: errors > 0
`)

	rules, err := ParseCheckRules(crd)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.NotNil(t, rules[0].Id)
	assert.Equal(t, "shared-id", *rules[0].Id)
}

// TestParseCheckRules_MultiAlertDerivesDistinctIDs is a regression test for
// a bug where every alert in a multi-alert PrometheusRule CRD got the exact
// same check-rule id (the CRD's own shared dash0.com/id), so each alert's
// upsert (PUT, create-or-*replace*) silently overwrote whatever the
// previous alert in the same apply run had just written: only the last
// alert in document order ended up with a real check rule server-side,
// even though the CLI reported success for both. Each alert must now get
// its own distinct, non-empty id derived from the shared id and its own
// composed name.
func TestParseCheckRules_MultiAlertDerivesDistinctIDs(t *testing.T) {
	crd := []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: multi
  labels:
    dash0.com/id: shared-id
spec:
  groups:
    - name: test-group
      rules:
        - alert: HighErrorRate
          expr: errors > 0
        - alert: DiskFull
          expr: disk > 0
`)

	rules, err := ParseCheckRules(crd)
	require.NoError(t, err)
	require.Len(t, rules, 2)

	require.NotNil(t, rules[0].Id)
	require.NotNil(t, rules[1].Id)
	assert.NotEqual(t, *rules[0].Id, *rules[1].Id, "each alert must get its own id, not the CRD's shared id repeated")
	assert.NotEqual(t, "shared-id", *rules[0].Id, "the derived id must not collide with the CRD's own literal shared id either")
	assert.NotEqual(t, "shared-id", *rules[1].Id)
	assert.Equal(t, "shared-id--test-group-higherrorrate", *rules[0].Id)
	assert.Equal(t, "shared-id--test-group-diskfull", *rules[1].Id)
}

// TestParseCheckRules_MultiAlertDerivedIDsAreStableAcrossReapply pins the
// idempotency property the derivation depends on: re-parsing the identical
// CRD content must produce the identical derived ids, so repeated applies
// keep upserting the same check rules rather than creating new ones each
// time.
func TestParseCheckRules_MultiAlertDerivedIDsAreStableAcrossReapply(t *testing.T) {
	crd := []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: multi
  labels:
    dash0.com/id: shared-id
spec:
  groups:
    - name: test-group
      rules:
        - alert: HighErrorRate
          expr: errors > 0
        - alert: DiskFull
          expr: disk > 0
`)

	first, err := ParseCheckRules(crd)
	require.NoError(t, err)
	second, err := ParseCheckRules(crd)
	require.NoError(t, err)

	require.Len(t, first, 2)
	require.Len(t, second, 2)
	assert.Equal(t, *first[0].Id, *second[0].Id)
	assert.Equal(t, *first[1].Id, *second[1].Id)
}

func TestSlugify(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"test-group - DiskFull", "test-group-diskfull"},
		{"g - HighErrorRate", "g-higherrorrate"},
		{"Group A - Alert/With Slashes", "group-a-alert-with-slashes"},
		{"  leading and trailing  ", "leading-and-trailing"},
		{"UPPER_CASE", "upper-case"},
		{"", ""},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, slugify(c.in), "slugify(%q)", c.in)
	}
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
