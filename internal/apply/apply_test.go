package apply

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeKind(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Dashboard", "dashboard"},
		{"dashboard", "dashboard"},
		{"DASHBOARD", "dashboard"},
		{"CheckRule", "checkrule"},
		{"check-rule", "checkrule"},
		{"check_rule", "checkrule"},
		{"Dash0Dashboard", "dashboard"},
		{"Dash0CheckRule", "checkrule"},
		{"PrometheusRule", "prometheusrule"},
		{"SyntheticCheck", "syntheticcheck"},
		{"synthetic-check", "syntheticcheck"},
		{"View", "view"},
		{"Dash0View", "view"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeKind(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsValidKind(t *testing.T) {
	validKinds := []string{
		"Dashboard",
		"dashboard",
		"CheckRule",
		"checkrule",
		"check-rule",
		"PrometheusRule",
		"prometheusrule",
		"SyntheticCheck",
		"syntheticcheck",
		"synthetic-check",
		"View",
		"view",
		"Dash0Dashboard",
		"Dash0View",
	}

	for _, kind := range validKinds {
		t.Run("valid_"+kind, func(t *testing.T) {
			assert.True(t, isValidKind(kind), "expected %q to be valid", kind)
		})
	}

	invalidKinds := []string{
		"Unknown",
		"Pod",
		"Deployment",
		"ConfigMap",
		"",
		"   ",
	}

	for _, kind := range invalidKinds {
		t.Run("invalid_"+kind, func(t *testing.T) {
			assert.False(t, isValidKind(kind), "expected %q to be invalid", kind)
		})
	}
}

func TestReadMultiDocumentYAML_SingleDocument(t *testing.T) {
	yaml := `kind: Dashboard
metadata:
  name: test-dashboard
spec:
  display:
    name: Test Dashboard
`
	docs, err := readMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "Dashboard", docs[0].Kind)
}

func TestReadMultiDocumentYAML_MultipleDocuments(t *testing.T) {
	yaml := `kind: Dashboard
metadata:
  name: dashboard-1
---
kind: CheckRule
name: check-rule-1
---
kind: View
metadata:
  name: view-1
`
	docs, err := readMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	require.Len(t, docs, 3)
	assert.Equal(t, "Dashboard", docs[0].Kind)
	assert.Equal(t, "CheckRule", docs[1].Kind)
	assert.Equal(t, "View", docs[2].Kind)
}

func TestReadMultiDocumentYAML_WithEmptyDocuments(t *testing.T) {
	// The parser will include documents without a kind field
	// Validation of the kind field happens in a separate step
	yaml := `---
kind: Dashboard
metadata:
  name: dashboard-1
---
---
kind: View
metadata:
  name: view-1
---
`
	docs, err := readMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	// Parser creates documents for each YAML document, including empty ones
	// The actual kind validation happens later in runApply
	require.GreaterOrEqual(t, len(docs), 2)

	// Filter to find documents with valid kinds
	var validDocs []resourceDocument
	for _, doc := range docs {
		if doc.Kind != "" {
			validDocs = append(validDocs, doc)
		}
	}
	require.Len(t, validDocs, 2)
	assert.Equal(t, "Dashboard", validDocs[0].Kind)
	assert.Equal(t, "View", validDocs[1].Kind)
}

func TestReadMultiDocumentYAML_EmptyInput(t *testing.T) {
	_, err := readMultiDocumentYAML("-", strings.NewReader(""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no input provided")
}

func TestReadMultiDocumentYAML_InvalidYAML(t *testing.T) {
	yaml := `kind: Dashboard
  invalid yaml: [
    unclosed bracket
`
	_, err := readMultiDocumentYAML("-", strings.NewReader(yaml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse YAML")
}

func TestReadMultiDocumentYAML_PreservesRawContent(t *testing.T) {
	yaml := `kind: CheckRule
name: test-rule
expression: up == 0
labels:
  severity: critical
`
	docs, err := readMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	require.Len(t, docs, 1)

	// Verify the raw content contains expected fields
	raw := string(docs[0].raw)
	assert.Contains(t, raw, "name: test-rule")
	assert.Contains(t, raw, "expression: up == 0")
	assert.Contains(t, raw, "severity: critical")
}

func TestApplyAction_String(t *testing.T) {
	assert.Equal(t, "created", string(actionCreated))
	assert.Equal(t, "updated", string(actionUpdated))
}

func TestConvertToCheckRule(t *testing.T) {
	rule := &PrometheusRule_{
		Alert: "HighErrorRate",
		Expr:  "sum(rate(errors[5m])) > 0.1",
		For:   "5m",
		Labels: map[string]string{
			"severity": "critical",
		},
		Annotations: map[string]string{
			"summary":     "High error rate detected",
			"description": "Error rate exceeds threshold",
		},
	}

	checkRule := convertToCheckRule(rule, "1m", "test-id")

	assert.Equal(t, "HighErrorRate", checkRule.Name)
	assert.Equal(t, "sum(rate(errors[5m])) > 0.1", checkRule.Expression)
	assert.NotNil(t, checkRule.For)
	assert.Equal(t, "5m", string(*checkRule.For))
	assert.NotNil(t, checkRule.Interval)
	assert.Equal(t, "1m", string(*checkRule.Interval))
	assert.NotNil(t, checkRule.Id)
	assert.Equal(t, "test-id", *checkRule.Id)
	assert.NotNil(t, checkRule.Summary)
	assert.Equal(t, "High error rate detected", *checkRule.Summary)
	assert.NotNil(t, checkRule.Description)
	assert.Equal(t, "Error rate exceeds threshold", *checkRule.Description)
	// Check that origin label is set
	require.NotNil(t, checkRule.Labels)
	assert.Equal(t, "dash0-cli", (*checkRule.Labels)["dash0.com/origin"])
}

func TestConvertToCheckRule_MinimalInput(t *testing.T) {
	rule := &PrometheusRule_{
		Alert: "SimpleAlert",
		Expr:  "up == 0",
	}

	checkRule := convertToCheckRule(rule, "", "")

	assert.Equal(t, "SimpleAlert", checkRule.Name)
	assert.Equal(t, "up == 0", checkRule.Expression)
	assert.Nil(t, checkRule.For)
	assert.Nil(t, checkRule.Interval)
	assert.Nil(t, checkRule.Id)
	// Origin label should still be set
	require.NotNil(t, checkRule.Labels)
	assert.Equal(t, "dash0-cli", (*checkRule.Labels)["dash0.com/origin"])
}

func TestPrometheusRuleParsing(t *testing.T) {
	yaml := `apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: test-rules
  labels:
    dash0.com/id: test-prom-rule-id
spec:
  groups:
    - name: test-group
      interval: 1m
      rules:
        - alert: HighErrorRate
          expr: sum(rate(errors[5m])) > 0.1
          for: 5m
          labels:
            severity: critical
          annotations:
            summary: High error rate detected
`
	docs, err := readMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "PrometheusRule", docs[0].Kind)
}

func TestReadMultiDocumentYAML_FromBuffer(t *testing.T) {
	yaml := `kind: Dashboard
metadata:
  name: buffer-test
`
	buf := bytes.NewBufferString(yaml)
	docs, err := readMultiDocumentYAML("-", buf)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "Dashboard", docs[0].Kind)
}
