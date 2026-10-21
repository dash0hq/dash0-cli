package asset

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{name: "deleted", deletedAt: &deletedAt, wantErr: `Dashboard "Overview" was saved, but the API still shows it as deleted`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RequireNotDeleted("Dashboard", "Overview", "dashboard-id", tt.deletedAt)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.wantErr)
			var stillDeleted *StillDeletedError
			require.True(t, errors.As(err, &stillDeleted), "got %T, want *StillDeletedError", err)
			assert.Equal(t, "dashboard-id", stillDeleted.ID)
		})
	}
}
