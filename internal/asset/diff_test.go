package asset

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
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
// (dash0yaml.Equivalent/Normalize) wired into UnifiedDiff/PrintDiff,
// covering behaviors a plain textual YAML diff would get wrong: slice
// reordering, duration-string formatting, check-rule default annotation
// values, and the bare "sharing" annotation key check rules use (not
// "dash0.com/sharing").

// TestPrintDiff_SyntheticCheck_ReorderedLocationsIsNotAChange proves the
// semantic engine's own order-insensitive comparison of set-like slices is
// doing the work: unlike spec.permissions, which marshalForDiff pre-sorts,
// spec.schedule.locations gets no pre-sort here.
func TestPrintDiff_SyntheticCheck_ReorderedLocationsIsNotAChange(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	check := func(locations ...string) *dash0api.SyntheticCheckDefinition {
		c := &dash0api.SyntheticCheckDefinition{Kind: "Dash0SyntheticCheck"}
		c.Metadata.Name = "API health"
		c.Spec.Schedule.Locations = locations
		return c
	}

	text, err := UnifiedDiff("", check("de-frankfurt", "us-oregon"), check("us-oregon", "de-frankfurt"))
	require.NoError(t, err)
	assert.Empty(t, text, "reordering a set-like slice must not be reported as drift")

	text, err = UnifiedDiff("", check("de-frankfurt", "us-oregon"), check("de-frankfurt"))
	require.NoError(t, err)
	assert.NotEmpty(t, text, "removing a location must still be reported")
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

	text, err := UnifiedDiff("", before, after)
	require.NoError(t, err)
	assert.NotEmpty(t, text, "a genuine element-level change must still be detected regardless of position")
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

	text, err := UnifiedDiff("", before, after)
	require.NoError(t, err)
	assert.NotEmpty(t, text, "a genuinely different duration must not be swallowed by the format-tolerant comparison")
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

	text, err := UnifiedDiff("", before, after)
	require.NoError(t, err)
	assert.NotEmpty(t, text, "a real sharing change on a check rule must be detected, not treated as a non-preserved annotation")
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

	text, err := UnifiedDiff("", before, after)
	require.NoError(t, err)
	assert.NotEmpty(t, text, "a real custom label change on a check rule must be detected")
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

	text, err := UnifiedDiff("", before, after)
	require.NoError(t, err)
	assert.Empty(t, text, "spec.routing.assets is API-managed and must never be reported as drift")
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

// TestPrintDiff_SLO_ServerBumpedMetadataIsNotAChange pins the normalization that
// makes `apply`/`update` of an unchanged SLO report "no changes" rather than a
// spurious diff. The server bumps dash0.com/version and dash0.com/updated-at on
// every PUT, even for an identical body, so both sides of the diff must be run
// through StripSLOServerFields. Before the fix, *dash0api.SloDefinition fell
// through marshalForDiff's default branch and neither side was stripped.
func TestPrintDiff_SLO_ServerBumpedMetadataIsNotAChange(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	// `before` as returned by GetSLO, `after` as returned by the PUT: same
	// user-authored document, server-managed metadata moved on.
	beforeJSON := `{
		"apiVersion": "openslo.com/v1",
		"kind": "SLO",
		"metadata": {
			"name": "checkout-availability",
			"labels": {
				"dash0.com/id": "slo_01k5vpx97efdnrkqan15b41k84",
				"dash0.com/version": "1",
				"dash0.com/dataset": "default",
				"dash0.com/origin": "tf_6f2a1c8e-3b47-4d90-a1e5-9c73b0d84f21"
			},
			"annotations": {
				"dash0.com/created-at": "2026-01-15T10:00:00Z",
				"dash0.com/updated-at": "2026-01-15T10:00:00Z"
			}
		},
		"spec": {"service": "checkout", "budgetingMethod": "Occurrences"}
	}`
	afterJSON := `{
		"apiVersion": "openslo.com/v1",
		"kind": "SLO",
		"metadata": {
			"name": "checkout-availability",
			"labels": {
				"dash0.com/id": "slo_01k5vpx97efdnrkqan15b41k84",
				"dash0.com/version": "2",
				"dash0.com/dataset": "default",
				"dash0.com/origin": "tf_6f2a1c8e-3b47-4d90-a1e5-9c73b0d84f21"
			},
			"annotations": {
				"dash0.com/created-at": "2026-01-15T10:00:00Z",
				"dash0.com/updated-at": "2026-01-15T12:00:00Z"
			}
		},
		"spec": {"service": "checkout", "budgetingMethod": "Occurrences"}
	}`

	var before, after dash0api.SloDefinition
	require.NoError(t, json.Unmarshal([]byte(beforeJSON), &before))
	require.NoError(t, json.Unmarshal([]byte(afterJSON), &after))

	var buf bytes.Buffer
	require.NoError(t, PrintDiff(&buf, "SLO", "checkout-availability", &before, &after))

	assert.Contains(t, buf.String(), `SLO "checkout-availability": no changes`)
	assert.NotContains(t, buf.String(), "updated-at")
	assert.NotContains(t, buf.String(), "dash0.com/version")
}

// A hand-authored SLO document can only pin dash0.com/origin — ids are
// server-assigned — so on `slos update -f <origin-only file>` the `before`
// fetched from the API carries an id that the `after` never will. The id is
// server-managed metadata, so its absence from the file must not render as a
// deletion. Regression test for a phantom "-dash0.com/id / +labels: {}" diff.
func TestPrintDiff_SLO_ServerAssignedIDIsNotAChange(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	// before: fetched by origin, so the server filled in id/version/dataset.
	beforeJSON := `{
		"apiVersion": "openslo.com/v1",
		"kind": "SLO",
		"metadata": {
			"name": "checkout-availability",
			"labels": {
				"dash0.com/id": "slo_01kyn68gwrf22afc4vc9xjdtg5",
				"dash0.com/version": "3",
				"dash0.com/dataset": "default",
				"dash0.com/origin": "manual-test-checkout-api"
			},
			"annotations": {
				"dash0.com/display-name": "Checkout API availability",
				"dash0.com/created-at": "2026-01-15T10:00:00Z"
			}
		},
		"spec": {"service": "checkout", "budgetingMethod": "Occurrences"}
	}`
	// after: the user's file — origin only, no id, no server metadata. It does
	// carry the client-settable display-name annotation, as a real authored
	// document does; that keeps the comparison about the id alone.
	afterJSON := `{
		"apiVersion": "openslo.com/v1",
		"kind": "SLO",
		"metadata": {
			"name": "checkout-availability",
			"labels": {"dash0.com/origin": "manual-test-checkout-api"},
			"annotations": {"dash0.com/display-name": "Checkout API availability"}
		},
		"spec": {"service": "checkout", "budgetingMethod": "Occurrences"}
	}`

	var before, after dash0api.SloDefinition
	require.NoError(t, json.Unmarshal([]byte(beforeJSON), &before))
	require.NoError(t, json.Unmarshal([]byte(afterJSON), &after))

	var buf bytes.Buffer
	require.NoError(t, PrintDiff(&buf, "SLO", "checkout-availability", &before, &after))

	assert.Contains(t, buf.String(), `SLO "checkout-availability": no changes`)
	assert.NotContains(t, buf.String(), "dash0.com/id", "the server-assigned id must not appear as a change")
	assert.NotContains(t, buf.String(), "labels: {}", "stripping every label must not render an empty-map diff")
}

// TestPrintDiff_SLO_RealSpecChangeStillDiffs is the counterpart guard: stripping
// server-managed metadata must not swallow a genuine change to the document.
func TestPrintDiff_SLO_RealSpecChangeStillDiffs(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	const tmpl = `{
		"apiVersion": "openslo.com/v1",
		"kind": "SLO",
		"metadata": {
			"name": "checkout-availability",
			"labels": {"dash0.com/version": "%s"},
			"annotations": {"dash0.com/updated-at": "2026-01-15T1%s:00:00Z"}
		},
		"spec": {"service": "%s", "budgetingMethod": "Occurrences"}
	}`

	var before, after dash0api.SloDefinition
	require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(tmpl, "1", "0", "checkout")), &before))
	require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(tmpl, "2", "2", "checkout-v2")), &after))

	var buf bytes.Buffer
	require.NoError(t, PrintDiff(&buf, "SLO", "checkout-availability", &before, &after))

	assert.NotContains(t, buf.String(), "no changes")
	assert.Contains(t, buf.String(), "checkout-v2")
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

