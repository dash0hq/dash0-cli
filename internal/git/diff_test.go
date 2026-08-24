package git

import (
	"testing"

	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiff_WholeFileDeletion(t *testing.T) {
	before := newSnapshot()
	before.Identifiers[IdentifierKey{Kind: "dashboard", Identifier: "id-1"}] = "dashboard.yaml"
	before.Paths["dashboard.yaml"] = true

	after := newSnapshot()

	plan := Diff(before, after)
	require.Len(t, plan.ByIdentifier, 1)
	assert.Equal(t, Deletion{Kind: "dashboard", Identifier: "id-1", Path: "dashboard.yaml"}, plan.ByIdentifier[0])
	assert.Empty(t, plan.AlertsByName)
	assert.Empty(t, plan.NoIdentifier)
	assert.False(t, plan.IsEmpty())
}

func TestDiff_NoChangeWhenIdentifierSurvives(t *testing.T) {
	before := newSnapshot()
	before.Identifiers[IdentifierKey{Kind: "dashboard", Identifier: "id-1"}] = "dashboard.yaml"

	after := newSnapshot()
	after.Identifiers[IdentifierKey{Kind: "dashboard", Identifier: "id-1"}] = "renamed.yaml"

	plan := Diff(before, after)
	assert.True(t, plan.IsEmpty(), "identifier survived under a different path — not a deletion")
}

func TestDiff_PrometheusAlertPartialRemoval(t *testing.T) {
	before := newSnapshot()
	before.Identifiers[IdentifierKey{Kind: "prometheusrule", Identifier: "crd-1"}] = "rules.yaml"
	before.PrometheusAlertsByIdentifier["crd-1"] = []dash0yaml.PrometheusAlertName{
		{GroupName: "g", AlertName: "A"},
		{GroupName: "g", AlertName: "B"},
	}

	after := newSnapshot()
	after.Identifiers[IdentifierKey{Kind: "prometheusrule", Identifier: "crd-1"}] = "rules.yaml"
	after.PrometheusAlertsByIdentifier["crd-1"] = []dash0yaml.PrometheusAlertName{
		{GroupName: "g", AlertName: "A"},
	}

	plan := Diff(before, after)
	assert.Empty(t, plan.ByIdentifier, "CRD survives, so it is not a whole-file deletion")
	require.Len(t, plan.AlertsByName, 1)
	assert.Equal(t, "crd-1", plan.AlertsByName[0].CRDIdentifier)
	assert.Equal(t, "g - B", plan.AlertsByName[0].CheckRuleName())
}

// TestDiff_PrometheusRecordingRoleDroppedWhileCRDSurvives is a regression
// test for a bug where a PrometheusRule CRD losing its last recording rule
// (while an alerting rule kept the CRD's identifier alive) produced no
// deletion signal at all: applyPrometheusRule simply stops calling
// ImportRecordingRule once RecordingOnlyPrometheusRule returns nil, so the
// recording rule created back when the CRD still had a record is left
// stale in Dash0 forever, and --since reported "no deletions" -- a false
// all-clear on a state that no longer matches git. Unlike alerting rules
// (tracked per-alert by name via PrometheusAlertsByIdentifier/AlertsByName),
// recording rules have no per-item identity, so this is a coarse
// presence/absence signal, surfaced as a "recordingrule"-kind entry in
// ByIdentifier rather than a new AlertsByName-shaped slice.
func TestDiff_PrometheusRecordingRoleDroppedWhileCRDSurvives(t *testing.T) {
	before := newSnapshot()
	before.Identifiers[IdentifierKey{Kind: "prometheusrule", Identifier: "crd-1"}] = "rules.yaml"
	before.PrometheusRecordingRoleByIdentifier["crd-1"] = true

	after := newSnapshot()
	after.Identifiers[IdentifierKey{Kind: "prometheusrule", Identifier: "crd-1"}] = "rules.yaml"
	after.PrometheusRecordingRoleByIdentifier["crd-1"] = false

	plan := Diff(before, after)
	require.Len(t, plan.ByIdentifier, 1)
	assert.Equal(t, Deletion{Kind: "recordingrule", Identifier: "crd-1", Path: "rules.yaml"}, plan.ByIdentifier[0])
}

// TestDiff_PrometheusRecordingRoleSurvivesIsNotADeletion pins the negative
// case: a CRD that still has a recording rule in both snapshots must not
// produce any "recordingrule" deletion entry.
func TestDiff_PrometheusRecordingRoleSurvivesIsNotADeletion(t *testing.T) {
	before := newSnapshot()
	before.Identifiers[IdentifierKey{Kind: "prometheusrule", Identifier: "crd-1"}] = "rules.yaml"
	before.PrometheusRecordingRoleByIdentifier["crd-1"] = true

	after := newSnapshot()
	after.Identifiers[IdentifierKey{Kind: "prometheusrule", Identifier: "crd-1"}] = "rules.yaml"
	after.PrometheusRecordingRoleByIdentifier["crd-1"] = true

	plan := Diff(before, after)
	assert.True(t, plan.IsEmpty())
}

// TestDiff_PrometheusWholeCRDDeletionSkipsRecordingRoleCheck is a regression
// test for a bug where a whole-CRD deletion (identifier gone entirely, not
// just its recording role) would double-report the recording rule: once as
// the "prometheusrule"-kind whole-CRD entry (whose dispatch,
// deletePrometheusRuleCRD, already attempts DeleteRecordingRule
// unconditionally) and again as a standalone "recordingrule"-kind entry,
// which would call DeleteRecordingRule a second, redundant time.
func TestDiff_PrometheusWholeCRDDeletionSkipsRecordingRoleCheck(t *testing.T) {
	before := newSnapshot()
	before.Identifiers[IdentifierKey{Kind: "prometheusrule", Identifier: "crd-1"}] = "rules.yaml"
	before.PrometheusRecordingRoleByIdentifier["crd-1"] = true

	after := newSnapshot()

	plan := Diff(before, after)
	require.Len(t, plan.ByIdentifier, 1)
	assert.Equal(t, "prometheusrule", plan.ByIdentifier[0].Kind)
}

func TestDiff_PrometheusWholeCRDDeletionSkipsAlertCheck(t *testing.T) {
	before := newSnapshot()
	before.Identifiers[IdentifierKey{Kind: "prometheusrule", Identifier: "crd-1"}] = "rules.yaml"
	before.PrometheusAlertsByIdentifier["crd-1"] = []dash0yaml.PrometheusAlertName{
		{GroupName: "g", AlertName: "A"},
	}

	after := newSnapshot()

	plan := Diff(before, after)
	require.Len(t, plan.ByIdentifier, 1)
	assert.Equal(t, "prometheusrule", plan.ByIdentifier[0].Kind)
	assert.Empty(t, plan.AlertsByName, "whole-CRD deletion must not also be reported as an alert removal")
}

// TestDiff_PrometheusWholeCRDDeletionCarriesAlertNames is a regression test
// for a bug where a whole-CRD deletion's Deletion entry carried only the
// CRD's own identifier, with no way for the delete dispatch to know the
// CRD's alerts each got their own derived check-rule id (see
// asset.DeriveAlertCheckRuleID) once the CRD has two or more of them --
// dispatch had no choice but to (wrongly) delete by the literal identifier
// alone, missing every alert's real check rule entirely.
func TestDiff_PrometheusWholeCRDDeletionCarriesAlertNames(t *testing.T) {
	before := newSnapshot()
	before.Identifiers[IdentifierKey{Kind: "prometheusrule", Identifier: "crd-1"}] = "rules.yaml"
	before.PrometheusAlertsByIdentifier["crd-1"] = []dash0yaml.PrometheusAlertName{
		{GroupName: "g", AlertName: "A"},
		{GroupName: "g", AlertName: "B"},
	}

	after := newSnapshot()

	plan := Diff(before, after)
	require.Len(t, plan.ByIdentifier, 1)
	assert.Equal(t, []dash0yaml.PrometheusAlertName{
		{GroupName: "g", AlertName: "A"},
		{GroupName: "g", AlertName: "B"},
	}, plan.ByIdentifier[0].PrometheusAlerts)
}

// TestDiff_NonPrometheusRuleDeletionHasNoAlerts pins that PrometheusAlerts
// is only ever populated for kind "prometheusrule", never accidentally
// carried over for an unrelated kind that happens to share PrometheusAlertsByIdentifier's
// map (it doesn't, but the zero-value contract is worth pinning directly).
func TestDiff_NonPrometheusRuleDeletionHasNoAlerts(t *testing.T) {
	before := newSnapshot()
	before.Identifiers[IdentifierKey{Kind: "dashboard", Identifier: "id-1"}] = "dashboard.yaml"

	after := newSnapshot()

	plan := Diff(before, after)
	require.Len(t, plan.ByIdentifier, 1)
	assert.Empty(t, plan.ByIdentifier[0].PrometheusAlerts)
}

func TestDiff_NoIdentifierFileDeleted(t *testing.T) {
	before := newSnapshot()
	before.NoIdentifier["orphan.yaml"] = NoIdentifierDoc{Kind: "dashboard", FilePath: "orphan.yaml"}
	before.Paths["orphan.yaml"] = true

	after := newSnapshot()

	plan := Diff(before, after)
	require.Len(t, plan.NoIdentifier, 1)
	assert.Equal(t, "orphan.yaml", plan.NoIdentifier[0])
}

func TestDiff_NoIdentifierFileSurvivesIsNotADeletion(t *testing.T) {
	before := newSnapshot()
	before.NoIdentifier["orphan.yaml"] = NoIdentifierDoc{Kind: "dashboard", FilePath: "orphan.yaml"}
	before.Paths["orphan.yaml"] = true

	after := newSnapshot()
	// The document itself survives, not just the file path — this is what a
	// real BuildSnapshotFromRef/BuildSnapshotFromDisk pass would produce for
	// an untouched no-identifier document.
	after.NoIdentifier["orphan.yaml"] = NoIdentifierDoc{Kind: "dashboard", FilePath: "orphan.yaml"}
	after.Paths["orphan.yaml"] = true

	plan := Diff(before, after)
	assert.True(t, plan.IsEmpty())
}

func TestDiff_NoIdentifierMultiDocumentUsesFilePathNotDocPath(t *testing.T) {
	// Regression test: a no-identifier document that is the 2nd+ document in
	// a multi-document file is keyed as "file.yaml#1" in one snapshot, but
	// could just as well be keyed "file.yaml" (index 0) in the other if the
	// file's document order shifted — e.g. an earlier document in the same
	// file was removed, re-indexing everything after it. The document itself
	// still survives, just under a different docPath, so this must not be
	// misreported as a deletion; Diff's per-file document *count* comparison
	// (not exact docPath string matching) is what makes that work.
	before := newSnapshot()
	before.NoIdentifier["combined.yaml#1"] = NoIdentifierDoc{Kind: "dashboard", FilePath: "combined.yaml"}
	before.Paths["combined.yaml"] = true

	after := newSnapshot()
	after.NoIdentifier["combined.yaml"] = NoIdentifierDoc{Kind: "dashboard", FilePath: "combined.yaml"}
	after.Paths["combined.yaml"] = true

	plan := Diff(before, after)
	assert.True(t, plan.IsEmpty())
}

// TestDiff_NoIdentifierDocRemovedFromSurvivingMultiDocFile is a regression
// test for a bug where a no-identifier document removed from a file that
// otherwise survives produced no signal at all: Diff's old check only
// compared file existence (after.Paths[doc.FilePath]), so as long as the
// file was still there — regardless of how many of its documents remained —
// nothing was ever reported. design.md's invariant is that a disappearing
// no-identifier document must always fail the run loudly, so a per-file
// count comparison is required, not just existence.
func TestDiff_NoIdentifierDocRemovedFromSurvivingMultiDocFile(t *testing.T) {
	before := newSnapshot()
	// Two no-identifier documents in the same file, plus one identified View
	// that survives untouched.
	before.NoIdentifier["combined.yaml"] = NoIdentifierDoc{Kind: "dashboard", FilePath: "combined.yaml"}
	before.NoIdentifier["combined.yaml#1"] = NoIdentifierDoc{Kind: "dashboard", FilePath: "combined.yaml"}
	before.Identifiers[IdentifierKey{Kind: "view", Identifier: "keep-id"}] = "combined.yaml#2"
	before.Paths["combined.yaml"] = true

	after := newSnapshot()
	// Only one no-identifier document survives; the View survives too.
	after.NoIdentifier["combined.yaml"] = NoIdentifierDoc{Kind: "dashboard", FilePath: "combined.yaml"}
	after.Identifiers[IdentifierKey{Kind: "view", Identifier: "keep-id"}] = "combined.yaml#1"
	after.Paths["combined.yaml"] = true

	plan := Diff(before, after)
	require.Len(t, plan.NoIdentifier, 1, "one of the two no-identifier documents vanished from the surviving file and must be reported")
	assert.Equal(t, "combined.yaml#1", plan.NoIdentifier[0])
	assert.Empty(t, plan.ByIdentifier, "the surviving View must not be affected")
}

func TestDiff_EmptyWhenBothSnapshotsEmpty(t *testing.T) {
	plan := Diff(newSnapshot(), newSnapshot())
	assert.True(t, plan.IsEmpty())
}

func TestDiff_ByIdentifierSortOrder(t *testing.T) {
	// Four deletions across three kinds, with two identifiers sharing the
	// same "dashboard" kind, so the sort must break ties on Identifier
	// rather than stopping at the Kind comparison. Map iteration order is
	// randomized on every run, so this alone is enough to exercise both
	// branches of the comparator regardless of insertion order.
	before := newSnapshot()
	before.Identifiers[IdentifierKey{Kind: "view", Identifier: "z"}] = "view.yaml"
	before.Identifiers[IdentifierKey{Kind: "dashboard", Identifier: "b"}] = "dashboard-b.yaml"
	before.Identifiers[IdentifierKey{Kind: "dashboard", Identifier: "a"}] = "dashboard-a.yaml"
	before.Identifiers[IdentifierKey{Kind: "checkrule", Identifier: "m"}] = "checkrule.yaml"

	after := newSnapshot()

	plan := Diff(before, after)
	require.Len(t, plan.ByIdentifier, 4)
	assert.Equal(t, []Deletion{
		{Kind: "checkrule", Identifier: "m", Path: "checkrule.yaml"},
		{Kind: "dashboard", Identifier: "a", Path: "dashboard-a.yaml"},
		{Kind: "dashboard", Identifier: "b", Path: "dashboard-b.yaml"},
		{Kind: "view", Identifier: "z", Path: "view.yaml"},
	}, plan.ByIdentifier)
}

func TestDiff_AlertsByNameSortOrder(t *testing.T) {
	// Two surviving CRDs, each losing alerts, with "crd-2" losing two so the
	// sort must break ties on CheckRuleName within the same CRDIdentifier.
	before := newSnapshot()
	before.Identifiers[IdentifierKey{Kind: "prometheusrule", Identifier: "crd-1"}] = "rules1.yaml"
	before.Identifiers[IdentifierKey{Kind: "prometheusrule", Identifier: "crd-2"}] = "rules2.yaml"
	before.PrometheusAlertsByIdentifier["crd-1"] = []dash0yaml.PrometheusAlertName{
		{GroupName: "g", AlertName: "B"},
		{GroupName: "g", AlertName: "A"},
	}
	before.PrometheusAlertsByIdentifier["crd-2"] = []dash0yaml.PrometheusAlertName{
		{GroupName: "g", AlertName: "Z"},
		{GroupName: "g", AlertName: "A"},
	}

	after := newSnapshot()
	after.Identifiers[IdentifierKey{Kind: "prometheusrule", Identifier: "crd-1"}] = "rules1.yaml"
	after.Identifiers[IdentifierKey{Kind: "prometheusrule", Identifier: "crd-2"}] = "rules2.yaml"
	after.PrometheusAlertsByIdentifier["crd-1"] = []dash0yaml.PrometheusAlertName{
		{GroupName: "g", AlertName: "A"},
	}
	after.PrometheusAlertsByIdentifier["crd-2"] = nil

	plan := Diff(before, after)
	require.Len(t, plan.AlertsByName, 3)
	names := make([]string, len(plan.AlertsByName))
	crds := make([]string, len(plan.AlertsByName))
	for i, d := range plan.AlertsByName {
		names[i] = d.CheckRuleName()
		crds[i] = d.CRDIdentifier
	}
	assert.Equal(t, []string{"crd-1", "crd-2", "crd-2"}, crds)
	assert.Equal(t, []string{"g - B", "g - A", "g - Z"}, names)
}
