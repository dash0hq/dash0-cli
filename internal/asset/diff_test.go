package asset

import (
	"bytes"
	"testing"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dashcolor "github.com/dash0hq/dash0-cli/internal/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintDiff_NoChanges(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	dashboard := &dash0api.DashboardDefinition{
		Kind: "Dashboard",
		Metadata: dash0api.DashboardMetadata{
			Name: "test",
		},
		Spec: map[string]interface{}{
			"display": map[string]interface{}{"name": "My Dashboard"},
		},
	}
	same := &dash0api.DashboardDefinition{
		Kind: "Dashboard",
		Metadata: dash0api.DashboardMetadata{
			Name: "test",
		},
		Spec: map[string]interface{}{
			"display": map[string]interface{}{"name": "My Dashboard"},
		},
	}

	var buf bytes.Buffer
	err := PrintDiff(&buf, "Dashboard", "My Dashboard", dashboard, same)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `Dashboard "My Dashboard": no changes`)
}

func TestPrintDiff_WithChanges(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	before := &dash0api.DashboardDefinition{
		Kind: "Dashboard",
		Metadata: dash0api.DashboardMetadata{
			Name: "test",
		},
		Spec: map[string]interface{}{
			"display": map[string]interface{}{"name": "Old Name"},
		},
	}
	after := &dash0api.DashboardDefinition{
		Kind: "Dashboard",
		Metadata: dash0api.DashboardMetadata{
			Name: "test",
		},
		Spec: map[string]interface{}{
			"display": map[string]interface{}{"name": "New Name"},
		},
	}

	var buf bytes.Buffer
	err := PrintDiff(&buf, "Dashboard", "New Name", before, after)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "--- Dashboard (before)")
	assert.Contains(t, output, "+++ Dashboard (after)")
	assert.Contains(t, output, "@@")
	assert.Contains(t, output, "-    name: Old Name")
	assert.Contains(t, output, "+    name: New Name")
}

func TestPrintDiff_StripsServerFields_Dashboard(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	v1 := int64(1)
	v2 := int64(2)
	ds1 := dash0api.Dataset("default")
	ds2 := dash0api.Dataset("other")
	id := "abc"

	before := &dash0api.DashboardDefinition{
		Kind: "Dashboard",
		Metadata: dash0api.DashboardMetadata{
			Name:      "test",
			CreatedAt: &t1,
			UpdatedAt: &t1,
			Version:   &v1,
			Dash0Extensions: &dash0api.DashboardMetadataExtensions{
				Dataset: &ds1,
				Id:      &id,
			},
		},
		Spec: map[string]interface{}{"display": map[string]interface{}{"name": "Dashboard"}},
	}
	after := &dash0api.DashboardDefinition{
		Kind: "Dashboard",
		Metadata: dash0api.DashboardMetadata{
			Name:      "test",
			CreatedAt: &t1,
			UpdatedAt: &t2,
			Version:   &v2,
			Dash0Extensions: &dash0api.DashboardMetadataExtensions{
				Dataset: &ds2,
				Id:      &id,
			},
		},
		Spec: map[string]interface{}{"display": map[string]interface{}{"name": "Dashboard"}},
	}

	var buf bytes.Buffer
	err := PrintDiff(&buf, "Dashboard", "Dashboard", before, after)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `Dashboard "Dashboard": no changes`)
}

func TestPrintDiff_StripsServerFields_CheckRule(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	ds1 := dash0api.Dataset("default")
	ds2 := dash0api.Dataset("other")

	before := &dash0api.PrometheusAlertRule{
		Name:       "High Error Rate",
		Expression: "rate(errors[5m]) > 0.1",
		Dataset:    &ds1,
		Labels:     &map[string]string{"dash0.com/origin": "cli"},
	}
	after := &dash0api.PrometheusAlertRule{
		Name:       "High Error Rate",
		Expression: "rate(errors[5m]) > 0.1",
		Dataset:    &ds2,
		Labels:     &map[string]string{"dash0.com/origin": "api"},
	}

	var buf bytes.Buffer
	err := PrintDiff(&buf, "Check rule", "High Error Rate", before, after)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `Check rule "High Error Rate": no changes`)
}

func TestPrintDiff_StripsServerFields_View(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	v1 := "1"
	v2 := "2"
	id := "view-id"

	before := &dash0api.ViewDefinition{
		Kind: "View",
		Metadata: dash0api.ViewMetadata{
			Name: "Error Logs",
			Labels: &dash0api.ViewLabels{
				Dash0Comid:      &id,
				Dash0Comversion: &v1,
			},
		},
		Spec: dash0api.ViewSpec{
			Display: dash0api.ViewDisplay{},
		},
	}
	after := &dash0api.ViewDefinition{
		Kind: "View",
		Metadata: dash0api.ViewMetadata{
			Name: "Error Logs",
			Labels: &dash0api.ViewLabels{
				Dash0Comid:      &id,
				Dash0Comversion: &v2,
			},
		},
		Spec: dash0api.ViewSpec{
			Display: dash0api.ViewDisplay{},
		},
	}

	var buf bytes.Buffer
	err := PrintDiff(&buf, "View", "Error Logs", before, after)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `View "Error Logs": no changes`)
}

func TestPrintDiff_ColorOutput(t *testing.T) {
	dashcolor.NoColor = false
	t.Setenv("CLICOLOR_FORCE", "1")

	before := &dash0api.DashboardDefinition{
		Kind:     "Dashboard",
		Metadata: dash0api.DashboardMetadata{Name: "test"},
		Spec:     map[string]interface{}{"display": map[string]interface{}{"name": "old"}},
	}
	after := &dash0api.DashboardDefinition{
		Kind:     "Dashboard",
		Metadata: dash0api.DashboardMetadata{Name: "test"},
		Spec:     map[string]interface{}{"display": map[string]interface{}{"name": "new"}},
	}

	var buf bytes.Buffer
	err := PrintDiff(&buf, "Dashboard", "Test", before, after)
	require.NoError(t, err)

	output := buf.String()
	// Color output includes ANSI escape codes
	assert.Contains(t, output, "\033[")
}