// storedCheckRuleJSON is a check rule as the API returns it for a file that set
// only name and expression: the API adds the source label, enabled, and the
// default interval.
const storedCheckRuleJSON = `{
	"name": "rule",
	"expression": "up == 0",
	"enabled": true,
	"interval": "1m0s",
	"metadata": {"labels": {"dash0.com/source": "api"}}
}`

func TestPrintDiff_CheckRule_ServerAddedFieldsAreNotAChange(t *testing.T) {
	dashcolor.NoColor = true
	defer func() { dashcolor.NoColor = false }()

	var stored dash0api.PrometheusAlertRule
	require.NoError(t, json.Unmarshal([]byte(storedCheckRuleJSON), &stored))
	local := &dash0api.PrometheusAlertRule{Name: "rule", Expression: "up == 0"}

	var buf bytes.Buffer
	require.NoError(t, PrintDiff(&buf, "Check rule", "rule", &stored, local))
	assert.Contains(t, buf.String(), `Check rule "rule": no changes`)
}

func TestUnifiedDiff_CheckRule_ShowsOnlyTheRealChange(t *testing.T) {
	var stored dash0api.PrometheusAlertRule
	require.NoError(t, json.Unmarshal([]byte(storedCheckRuleJSON), &stored))
	interval := dash0api.Duration("1m")
	local := &dash0api.PrometheusAlertRule{Name: "rule", Expression: "up == 1", Interval: &interval}

	text, err := UnifiedDiff("Check rule", &stored, local)
	require.NoError(t, err)
	assert.Contains(t, text, "-expression: up == 0")
	assert.Contains(t, text, "+expression: up == 1")
	assert.NotContains(t, text, "dash0.com/source")
	assert.NotContains(t, text, "1m0s")
}

