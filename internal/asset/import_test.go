package asset

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestActionForExisting(t *testing.T) {
	existing := &struct{ Name string }{Name: "existing"}
	deletedAt := time.Date(2026, 10, 13, 12, 50, 57, 0, time.UTC)

	tests := []struct {
		name       string
		deletedAt  *time.Time
		wantAction ImportAction
		wantBefore any
	}{
		{name: "active", wantAction: ActionUpdated, wantBefore: existing},
		{name: "deleted", deletedAt: &deletedAt, wantAction: ActionRestored},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action, before := actionForExisting(existing, tt.deletedAt)
			assert.Equal(t, tt.wantAction, action)
			assert.Equal(t, tt.wantBefore, before)
		})
	}
}
