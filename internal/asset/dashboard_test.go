package asset

import (
	"testing"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestClearDashboardDeletedAt(t *testing.T) {
	deletedAt := time.Date(2026, 10, 13, 12, 50, 57, 0, time.UTC)
	folderPath := "team/overview"

	tests := []struct {
		name      string
		dashboard *dash0api.DashboardDefinition
		want      *dash0api.DashboardDefinition
	}{
		{name: "nil dashboard"},
		{name: "no annotations", dashboard: &dash0api.DashboardDefinition{}, want: &dash0api.DashboardDefinition{}},
		{
			name: "deleted",
			dashboard: &dash0api.DashboardDefinition{Metadata: dash0api.DashboardMetadata{
				Annotations: &dash0api.DashboardAnnotations{Dash0ComdeletedAt: &deletedAt, Dash0ComfolderPath: &folderPath},
			}},
			want: &dash0api.DashboardDefinition{Metadata: dash0api.DashboardMetadata{
				Annotations: &dash0api.DashboardAnnotations{Dash0ComfolderPath: &folderPath},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ClearDashboardDeletedAt(tt.dashboard)
			assert.Equal(t, tt.want, tt.dashboard)
		})
	}
}

func TestDashboardIdentity_PersesTitleDoesNotMoveInferredOrigin(t *testing.T) {
	perses := func(title string) []byte {
		return []byte("apiVersion: perses.dev/v1alpha1\nkind: PersesDashboard\nmetadata:\n  name: checkout\nspec:\n  display:\n    name: " + title + "\n")
	}
	before, beforeName, err := ParseDashboard(perses("Checkout"))
	require.NoError(t, err)
	after, afterName, err := ParseDashboard(perses("Checkout overview"))
	require.NoError(t, err)

	assert.Equal(t, "checkout", beforeName)
	assert.Equal(t, DashboardIdentity(before, beforeName, nil).Inferred, DashboardIdentity(after, afterName, nil).Inferred)
	assert.Equal(t, "Checkout overview", dash0api.GetDashboardName(after), "the title still reaches the API")
}