func TestUnifiedDiff_UserAnnotationChangesAreChanges(t *testing.T) {
	decode := func(t *testing.T, doc string, into any) any {
		t.Helper()
		require.NoError(t, dash0yaml.Unmarshal([]byte(doc), into))
		return into
	}
	cases := map[string]struct {
		before, after string
		newAsset      func() any
		wantLine      string
	}{
		"synthetic check folder path": {
			before:   "kind: Dash0SyntheticCheck\nmetadata:\n  name: n\n  annotations:\n    dash0.com/folder-path: /a\n",
			after:    "kind: Dash0SyntheticCheck\nmetadata:\n  name: n\n  annotations:\n    dash0.com/folder-path: /b\n",
			newAsset: func() any { return &dash0api.SyntheticCheckDefinition{} },
			wantLine: "+    dash0.com/folder-path: /b",
		},
		"SLO display name": {
			before:   "kind: SLO\nmetadata:\n  name: n\n  annotations:\n    dash0.com/display-name: A\n",
			after:    "kind: SLO\nmetadata:\n  name: n\n  annotations:\n    dash0.com/display-name: B\n",
			newAsset: func() any { return &dash0api.SloDefinition{} },
			wantLine: "+    dash0.com/display-name: B",
		},
		"view annotation removed": {
			before:   "kind: View\nmetadata:\n  name: n\n  annotations:\n    dash0.com/folder-path: /a\n",
			after:    "kind: View\nmetadata:\n  name: n\n",
			newAsset: func() any { return &dash0api.ViewDefinition{} },
			wantLine: "-    dash0.com/folder-path: /a",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			text, err := UnifiedDiff("Asset", decode(t, c.before, c.newAsset()), decode(t, c.after, c.newAsset()))

			require.NoError(t, err)
			assert.Contains(t, text, c.wantLine)
		})
	}
}

func TestUnifiedDiff_ServerDefaultAnnotationIsNotAChange(t *testing.T) {
	var stored, local dash0api.SyntheticCheckDefinition
	require.NoError(t, dash0yaml.Unmarshal([]byte("kind: Dash0SyntheticCheck\nmetadata:\n  name: n\n  annotations:\n    dash0.com/enabled: \"true\"\n"), &stored))
	require.NoError(t, dash0yaml.Unmarshal([]byte("kind: Dash0SyntheticCheck\nmetadata:\n  name: n\n"), &local))

	text, err := UnifiedDiff("Synthetic check", &stored, &local)

	require.NoError(t, err)
	assert.Empty(t, text)
}

func TestUnifiedDiff_RecordingRuleAnnotationsAreNotCompared(t *testing.T) {
	var stored, local dash0api.RecordingRule
	require.NoError(t, dash0yaml.Unmarshal([]byte("kind: PrometheusRule\nmetadata:\n  name: n\n"), &stored))
	require.NoError(t, dash0yaml.Unmarshal([]byte("kind: PrometheusRule\nmetadata:\n  name: n\n  annotations:\n    team: qa\n"), &local))

	text, err := UnifiedDiff("Recording rule", &stored, &local)

	require.NoError(t, err)
	assert.Empty(t, text, "the API does not store a recording rule CRD's annotations")
}

// The API enables a check rule whose PUT leaves enabled out, so a file without
// enabled re-enables a disabled rule.
func TestUnifiedDiff_CheckRuleWithoutEnabled(t *testing.T) {
	disabled, enabled := false, true
	cases := map[string]struct {
		stored   *bool
		wantDiff string
	}{
		"re-enables a disabled rule":  {stored: &disabled, wantDiff: "-enabled: false"},
		"keeps an enabled rule as is": {stored: &enabled, wantDiff: ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			stored := &dash0api.PrometheusAlertRule{Name: "r", Expression: "up == 0", Enabled: c.stored}
			local := &dash0api.PrometheusAlertRule{Name: "r", Expression: "up == 0"}

			text, err := UnifiedDiff("Check rule", stored, local)

			require.NoError(t, err)
			if c.wantDiff == "" {
				assert.Empty(t, text)
			} else {
				assert.Contains(t, text, c.wantDiff)
			}
			assert.Nil(t, local.Enabled, "the comparison must not change the document apply sends")
		})
	}
}

