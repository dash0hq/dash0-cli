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

// The tests below exercise the semantic comparison engine
// (dash0yaml.Equivalent/Normalize) wired into HasDifference/PrintDiff,
// covering behaviors a plain textual YAML diff would get wrong: slice
// reordering, duration-string formatting, check-rule default annotation
// values, and the bare "sharing" annotation key check rules use (not
// "dash0.com/sharing").

// TestPrintDiff_View_ReorderedFilterCriteriaIsNotAChange proves the
// semantic engine's own order-independent slice comparison is doing the
// work here -- unlike TestPrintDiff_View_PermissionOrderIsNotAChange, which
// relies on marshalForDiff's own pre-existing SortViewPermissions call, a
// view's implicit filter criteria (a real JSON array, spec.filter) has no
// such pre-sort.
func TestPrintDiff_View_ReorderedFilterCriteriaIsNotAChange(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	before := &dash0api.ViewDefinition{
		Kind:     "View",
		Metadata: dash0api.ViewMetadata{Name: "Error Logs"},
		Spec: dash0api.ViewSpec{
			Display: dash0api.ViewDisplay{},
			Filter: &dash0api.FilterCriteria{
				{Key: "service.name", Operator: "is_set"},
				{Key: "severity", Operator: "is_set"},
			},
		},
	}
	after := &dash0api.ViewDefinition{
		Kind:     "View",
		Metadata: dash0api.ViewMetadata{Name: "Error Logs"},
		Spec: dash0api.ViewSpec{
			Display: dash0api.ViewDisplay{},
			Filter: &dash0api.FilterCriteria{
				{Key: "severity", Operator: "is_set"},
				{Key: "service.name", Operator: "is_set"},
			},
		},
	}

	different, err := HasDifference(before, after)
	require.NoError(t, err)
	assert.False(t, different, "a real JSON array's element order must not be reported as drift")
}

// TestPrintDiff_View_ReorderedFilterCriteriaRealChangeDetected proves the
// order-independent comparison above doesn't also hide a genuine change.
func TestPrintDiff_View_ReorderedFilterCriteriaRealChangeDetected(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	before := &dash0api.ViewDefinition{
		Kind:     "View",
		Metadata: dash0api.ViewMetadata{Name: "Error Logs"},
		Spec: dash0api.ViewSpec{
			Display: dash0api.ViewDisplay{},
			Filter: &dash0api.FilterCriteria{
				{Key: "service.name", Operator: "is_set"},
				{Key: "severity", Operator: "is_set"},
			},
		},
	}
	after := &dash0api.ViewDefinition{
		Kind:     "View",
		Metadata: dash0api.ViewMetadata{Name: "Error Logs"},
		Spec: dash0api.ViewSpec{
			Display: dash0api.ViewDisplay{},
			Filter: &dash0api.FilterCriteria{
				{Key: "severity", Operator: "is_not_set"},
				{Key: "service.name", Operator: "is_set"},
			},
		},
	}

	different, err := HasDifference(before, after)
	require.NoError(t, err)
	assert.True(t, different, "a genuine element-level change must still be detected regardless of position")
}

func TestPrintDiff_CheckRule_DurationFormatEquivalence_NoChanges(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	for1 := "2m"
	for2 := "2m0s"

	before := &dash0api.PrometheusAlertRule{
		Name:       "High Error Rate",
		Expression: "rate(errors[5m]) > 0.1",
		For:        &for1,
	}
	after := &dash0api.PrometheusAlertRule{
		Name:       "High Error Rate",
		Expression: "rate(errors[5m]) > 0.1",
		For:        &for2,
	}

	var buf bytes.Buffer
	err := PrintDiff(&buf, "Check rule", "High Error Rate", before, after)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `Check rule "High Error Rate": no changes`)
}

func TestPrintDiff_CheckRule_DurationFormatRealChange_Detected(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	for1 := "2m"
	for2 := "3m"

	before := &dash0api.PrometheusAlertRule{
		Name:       "High Error Rate",
		Expression: "rate(errors[5m]) > 0.1",
		For:        &for1,
	}
	after := &dash0api.PrometheusAlertRule{
		Name:       "High Error Rate",
		Expression: "rate(errors[5m]) > 0.1",
		For:        &for2,
	}

	different, err := HasDifference(before, after)
	require.NoError(t, err)
	assert.True(t, different, "a genuinely different duration must not be swallowed by the format-tolerant comparison")
}

