package asset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDashboardInput_NativeDashboard(t *testing.T) {
	data := []byte(`kind: Dashboard
metadata:
  name: My Dashboard
  dash0Extensions:
    id: d1e2f3a4-5678-90ab-cdef-1234567890ab
spec:
  display:
    name: My Dashboard
`)
	dashboard, err := ParseDashboardInput(data)
	require.NoError(t, err)
	assert.Equal(t, "My Dashboard", ExtractDashboardDisplayName(dashboard))
	assert.Equal(t, "d1e2f3a4-5678-90ab-cdef-1234567890ab", ExtractDashboardID(dashboard))
}

func TestParseDashboardInput_PersesDashboard(t *testing.T) {
	data := []byte(`apiVersion: perses.dev/v1alpha1
kind: PersesDashboard
metadata:
  name: my-perses-dashboard
  labels:
    dash0.com/id: p1e2r3s4-5678-90ab-cdef-1234567890ab
spec:
  display:
    name: My Perses Dashboard
  duration: 5m
  panels: {}
`)
	dashboard, err := ParseDashboardInput(data)
	require.NoError(t, err)
	assert.Equal(t, "My Perses Dashboard", ExtractDashboardDisplayName(dashboard))
	assert.Equal(t, "p1e2r3s4-5678-90ab-cdef-1234567890ab", ExtractDashboardID(dashboard))
}

func TestParseDashboardInput_PersesDashboardV1alpha2(t *testing.T) {
	data := []byte(`apiVersion: perses.dev/v1alpha2
kind: PersesDashboard
metadata:
  name: my-perses-v2
spec:
  config:
    display:
      name: V2 Dashboard
    panels: {}
`)
	dashboard, err := ParseDashboardInput(data)
	require.NoError(t, err)
	assert.Equal(t, "V2 Dashboard", ExtractDashboardDisplayName(dashboard))
}

func TestParseDashboardInput_InvalidYAML(t *testing.T) {
	data := []byte(`{invalid yaml`)
	_, err := ParseDashboardInput(data)
	require.Error(t, err)
}

func TestParseCheckRuleInputs_NativeCheckRule(t *testing.T) {
	data := []byte(`kind: CheckRule
id: b2c3d4e5-6789-01bc-def0-234567890abc
name: High Error Rate
expression: sum(rate(http_requests_total{status=~"5.."}[5m])) > 0.1
`)
	rules, err := ParseCheckRuleInputs(data)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "High Error Rate", rules[0].Name)
	assert.Equal(t, "b2c3d4e5-6789-01bc-def0-234567890abc", *rules[0].Id)
}

func TestParseCheckRuleInputs_PrometheusRuleSingleAlert(t *testing.T) {
	data := []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: test-rule
  labels:
    dash0.com/id: ca9af402-03aa-4c77-8a81-b4960b5126fd
spec:
  groups:
    - name: Alerting
      interval: 2m
      rules:
        - alert: High Error Rate
          expr: rate(errors[5m]) > 0.1
          for: 5m
          annotations:
            summary: Error rate is high
`)
	rules, err := ParseCheckRuleInputs(data)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "High Error Rate", rules[0].Name)
	assert.Equal(t, "ca9af402-03aa-4c77-8a81-b4960b5126fd", *rules[0].Id)
	assert.Equal(t, "2m", string(*rules[0].Interval))
	assert.Equal(t, "Error rate is high", *rules[0].Summary)
}

func TestParseCheckRuleInputs_PrometheusRuleMultipleAlerts(t *testing.T) {
	data := []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: multi-rule
spec:
  groups:
    - name: Alerting
      rules:
        - alert: Alert One
          expr: up == 0
        - alert: Alert Two
          expr: up == 1
`)
	rules, err := ParseCheckRuleInputs(data)
	require.NoError(t, err)
	require.Len(t, rules, 2)
	assert.Equal(t, "Alert One", rules[0].Name)
	assert.Equal(t, "Alert Two", rules[1].Name)
}

func TestParseCheckRuleInputs_PrometheusRuleSkipsRecordingRules(t *testing.T) {
	data := []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: mixed-rule
spec:
  groups:
    - name: Rules
      rules:
        - record: http_requests:rate5m
          expr: rate(http_requests_total[5m])
        - alert: High Error Rate
          expr: rate(errors[5m]) > 0.1
`)
	rules, err := ParseCheckRuleInputs(data)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "High Error Rate", rules[0].Name)
}

func TestParseCheckRuleInputs_PrometheusRuleNoAlerts(t *testing.T) {
	data := []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: recording-only
spec:
  groups:
    - name: Rules
      rules:
        - record: http_requests:rate5m
          expr: rate(http_requests_total[5m])
`)
	_, err := ParseCheckRuleInputs(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no alerting rules found")
}

func TestParseCheckRuleInputs_InvalidYAML(t *testing.T) {
	data := []byte(`{invalid yaml`)
	_, err := ParseCheckRuleInputs(data)
	require.Error(t, err)
}

func TestParseCheckRuleInputs_InferredCheckRule(t *testing.T) {
	data := []byte(`name: Inferred Rule
expression: up == 0
`)
	rules, err := ParseCheckRuleInputs(data)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "Inferred Rule", rules[0].Name)
}