func unmarshalForTest[T any](t *testing.T, document string) *T {
	t.Helper()
	var value T
	require.NoError(t, dash0yaml.Unmarshal([]byte(document), &value))
	return &value
}

// The API stores the default frequency, 10m0s, when a write leaves frequency
// out, even over another stored value.
func TestUnifiedDiff_NotificationChannelWithoutFrequency(t *testing.T) {
	const channel = `kind: Dash0NotificationChannel
metadata:
  name: alerts
  labels:
    dash0.com/origin: alerts
spec:
  type: webhook
  config:
    url: https://example.com/hook
    method: POST
`
	cases := map[string]struct {
		storedFrequency string
		wantDiff        string
	}{
		"keeps the default":             {storedFrequency: "10m0s", wantDiff: ""},
		"resets a non-default to 10m0s": {storedFrequency: "30m0s", wantDiff: "+  frequency: 10m0s"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			stored := unmarshalForTest[dash0api.NotificationChannelDefinition](t, channel+"  frequency: "+c.storedFrequency+"\n")
			local := unmarshalForTest[dash0api.NotificationChannelDefinition](t, channel)

			text, err := UnifiedDiff("Notification channel", stored, local)

			require.NoError(t, err)
			if c.wantDiff == "" {
				assert.Empty(t, text)
			} else {
				assert.Contains(t, text, "-  frequency: "+c.storedFrequency)
				assert.Contains(t, text, c.wantDiff)
			}
			assert.Nil(t, local.Spec.Frequency, "the comparison must not change the document apply sends")
		})
	}
}

// The API stores the default priority, 2, when a write leaves priority out,
// even over another stored value.
func TestUnifiedDiff_TimeSeriesAggregationWithoutPriority(t *testing.T) {
	const aggregation = `kind: Dash0TimeSeriesAggregation
metadata:
  name: duration
  labels:
    dash0.com/origin: duration
spec:
  enabled: true
  match:
    metricNameMatcher:
      operator: is
      value: http.server.request.duration
  sample:
    interval: 5m
`
	cases := map[string]struct {
		storedPriority string
		wantDiff       string
	}{
		"keeps the default":         {storedPriority: "2", wantDiff: ""},
		"resets a non-default to 2": {storedPriority: "4", wantDiff: "+  priority: 2"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			stored := unmarshalForTest[dash0api.TimeSeriesAggregationDefinition](t, aggregation+"  priority: "+c.storedPriority+"\n")
			local := unmarshalForTest[dash0api.TimeSeriesAggregationDefinition](t, aggregation)

			text, err := UnifiedDiff("Time series aggregation", stored, local)

			require.NoError(t, err)
			if c.wantDiff == "" {
				assert.Empty(t, text)
			} else {
				assert.Contains(t, text, "-  priority: "+c.storedPriority)
				assert.Contains(t, text, c.wantDiff)
			}
			assert.Nil(t, local.Spec.Priority, "the comparison must not change the document apply sends")
		})
	}
}

// The API keeps a grant's stored defaultEnabled when a write leaves it out.
func TestUnifiedDiff_SkillGrantWithoutDefaultEnabled(t *testing.T) {
	skill := func(defaultEnabled string) string {
		document := `kind: Dash0Skill
metadata:
  labels:
    dash0.com/origin: dash0-cli_triage
spec:
  files:
    - relativePath: SKILL.md
      content: "---\nname: triage\n---\nBody.\n"
  permissions:
    - role: basic_member
      actions:
        - custom_skill:read
`
		if defaultEnabled != "" {
			document += "      defaultEnabled: " + defaultEnabled + "\n"
		}
		return document
	}
	cases := map[string]struct {
		stored, local string
		wantDiff      string
	}{
		"omitted over a stored true":  {stored: "true", local: "", wantDiff: ""},
		"omitted over a stored false": {stored: "false", local: "", wantDiff: ""},
		"explicit true over a false":  {stored: "false", local: "true", wantDiff: "+    defaultEnabled: true"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			stored := unmarshalForTest[dash0api.SkillDefinition](t, skill(c.stored))
			local := unmarshalForTest[dash0api.SkillDefinition](t, skill(c.local))

			text, err := UnifiedDiff("Agent0 skill", stored, local)

			require.NoError(t, err)
			if c.wantDiff == "" {
				assert.Empty(t, text)
			} else {
				assert.Contains(t, text, c.wantDiff)
			}
			if c.local == "" {
				assert.Nil(t, (*local.Spec.Permissions)[0].DefaultEnabled, "the comparison must not change the document apply sends")
			}
		})
	}
}
