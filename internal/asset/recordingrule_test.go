package asset

import (
	"testing"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
)

func TestRecordingRuleDeletedAt(t *testing.T) {
	deletedAt := time.Date(2026, 10, 13, 12, 50, 57, 0, time.UTC)
	deletedAtFractional := time.Date(2026, 10, 13, 12, 50, 57, 123456000, time.UTC)
	withAnnotations := func(annotations map[string]string) *dash0api.RecordingRule {
		return &dash0api.RecordingRule{Metadata: dash0api.PrometheusRuleMetadata{Annotations: &annotations}}
	}

	tests := []struct {
		name string
		rule *dash0api.RecordingRule
		want *time.Time
	}{
		{name: "nil rule"},
		{name: "no annotations", rule: &dash0api.RecordingRule{}},
		{name: "no deleted-at annotation", rule: withAnnotations(map[string]string{"dash0.com/created-at": "2026-10-01T00:00:00Z"})},
		{name: "empty value", rule: withAnnotations(map[string]string{"dash0.com/deleted-at": ""})},
		{name: "malformed value", rule: withAnnotations(map[string]string{"dash0.com/deleted-at": "yesterday"})},
		{name: "deleted", rule: withAnnotations(map[string]string{"dash0.com/deleted-at": "2026-10-13T12:50:57Z"}), want: &deletedAt},
		{name: "fractional seconds", rule: withAnnotations(map[string]string{"dash0.com/deleted-at": "2026-10-13T12:50:57.123456Z"}), want: &deletedAtFractional},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RecordingRuleDeletedAt(tt.rule)
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			if assert.NotNil(t, got) {
				assert.True(t, tt.want.Equal(*got), "got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestClearRecordingRuleDeletedAt(t *testing.T) {
	withAnnotations := func(annotations map[string]string) *dash0api.RecordingRule {
		return &dash0api.RecordingRule{Metadata: dash0api.PrometheusRuleMetadata{Annotations: &annotations}}
	}

	tests := []struct {
		name string
		rule *dash0api.RecordingRule
		want *dash0api.RecordingRule
	}{
		{name: "nil rule"},
		{name: "no annotations", rule: &dash0api.RecordingRule{}, want: &dash0api.RecordingRule{}},
		{
			name: "deleted",
			rule: withAnnotations(map[string]string{
				"dash0.com/deleted-at": "2026-10-13T12:50:57Z",
				"summary":              "kept",
			}),
			want: withAnnotations(map[string]string{"summary": "kept"}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ClearRecordingRuleDeletedAt(tt.rule)
			assert.Equal(t, tt.want, tt.rule)
		})
	}
}
