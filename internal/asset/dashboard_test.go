package asset

import (
	"testing"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
)

func TestDashboardDeletedAt(t *testing.T) {
	deletedAt := time.Date(2026, 10, 13, 12, 50, 57, 0, time.UTC)

	tests := []struct {
		name      string
		dashboard *dash0api.DashboardDefinition
		want      *time.Time
	}{
		{name: "nil dashboard"},
		{name: "no annotations", dashboard: &dash0api.DashboardDefinition{}},
		{
			name: "deleted",
			dashboard: &dash0api.DashboardDefinition{Metadata: dash0api.DashboardMetadata{
				Annotations: &dash0api.DashboardAnnotations{Dash0ComdeletedAt: &deletedAt},
			}},
			want: &deletedAt,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DashboardDeletedAt(tt.dashboard))
		})
	}
}
