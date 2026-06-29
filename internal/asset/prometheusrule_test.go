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
