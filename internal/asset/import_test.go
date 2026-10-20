package asset

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestActionForExisting(t *testing.T) {
	deletedAt := time.Date(2026, 10, 13, 12, 50, 57, 0, time.UTC)

	tests := []struct {
		name      string
		deletedAt *time.Time
		want      ImportAction
	}{
		{name: "active", want: ActionUpdated},
		{name: "deleted", deletedAt: &deletedAt, want: ActionRestored},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, actionForExisting(tt.deletedAt))
		})
	}
}

func TestRequireNotDeleted(t *testing.T) {
	deletedAt := time.Date(2026, 10, 13, 12, 50, 57, 0, time.UTC)

	tests := []struct {
		name      string
		deletedAt *time.Time
		wantErr   string
	}{
		{name: "active"},
		{name: "deleted", deletedAt: &deletedAt, wantErr: `dashboard "Overview" is still deleted after the update`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RequireNotDeleted("dashboard", "Overview", tt.deletedAt)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}
