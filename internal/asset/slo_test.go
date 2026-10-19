package asset

import (
	"testing"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
)

func TestSLODeletedAt(t *testing.T) {
	deletedAt := time.Date(2026, 10, 13, 12, 50, 57, 0, time.UTC)

	tests := []struct {
		name string
		slo  *dash0api.SloDefinition
		want *time.Time
	}{
		{name: "nil SLO"},
		{name: "no annotations", slo: &dash0api.SloDefinition{}},
		{
			name: "deleted",
			slo: &dash0api.SloDefinition{Metadata: dash0api.SloMetadata{
				Annotations: &dash0api.SloAnnotations{Dash0ComdeletedAt: &deletedAt},
			}},
			want: &deletedAt,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SLODeletedAt(tt.slo))
		})
	}
}
