package asset

import (
	"testing"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
)

func TestViewDeletedAt(t *testing.T) {
	deletedAt := time.Date(2026, 10, 13, 12, 50, 57, 0, time.UTC)

	tests := []struct {
		name string
		view *dash0api.ViewDefinition
		want *time.Time
	}{
		{name: "nil view"},
		{name: "no annotations", view: &dash0api.ViewDefinition{}},
		{
			name: "deleted",
			view: &dash0api.ViewDefinition{Metadata: dash0api.ViewMetadata{
				Annotations: &dash0api.ViewAnnotations{Dash0ComdeletedAt: &deletedAt},
			}},
			want: &deletedAt,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ViewDeletedAt(tt.view))
		})
	}
}
