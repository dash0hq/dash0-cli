package asset

import (
	"testing"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
)

func TestSyntheticCheckDeletedAt(t *testing.T) {
	deletedAt := time.Date(2026, 10, 13, 12, 50, 57, 0, time.UTC)

	tests := []struct {
		name  string
		check *dash0api.SyntheticCheckDefinition
		want  *time.Time
	}{
		{name: "nil check"},
		{name: "no annotations", check: &dash0api.SyntheticCheckDefinition{}},
		{
			name: "deleted",
			check: &dash0api.SyntheticCheckDefinition{Metadata: dash0api.SyntheticCheckMetadata{
				Annotations: &dash0api.SyntheticCheckAnnotations{Dash0ComdeletedAt: &deletedAt},
			}},
			want: &deletedAt,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SyntheticCheckDeletedAt(tt.check))
		})
	}
}

func TestClearSyntheticCheckDeletedAt(t *testing.T) {
	deletedAt := time.Date(2026, 10, 13, 12, 50, 57, 0, time.UTC)

	tests := []struct {
		name  string
		check *dash0api.SyntheticCheckDefinition
		want  *dash0api.SyntheticCheckDefinition
	}{
		{name: "nil check"},
		{name: "no annotations", check: &dash0api.SyntheticCheckDefinition{}, want: &dash0api.SyntheticCheckDefinition{}},
		{
			name: "deleted",
			check: &dash0api.SyntheticCheckDefinition{Metadata: dash0api.SyntheticCheckMetadata{
				Annotations: &dash0api.SyntheticCheckAnnotations{Dash0ComdeletedAt: &deletedAt},
			}},
			want: &dash0api.SyntheticCheckDefinition{Metadata: dash0api.SyntheticCheckMetadata{
				Annotations: &dash0api.SyntheticCheckAnnotations{},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ClearSyntheticCheckDeletedAt(tt.check)
			assert.Equal(t, tt.want, tt.check)
		})
	}
}
