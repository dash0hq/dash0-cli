package asset

import (
	"bytes"
	"encoding/json"
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

// TestPrintDiff_Dashboard_PanelKeyOrderIsNotAChange guards against issue #231.
// spec.panels is a JSON object keyed by panel ID, not an array, so unlike
// spec.permissions it needs no explicit sort: encoding/json (used by
// sigs.k8s.io/yaml under marshalForDiff) already sorts map keys on every
// marshal, regardless of what order the two source documents' panel keys
// were in.
func TestPrintDiff_Dashboard_PanelKeyOrderIsNotAChange(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	beforeJSON := `{
		"kind": "Dashboard",
		"metadata": {"name": "test"},
		"spec": {
			"panels": {
				"zzz-panel": {"kind": "Panel", "spec": {"display": {"name": "Z"}}},
				"aaa-panel": {"kind": "Panel", "spec": {"display": {"name": "A"}}}
			}
		}
	}`
	afterJSON := `{
		"kind": "Dashboard",
		"metadata": {"name": "test"},
		"spec": {
			"panels": {
				"aaa-panel": {"kind": "Panel", "spec": {"display": {"name": "A"}}},
				"zzz-panel": {"kind": "Panel", "spec": {"display": {"name": "Z"}}}
			}
		}
	}`

	var before, after dash0api.DashboardDefinition
	require.NoError(t, json.Unmarshal([]byte(beforeJSON), &before))
	require.NoError(t, json.Unmarshal([]byte(afterJSON), &after))

	var buf bytes.Buffer
	err := PrintDiff(&buf, "Dashboard", "test", &before, &after)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `Dashboard "test": no changes`)
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

// TestPrintDiff_View_PermissionOrderIsNotAChange guards against issue #231:
// the server does not guarantee a stable order for spec.permissions, so a
// before/after pair that only differs in permission order must not render as
// a change.
func TestPrintDiff_View_PermissionOrderIsNotAChange(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	before := &dash0api.ViewDefinition{
		Kind:     "View",
		Metadata: dash0api.ViewMetadata{Name: "Error Logs"},
		Spec: dash0api.ViewSpec{
			Display: dash0api.ViewDisplay{},
			Permissions: &[]dash0api.ViewPermission{
				{Role: strPtr("basic_member")},
				{Role: strPtr("admin")},
			},
		},
	}
	after := &dash0api.ViewDefinition{
		Kind:     "View",
		Metadata: dash0api.ViewMetadata{Name: "Error Logs"},
		Spec: dash0api.ViewSpec{
			Display: dash0api.ViewDisplay{},
			Permissions: &[]dash0api.ViewPermission{
				{Role: strPtr("admin")},
				{Role: strPtr("basic_member")},
			},
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

// TestPrintDiff_TimeSeriesAggregation_VersionBumpIsNotAChange pins the
// marshalForDiff case for this kind. The server increments dash0.com/version
// on every PUT, so without stripping it, a reapply of unchanged content would
// always render a diff and could never report "no changes".
func TestPrintDiff_TimeSeriesAggregation_VersionBumpIsNotAChange(t *testing.T) {
	aggregation := func(version, updatedAt string) *dash0api.TimeSeriesAggregationDefinition {
		origin := "http-server-request-duration"
		v := version
		source := dash0api.Api
		dataset := "default"
		parsed, err := time.Parse(time.RFC3339, updatedAt)
		require.NoError(t, err)
		return &dash0api.TimeSeriesAggregationDefinition{
			Kind: dash0api.Dash0TimeSeriesAggregation,
			Metadata: dash0api.TimeSeriesAggregationMetadata{
				Name: origin,
				Annotations: &dash0api.TimeSeriesAggregationAnnotations{
					Dash0ComupdatedAt: &parsed,
				},
				Labels: &dash0api.TimeSeriesAggregationLabels{
					Dash0Comorigin:  &origin,
					Dash0Comversion: &v,
					Dash0Comsource:  &source,
					Dash0Comdataset: &dataset,
				},
			},
			Spec: dash0api.TimeSeriesAggregationSpec{
				Enabled: true,
				Match: dash0api.TimeSeriesAggregationMetricNameMatch{
					MetricNameMatcher: dash0api.Matcher{Operator: "is", Value: matcherValue(t, "http.server.request.duration")},
				},
				Sample: dash0api.TimeSeriesAggregationSample{Interval: "5m"},
			},
		}
	}

	var buf bytes.Buffer
	err := PrintDiff(&buf,
		"Time series aggregation", "HTTP server request duration",
		aggregation("1", "2026-09-04T10:27:37Z"),
		aggregation("2", "2026-09-04T10:31:02Z"),
	)

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "no changes")
}

// matcherValue builds the Matcher.Value union from a plain string.
func matcherValue(t *testing.T, value string) *dash0api.Matcher_Value {
	t.Helper()
	var v dash0api.Matcher_Value
	require.NoError(t, v.FromAttributeFilterStringValue(value))
	return &v
}