// TestPrintDiff_CheckRule_DefaultAnnotationValues_NoChanges pins that
// setting a check rule annotation to the value the Dash0 JSON -> Prometheus
// YAML conversion omits by default (dash0-threshold-critical/degraded: "0",
// dash0-enabled: "true") is treated as equivalent to omitting it.
func TestPrintDiff_CheckRule_DefaultAnnotationValues_NoChanges(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	before := &dash0api.PrometheusAlertRule{
		Name:       "High Error Rate",
		Expression: "rate(errors[5m]) > 0.1",
		Annotations: &dash0api.PrometheusAlertRule_Annotations{
			AdditionalProperties: map[string]string{
				"summary":                  "Test summary",
				"dash0-threshold-critical": "0",
				"dash0-enabled":            "true",
			},
		},
	}
	after := &dash0api.PrometheusAlertRule{
		Name:       "High Error Rate",
		Expression: "rate(errors[5m]) > 0.1",
		Annotations: &dash0api.PrometheusAlertRule_Annotations{
			AdditionalProperties: map[string]string{
				"summary": "Test summary",
			},
		},
	}

	var buf bytes.Buffer
	err := PrintDiff(&buf, "Check rule", "High Error Rate", before, after)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `Check rule "High Error Rate": no changes`)
}

// TestPrintDiff_CheckRule_SharingChangeDetected pins that check rules use
// the bare "sharing" annotation key (dash0api.PrometheusAlertRule_Annotations.Sharing,
// JSON tag "sharing"), not "dash0.com/sharing" -- and that a real change to
// it is detected, not silently stripped as a non-preserved annotation.
func TestPrintDiff_CheckRule_SharingChangeDetected(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	before := &dash0api.PrometheusAlertRule{
		Name:       "High Error Rate",
		Expression: "rate(errors[5m]) > 0.1",
		Annotations: &dash0api.PrometheusAlertRule_Annotations{
			Sharing: strPtr("team:team_01abc"),
		},
	}
	after := &dash0api.PrometheusAlertRule{
		Name:       "High Error Rate",
		Expression: "rate(errors[5m]) > 0.1",
		Annotations: &dash0api.PrometheusAlertRule_Annotations{
			Sharing: strPtr("team:team_02xyz"),
		},
	}

	different, err := HasDifference(before, after)
	require.NoError(t, err)
	assert.True(t, different, "a real sharing change on a check rule must be detected, not treated as a non-preserved annotation")
}

// TestPrintDiff_CheckRule_CustomLabelChangeDetected pins the fix for the gap
// found while designing WithAnnotationsRoot: CheckRule's flat "labels" map
// can carry genuine user-set custom labels (unlike the provenance/id-only
// metadata.labels on every other kind), and a real change to one must not
// be silently ignored.
func TestPrintDiff_CheckRule_CustomLabelChangeDetected(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	before := &dash0api.PrometheusAlertRule{
		Name:       "High Error Rate",
		Expression: "rate(errors[5m]) > 0.1",
		Labels:     &map[string]string{"team": "platform"},
	}
	after := &dash0api.PrometheusAlertRule{
		Name:       "High Error Rate",
		Expression: "rate(errors[5m]) > 0.1",
		Labels:     &map[string]string{"team": "backend"},
	}

	different, err := HasDifference(before, after)
	require.NoError(t, err)
	assert.True(t, different, "a real custom label change on a check rule must be detected")
}

// TestPrintDiff_NotificationChannel_RoutingAssetsIgnored pins that
// spec.routing.assets (API-managed, populated as a back-reference when a
// check rule or synthetic check binds to the channel) never counts as
// drift, matching apply's existing RoutingAssetsWarning behavior.
func TestPrintDiff_NotificationChannel_RoutingAssetsIgnored(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	before := &dash0api.NotificationChannelDefinition{
		Metadata: dash0api.NotificationChannelMetadata{Name: "Slack Alerts"},
		Spec: dash0api.NotificationChannelSpec{
			Routing: &dash0api.NotificationChannelRouting{
				Assets: []dash0api.NotificationChannelRoutingAsset{},
			},
		},
	}
	after := &dash0api.NotificationChannelDefinition{
		Metadata: dash0api.NotificationChannelMetadata{Name: "Slack Alerts"},
		Spec: dash0api.NotificationChannelSpec{
			Routing: &dash0api.NotificationChannelRouting{
				Assets: []dash0api.NotificationChannelRoutingAsset{
					{Kind: dash0api.CheckRule, Id: "id-1", Name: "some check rule"},
				},
			},
		},
	}

	different, err := HasDifference(before, after)
	require.NoError(t, err)
	assert.False(t, different, "spec.routing.assets is API-managed and must never be reported as drift")
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

