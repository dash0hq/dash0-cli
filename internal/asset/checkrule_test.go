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
