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

func TestConvertToDashboard_WithAnnotations(t *testing.T) {
	perses := &PersesDashboard{
		APIVersion: "perses.dev/v1alpha1",
		Kind:       "PersesDashboard",
		Metadata: PersesDashboardMetadata{
			Name: "annotated-dashboard",
			Annotations: map[string]string{
				"dash0.com/folder-path": "/test/foo/bar",
				"dash0.com/sharing":     "role:basic_member",
				"dash0.com/source":      "terraform",
			},
		},
		Spec: map[string]interface{}{
			"display": map[string]interface{}{
				"name": "Annotated Dashboard",
			},
		},
	}

	dashboard := ConvertToDashboard(perses)

	require.NotNil(t, dashboard.Metadata.Annotations)
	require.NotNil(t, dashboard.Metadata.Annotations.Dash0ComfolderPath)
	assert.Equal(t, "/test/foo/bar", *dashboard.Metadata.Annotations.Dash0ComfolderPath)
	require.NotNil(t, dashboard.Metadata.Annotations.Dash0Comsharing)
	assert.Equal(t, "role:basic_member", *dashboard.Metadata.Annotations.Dash0Comsharing)
	require.NotNil(t, dashboard.Metadata.Annotations.Dash0Comsource)
	assert.Equal(t, dash0api.DashboardSource("terraform"), *dashboard.Metadata.Annotations.Dash0Comsource)
}

func TestConvertToDashboard_WithFolderPathOnly(t *testing.T) {
	perses := &PersesDashboard{
		APIVersion: "perses.dev/v1alpha1",
		Kind:       "PersesDashboard",
		Metadata: PersesDashboardMetadata{
			Name: "folder-path-dashboard",
			Annotations: map[string]string{
				"dash0.com/folder-path": "/my/folder",
			},
		},
		Spec: map[string]interface{}{
			"display": map[string]interface{}{
				"name": "Folder Path Dashboard",
			},
		},
	}

	dashboard := ConvertToDashboard(perses)

	require.NotNil(t, dashboard.Metadata.Annotations)
	require.NotNil(t, dashboard.Metadata.Annotations.Dash0ComfolderPath)
	assert.Equal(t, "/my/folder", *dashboard.Metadata.Annotations.Dash0ComfolderPath)
	assert.Nil(t, dashboard.Metadata.Annotations.Dash0Comsharing)
	assert.Nil(t, dashboard.Metadata.Annotations.Dash0Comsource)
}

func TestConvertToDashboard_WithNoRelevantAnnotations(t *testing.T) {
	perses := &PersesDashboard{
		APIVersion: "perses.dev/v1alpha1",
		Kind:       "PersesDashboard",
		Metadata: PersesDashboardMetadata{
			Name: "irrelevant-annotations",
			Annotations: map[string]string{
				"some-other-annotation": "value",
			},
		},
		Spec: map[string]interface{}{
			"display": map[string]interface{}{
				"name": "No Relevant Annotations",
			},
		},
	}

	dashboard := ConvertToDashboard(perses)

	assert.Nil(t, dashboard.Metadata.Annotations)
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
