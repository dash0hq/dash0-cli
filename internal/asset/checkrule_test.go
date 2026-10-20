package asset

import (
	"testing"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
)

func TestCheckRuleDeletedAt(t *testing.T) {
	deletedAt := time.Date(2026, 10, 13, 12, 50, 57, 0, time.UTC)

	tests := []struct {
		name string
		rule *dash0api.PrometheusAlertRule
		want *time.Time
	}{
		{name: "nil rule"},
		{name: "no metadata", rule: &dash0api.PrometheusAlertRule{}},
		{name: "no annotations", rule: &dash0api.PrometheusAlertRule{Metadata: &dash0api.PrometheusAlertRuleMetadata{}}},
		{
			name: "deleted",
			rule: &dash0api.PrometheusAlertRule{Metadata: &dash0api.PrometheusAlertRuleMetadata{
				Annotations: &dash0api.PrometheusAlertRuleMetadataAnnotations{Dash0ComdeletedAt: &deletedAt},
			}},
			want: &deletedAt,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CheckRuleDeletedAt(tt.rule))
		})
	}
}

func TestClearCheckRuleDeletedAt(t *testing.T) {
	deletedAt := time.Date(2026, 10, 13, 12, 50, 57, 0, time.UTC)
	sourceLabels := &dash0api.PrometheusAlertRuleMetadataLabels{}

	tests := []struct {
		name string
		rule *dash0api.PrometheusAlertRule
		want *dash0api.PrometheusAlertRule
	}{
		{name: "nil rule"},
		{name: "no metadata", rule: &dash0api.PrometheusAlertRule{}, want: &dash0api.PrometheusAlertRule{}},
		{
			name: "deleted",
			rule: &dash0api.PrometheusAlertRule{Metadata: &dash0api.PrometheusAlertRuleMetadata{
				Annotations: &dash0api.PrometheusAlertRuleMetadataAnnotations{Dash0ComdeletedAt: &deletedAt},
			}},
			want: &dash0api.PrometheusAlertRule{},
		},
		{
			name: "deleted with labels",
			rule: &dash0api.PrometheusAlertRule{Metadata: &dash0api.PrometheusAlertRuleMetadata{
				Annotations: &dash0api.PrometheusAlertRuleMetadataAnnotations{Dash0ComdeletedAt: &deletedAt},
				Labels:      sourceLabels,
			}},
			want: &dash0api.PrometheusAlertRule{Metadata: &dash0api.PrometheusAlertRuleMetadata{Labels: sourceLabels}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ClearCheckRuleDeletedAt(tt.rule)
			assert.Equal(t, tt.want, tt.rule)
		})
	}
}

func TestRequireCheckRuleNotDeleted(t *testing.T) {
	deletedAt := time.Date(2026, 10, 13, 12, 50, 57, 0, time.UTC)

	tests := []struct {
		name    string
		rule    *dash0api.PrometheusAlertRule
		wantErr string
	}{
		{name: "active", rule: &dash0api.PrometheusAlertRule{Name: "rule"}},
		{
			name: "deleted",
			rule: &dash0api.PrometheusAlertRule{Name: "rule", Metadata: &dash0api.PrometheusAlertRuleMetadata{
				Annotations: &dash0api.PrometheusAlertRuleMetadataAnnotations{Dash0ComdeletedAt: &deletedAt},
			}},
			wantErr: `check rule "rule" is still deleted after the update`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RequireCheckRuleNotDeleted(tt.rule)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}
