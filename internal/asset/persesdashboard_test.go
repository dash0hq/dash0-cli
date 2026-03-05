package asset

import (
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertToDashboard_WithDisplayName(t *testing.T) {
	perses := &PersesDashboard{
		APIVersion: "perses.dev/v1alpha1",
		Kind:       "PersesDashboard",
		Metadata: PersesDashboardMetadata{
			Name: "my-perses-dashboard",
		},
		Spec: map[string]interface{}{
			"display": map[string]interface{}{
				"name": "My Perses Dashboard",
			},
			"duration": "5m",
			"panels":   map[string]interface{}{},
		},
	}

	dashboard := ConvertToDashboard(perses)

	assert.Equal(t, dash0api.Dashboard, dashboard.Kind)
	assert.Equal(t, "My Perses Dashboard", dashboard.Metadata.Name)
	assert.Equal(t, "My Perses Dashboard", ExtractDashboardDisplayName(dashboard))
	assert.Equal(t, "5m", dashboard.Spec["duration"])
	assert.Nil(t, dashboard.Metadata.Dash0Extensions)
}

func TestConvertToDashboard_WithoutDisplayName(t *testing.T) {
	perses := &PersesDashboard{
		APIVersion: "perses.dev/v1alpha1",
		Kind:       "PersesDashboard",
		Metadata: PersesDashboardMetadata{
			Name: "fallback-name",
		},
		Spec: map[string]interface{}{
			"duration": "5m",
		},
	}

	dashboard := ConvertToDashboard(perses)

	assert.Equal(t, dash0api.Dashboard, dashboard.Kind)
	assert.Equal(t, "fallback-name", dashboard.Metadata.Name)
	assert.Equal(t, "fallback-name", ExtractDashboardDisplayName(dashboard))
}

func TestConvertToDashboard_WithDisplaySectionButNoName(t *testing.T) {
	perses := &PersesDashboard{
		APIVersion: "perses.dev/v1alpha1",
		Kind:       "PersesDashboard",
		Metadata: PersesDashboardMetadata{
			Name: "metadata-name",
		},
		Spec: map[string]interface{}{
			"display": map[string]interface{}{
				"description": "A dashboard without a display name",
			},
		},
	}

	dashboard := ConvertToDashboard(perses)

	assert.Equal(t, "metadata-name", dashboard.Metadata.Name)
	assert.Equal(t, "metadata-name", ExtractDashboardDisplayName(dashboard))
}

func TestConvertToDashboard_V1Alpha2_WithConfigWrapper(t *testing.T) {
	perses := &PersesDashboard{
		APIVersion: "perses.dev/v1alpha2",
		Kind:       "PersesDashboard",
		Metadata: PersesDashboardMetadata{
			Name: "v1alpha2-dashboard",
		},
		Spec: map[string]interface{}{
			"config": map[string]interface{}{
				"display": map[string]interface{}{
					"name": "V1Alpha2 Dashboard",
				},
				"duration": "10m",
				"panels":   map[string]interface{}{},
			},
		},
	}

	dashboard := ConvertToDashboard(perses)

	assert.Equal(t, dash0api.Dashboard, dashboard.Kind)
	assert.Equal(t, "V1Alpha2 Dashboard", dashboard.Metadata.Name)
	assert.Equal(t, "V1Alpha2 Dashboard", ExtractDashboardDisplayName(dashboard))
	assert.Equal(t, "10m", dashboard.Spec["duration"])
	// The config wrapper should be unwrapped
	_, hasConfig := dashboard.Spec["config"]
	assert.False(t, hasConfig, "spec.config should be unwrapped")
}

func TestConvertToDashboard_WithDash0ID(t *testing.T) {
	perses := &PersesDashboard{
		APIVersion: "perses.dev/v1alpha1",
		Kind:       "PersesDashboard",
		Metadata: PersesDashboardMetadata{
			Name: "dashboard-with-id",
			Labels: map[string]string{
				"dash0.com/id": "a1b2c3d4-5678-90ab-cdef-1234567890ab",
			},
		},
		Spec: map[string]interface{}{
			"display": map[string]interface{}{
				"name": "Dashboard With ID",
			},
		},
	}

	dashboard := ConvertToDashboard(perses)

	require.NotNil(t, dashboard.Metadata.Dash0Extensions)
	require.NotNil(t, dashboard.Metadata.Dash0Extensions.Id)
	assert.Equal(t, "a1b2c3d4-5678-90ab-cdef-1234567890ab", *dashboard.Metadata.Dash0Extensions.Id)
}

func TestConvertToDashboard_NilSpec(t *testing.T) {
	perses := &PersesDashboard{
		APIVersion: "perses.dev/v1alpha1",
		Kind:       "PersesDashboard",
		Metadata: PersesDashboardMetadata{
			Name: "nil-spec-dashboard",
		},
		Spec: nil,
	}

	dashboard := ConvertToDashboard(perses)

	assert.Equal(t, dash0api.Dashboard, dashboard.Kind)
	assert.Equal(t, "nil-spec-dashboard", dashboard.Metadata.Name)
}

func TestExtractPersesDashboardName_FromDisplayName(t *testing.T) {
	perses := &PersesDashboard{
		Metadata: PersesDashboardMetadata{
			Name: "metadata-name",
		},
		Spec: map[string]interface{}{
			"display": map[string]interface{}{
				"name": "Display Name",
			},
		},
	}

	assert.Equal(t, "Display Name", ExtractPersesDashboardName(perses))
}

func TestExtractPersesDashboardName_FromMetadataName(t *testing.T) {
	perses := &PersesDashboard{
		Metadata: PersesDashboardMetadata{
			Name: "metadata-name",
		},
		Spec: map[string]interface{}{},
	}

	assert.Equal(t, "metadata-name", ExtractPersesDashboardName(perses))
}

func TestExtractPersesDashboardName_V1Alpha2(t *testing.T) {
	perses := &PersesDashboard{
		Metadata: PersesDashboardMetadata{
			Name: "metadata-name",
		},
		Spec: map[string]interface{}{
			"config": map[string]interface{}{
				"display": map[string]interface{}{
					"name": "V1Alpha2 Name",
				},
			},
		},
	}

	assert.Equal(t, "V1Alpha2 Name", ExtractPersesDashboardName(perses))
}

func TestExtractPersesDashboardID(t *testing.T) {
	perses := &PersesDashboard{
		Metadata: PersesDashboardMetadata{
			Labels: map[string]string{
				"dash0.com/id": "test-id",
			},
		},
	}

	assert.Equal(t, "test-id", ExtractPersesDashboardID(perses))
}

func TestExtractPersesDashboardID_NoLabels(t *testing.T) {
	perses := &PersesDashboard{
		Metadata: PersesDashboardMetadata{},
	}

	assert.Equal(t, "", ExtractPersesDashboardID(perses))
}
