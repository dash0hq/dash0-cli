package asset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractPrometheusRuleID(t *testing.T) {
	tests := []struct {
		name string
		rule *PrometheusRule
		want string
	}{
		{
			name: "nil labels",
			rule: &PrometheusRule{},
			want: "",
		},
		{
			name: "empty labels",
			rule: &PrometheusRule{
				Metadata: PrometheusRuleMetadata{
					Labels: map[string]string{},
				},
			},
			want: "",
		},
		{
			name: "id present",
			rule: &PrometheusRule{
				Metadata: PrometheusRuleMetadata{
					Labels: map[string]string{
						"dash0.com/id": "ca9af402-03aa-4c77-8a81-b4960b5126fd",
					},
				},
			},
			want: "ca9af402-03aa-4c77-8a81-b4960b5126fd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractPrometheusRuleID(tt.rule)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractPrometheusRuleName(t *testing.T) {
	rule := &PrometheusRule{
		Metadata: PrometheusRuleMetadata{
			Name: "my-prom-rule",
		},
	}
	assert.Equal(t, "my-prom-rule", ExtractPrometheusRuleName(rule))

	assert.Equal(t, "", ExtractPrometheusRuleName(&PrometheusRule{}))
}

func TestConvertToCheckRule(t *testing.T) {
	tests := []struct {
		name          string
		rule          *PrometheusAlertingRule
		groupInterval string
		ruleID        string
		wantName      string
		wantExpr      string
		wantID        *string
		wantFor       string
		wantInterval  string
		wantSummary   string
		wantDesc      string
		wantLabels    bool
	}{
		{
			name: "full rule with all fields",
			rule: &PrometheusAlertingRule{
				Alert: "High Error Rate",
				Expr:  "rate(errors[5m]) > 0.1",
				For:   "2m",
				Labels: map[string]string{
					"severity": "critical",
				},
				Annotations: map[string]string{
					"summary":     "Error rate is high",
					"description": "Error rate exceeded threshold",
				},
			},
			groupInterval: "1m",
			ruleID:        "test-id",
			wantName:      "High Error Rate",
			wantExpr:      "rate(errors[5m]) > 0.1",
			wantID:        strPtr("test-id"),
			wantFor:       "2m",
			wantInterval:  "1m",
			wantSummary:   "Error rate is high",
			wantDesc:      "Error rate exceeded threshold",
			wantLabels:    true,
		},
		{
			name: "minimal rule",
			rule: &PrometheusAlertingRule{
				Alert: "SimpleAlert",
				Expr:  "up == 0",
			},
			groupInterval: "",
			ruleID:        "",
			wantName:      "SimpleAlert",
			wantExpr:      "up == 0",
			wantID:        nil,
			wantFor:       "",
			wantInterval:  "",
			wantSummary:   "",
			wantDesc:      "",
			wantLabels:    false,
		},
		{
			name: "annotations without summary or description",
			rule: &PrometheusAlertingRule{
				Alert: "CustomAnnotations",
				Expr:  "up == 0",
				Annotations: map[string]string{
					"runbook_url": "https://example.com/runbook",
				},
			},
			groupInterval: "",
			ruleID:        "",
			wantName:      "CustomAnnotations",
			wantExpr:      "up == 0",
			wantID:        nil,
			wantSummary:   "",
			wantDesc:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToCheckRule(tt.rule, tt.groupInterval, tt.ruleID)

			assert.Equal(t, tt.wantName, result.Name)
			assert.Equal(t, tt.wantExpr, result.Expression)

			if tt.wantID != nil {
				require.NotNil(t, result.Id)
				assert.Equal(t, *tt.wantID, *result.Id)
			} else {
				assert.Nil(t, result.Id)
			}

			if tt.wantFor != "" {
				require.NotNil(t, result.For)
				assert.Equal(t, tt.wantFor, string(*result.For))
			} else {
				assert.Nil(t, result.For)
			}

			if tt.wantInterval != "" {
				require.NotNil(t, result.Interval)
				assert.Equal(t, tt.wantInterval, string(*result.Interval))
			} else {
				assert.Nil(t, result.Interval)
			}

			if tt.wantSummary != "" {
				require.NotNil(t, result.Summary)
				assert.Equal(t, tt.wantSummary, *result.Summary)
			} else {
				assert.Nil(t, result.Summary)
			}

			if tt.wantDesc != "" {
				require.NotNil(t, result.Description)
				assert.Equal(t, tt.wantDesc, *result.Description)
			} else {
				assert.Nil(t, result.Description)
			}

			if tt.wantLabels {
				require.NotNil(t, result.Labels)
				assert.Equal(t, "critical", (*result.Labels)["severity"])
			}
		})
	}
}

func TestConvertToCheckRule_AnnotationsPreserved(t *testing.T) {
	rule := &PrometheusAlertingRule{
		Alert: "Test",
		Expr:  "up == 0",
		Annotations: map[string]string{
			"summary":     "Sum",
			"description": "Desc",
			"runbook_url": "https://example.com",
		},
	}

	result := ConvertToCheckRule(rule, "", "")

	require.NotNil(t, result.Annotations)
	assert.Equal(t, "https://example.com", (*result.Annotations)["runbook_url"])
	assert.Equal(t, "Sum", (*result.Annotations)["summary"])
}
