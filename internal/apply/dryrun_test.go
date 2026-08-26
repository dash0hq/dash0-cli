package apply

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/asset"
	gitutil "github.com/dash0hq/dash0-cli/internal/git"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStripScope is a table test for the deletion-path/validated-document
// path basis mismatch under a subdirectory -f target: gitutil.Deletion.Path
// is repo-root-relative, assetDocument.filePath is -f-target-relative, and
// scope (the -f target's own repo-root-relative path) is what bridges them.
func TestStripScope(t *testing.T) {
	cases := []struct {
		path  string
		scope string
		want  string
	}{
		{"keep.yaml", "", "keep.yaml"},
		{"dashboards/keep.yaml", "dashboards", "keep.yaml"},
		{"dashboards/nested/keep.yaml", "dashboards", "nested/keep.yaml"},
		// No prefix match: leave the path untouched rather than guessing.
		{"other/keep.yaml", "dashboards", "other/keep.yaml"},
		// A directory name that merely starts with scope's name, without the
		// separator, must not be treated as a match.
		{"dashboards2/keep.yaml", "dashboards", "dashboards2/keep.yaml"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, stripScope(c.path, c.scope), "path %q scope %q", c.path, c.scope)
	}
}

func withAgentMode(t *testing.T, enabled bool) {
	t.Helper()
	prev := agentmode.Enabled
	agentmode.Enabled = enabled
	t.Cleanup(func() { agentmode.Enabled = prev })
}

// TestRunDryRun_JSON_PlainNoSince pins the agent-mode JSON shape for a plain
// --dry-run (no --since): an array of {path, changes}, one entry per file,
// every change carrying op "apply".
func TestRunDryRun_JSON_PlainNoSince(t *testing.T) {
	withAgentMode(t, true)

	documents := []assetDocument{
		{kind: "dashboard", name: "Kept Dashboard", id: "11111111-1111-1111-1111-111111111111", filePath: "dashboard.yaml"},
	}

	stdout := testutil.CaptureStdout(t, func() {
		require.NoError(t, runDryRun(documents, true, "dir", "", nil))
	})

	var out []dryRunFileJSON
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	require.Len(t, out, 1)
	assert.Equal(t, "dashboard.yaml", out[0].Path)
	require.Len(t, out[0].Changes, 1)
	assert.Equal(t, dryRunChangeJSON{Op: "apply", Kind: "Dashboard", Name: "Kept Dashboard", OriginOrID: "11111111-1111-1111-1111-111111111111"}, out[0].Changes[0])
}

// TestRunDryRun_JSON_MergedWithDeletions pins the JSON shape when a file has
// both a surviving (apply) and a removed (delete) asset -- they must appear
// as two entries in the same file's changes array, not as separate file
// entries.
func TestRunDryRun_JSON_MergedWithDeletions(t *testing.T) {
	withAgentMode(t, true)

	documents := []assetDocument{
		{kind: "view", name: "error-logs-view", id: "33333333-3333-3333-3333-333333333333", filePath: "assets.yaml"},
	}
	dp := &deletionPlan{
		plan: gitutil.DeletionPlan{
			ByIdentifier: []gitutil.Deletion{
				{Kind: "checkrule", Identifier: "44444444-4444-4444-4444-444444444444", Path: "assets.yaml#1"},
			},
		},
		names: map[string]string{"assets.yaml#1": "High Error Rate"},
	}

	stdout := testutil.CaptureStdout(t, func() {
		require.NoError(t, runDryRun(documents, true, "dir", "abc123", dp))
	})

	var out []dryRunFileJSON
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	require.Len(t, out, 1)
	assert.Equal(t, "assets.yaml", out[0].Path)
	require.Len(t, out[0].Changes, 2)
	assert.Equal(t, dryRunChangeJSON{Op: "apply", Kind: "View", Name: "error-logs-view", OriginOrID: "33333333-3333-3333-3333-333333333333"}, out[0].Changes[0])
	assert.Equal(t, dryRunChangeJSON{Op: "delete", Kind: "Check rule", Name: "High Error Rate", OriginOrID: "44444444-4444-4444-4444-444444444444", Since: "abc123"}, out[0].Changes[1])
}

// TestRunDryRun_JSON_DeletionIncludesKindAndSinceRef is a regression test
// for a bug where agent mode's --dry-run JSON gave an approving agent less
// context than a human reading the text output: the text renderer's
// "Delete View "Team Logs" (team-logs)" names the asset's kind, but the
// JSON change entry carried only {op, name, originOrId} -- no kind, and no
// mention of which --since ref determined the deletion. An agent deciding
// whether a deletion is safe to approve needs to know what kind of asset is
// about to be removed (a view is a very different risk than a production
// dashboard) as much as a human does.
func TestRunDryRun_JSON_DeletionIncludesKindAndSinceRef(t *testing.T) {
	withAgentMode(t, true)

	dp := &deletionPlan{
		plan: gitutil.DeletionPlan{
			ByIdentifier: []gitutil.Deletion{
				{Kind: "view", Identifier: "team-logs", Path: "dashboards.yaml"},
			},
		},
		names: map[string]string{"dashboards.yaml": "Team Logs"},
	}

	stdout := testutil.CaptureStdout(t, func() {
		require.NoError(t, runDryRun(nil, true, "dashboards", "HEAD~1", dp))
	})

	var out []dryRunFileJSON
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	require.Len(t, out, 1)
	require.Len(t, out[0].Changes, 1)
	assert.Equal(t, dryRunChangeJSON{Op: "delete", Kind: "View", Name: "Team Logs", OriginOrID: "team-logs", Since: "HEAD~1"}, out[0].Changes[0])
}

// TestRunDryRun_JSON_MergedWithDeletions_SubdirectoryScope is a regression
// test for a bug where a -f target that is a subdirectory of the repo (not
// the repo root) grouped a file's surviving and deleted assets under two
// different keys: gitutil.Deletion.Path is always repo-root-relative (from
// git ls-tree), while assetDocument.filePath is always relative to the -f
// target itself -- so "dashboards/removed.yaml" (deletion) and
// "removed.yaml" (had it survived) would never merge, and the deletion
// would render as a separate, inconsistently-prefixed file entry instead of
// joining its file's other row. dp.scope must be stripped from the
// deletion's path before grouping.
func TestRunDryRun_JSON_MergedWithDeletions_SubdirectoryScope(t *testing.T) {
	withAgentMode(t, true)

	documents := []assetDocument{
		{kind: "dashboard", name: "Kept Dashboard", id: "11111111-1111-1111-1111-111111111111", filePath: "keep.yaml"},
	}
	dp := &deletionPlan{
		plan: gitutil.DeletionPlan{
			ByIdentifier: []gitutil.Deletion{
				{Kind: "dashboard", Identifier: "22222222-2222-2222-2222-222222222222", Path: "dashboards/removed.yaml"},
			},
		},
		names: map[string]string{"dashboards/removed.yaml": "Old Dashboard"},
		scope: "dashboards",
	}

	stdout := testutil.CaptureStdout(t, func() {
		require.NoError(t, runDryRun(documents, true, "dashboards", "abc123", dp))
	})

	var out []dryRunFileJSON
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	require.Len(t, out, 2)
	paths := []string{out[0].Path, out[1].Path}
	assert.ElementsMatch(t, []string{"keep.yaml", "removed.yaml"}, paths, "deletion path must have dp.scope stripped, matching validated documents' basis")
}

// TestRunDryRun_JSON_MergedWithDeletions_SameFileSubdirectoryScope covers
// the actual merge case under a subdirectory -f target: a multi-document
// file with one surviving and one deleted asset must still land in a single
// {path, changes} entry, not two, once dp.scope is accounted for.
func TestRunDryRun_JSON_MergedWithDeletions_SameFileSubdirectoryScope(t *testing.T) {
	withAgentMode(t, true)

	documents := []assetDocument{
		{kind: "view", name: "error-logs-view", id: "33333333-3333-3333-3333-333333333333", filePath: "assets.yaml"},
	}
	dp := &deletionPlan{
		plan: gitutil.DeletionPlan{
			ByIdentifier: []gitutil.Deletion{
				{Kind: "checkrule", Identifier: "44444444-4444-4444-4444-444444444444", Path: "dashboards/assets.yaml#1"},
			},
		},
		names: map[string]string{"dashboards/assets.yaml#1": "High Error Rate"},
		scope: "dashboards",
	}

	stdout := testutil.CaptureStdout(t, func() {
		require.NoError(t, runDryRun(documents, true, "dashboards", "abc123", dp))
	})

	var out []dryRunFileJSON
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	require.Len(t, out, 1, "the surviving and deleted assets from the same file must merge into one entry")
	assert.Equal(t, "assets.yaml", out[0].Path)
	require.Len(t, out[0].Changes, 2)
}

// TestRunDryRun_JSON_SingleFileTarget pins that a single-file (non-directory)
// -f target reports one file entry keyed by the literal -f argument, since
// there is no real per-document file grouping to report.
func TestRunDryRun_JSON_SingleFileTarget(t *testing.T) {
	withAgentMode(t, true)

	documents := []assetDocument{
		{kind: "dashboard", name: "Solo Dashboard", id: "id-1"},
	}

	stdout := testutil.CaptureStdout(t, func() {
		require.NoError(t, runDryRun(documents, false, "dashboard.yaml", "", nil))
	})

	var out []dryRunFileJSON
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	require.Len(t, out, 1)
	assert.Equal(t, "dashboard.yaml", out[0].Path)
	require.Len(t, out[0].Changes, 1)
	assert.Equal(t, "apply", out[0].Changes[0].Op)
}

// TestRunDryRun_TextMode_Unaffected confirms agent mode's JSON path is
// opt-in only -- with agent mode disabled, runDryRun still renders the
// existing plain-text output.
func TestRunDryRun_TextMode_Unaffected(t *testing.T) {
	withAgentMode(t, false)

	documents := []assetDocument{
		{kind: "dashboard", name: "Kept Dashboard", id: "11111111-1111-1111-1111-111111111111", filePath: "dashboard.yaml"},
	}

	stdout := testutil.CaptureStdout(t, func() {
		require.NoError(t, runDryRun(documents, true, "dir", "", nil))
	})

	assert.Contains(t, stdout, "Dry run: 1 document from 1 file validated")
	assert.Contains(t, stdout, `Apply Dashboard "Kept Dashboard" (11111111-1111-1111-1111-111111111111)`)
	assert.False(t, bytes.HasPrefix([]byte(stdout), []byte("[")), "text mode must not emit JSON")
}

// prometheusRuleCRDForDryRun is a two-alert CRD whose dash0.com/id label is
// what buildDryRunRows keys crdFileByIdentifier on, so an alert deletion
// (which carries only its surviving CRD's identifier, never a file path)
// lands under the same file entry as the CRD's own "apply" row.
const prometheusRuleCRDForDryRun = `apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: test-rules
  labels:
    dash0.com/id: shared-id
spec:
  groups:
    - name: test-group
      rules:
        - alert: HighErrorRate
          expr: sum(rate(errors[5m])) > 0.1
`

// TestRunDryRun_TextMode_PrometheusRuleAlertPartialDeletion is the --dry-run
// analogue of TestApply_Since_PrometheusRuleAlertPartialDeletion: one alert
// removed from a CRD that otherwise survives. The surviving CRD's "apply"
// row and the removed alert's "delete" row must group under the same file,
// and the delete line must carry the detail suffix naming which CRD the
// alert was removed from -- without it, "Delete Check rule "test-group -
// DiskFull"" gives no hint that the CRD itself is being kept.
func TestRunDryRun_TextMode_PrometheusRuleAlertPartialDeletion(t *testing.T) {
	withAgentMode(t, false)

	documents := []assetDocument{
		{kind: "PrometheusRule", name: "test-rules", filePath: "rules.yaml", raw: []byte(prometheusRuleCRDForDryRun)},
	}
	dp := &deletionPlan{
		plan: gitutil.DeletionPlan{
			AlertsByName: []gitutil.AlertDeletion{
				{
					CRDIdentifier:       "shared-id",
					PrometheusAlertName: asset.PrometheusAlertName{GroupName: "test-group", AlertName: "DiskFull"},
				},
			},
		},
	}

	stdout := testutil.CaptureStdout(t, func() {
		require.NoError(t, runDryRun(documents, true, "dir", "HEAD~1", dp))
	})

	assert.Contains(t, stdout, "1 deletion pending due to --since 'HEAD~1'")
	// A single file heading, with both rows nested under it.
	assert.Equal(t, 1, strings.Count(stdout, "\n  rules.yaml\n"), "both rows must group under the surviving CRD's own file")
	assert.Contains(t, stdout, `    * Apply PrometheusRule "test-rules" (shared-id)`)
	assert.Contains(t, stdout, `    * Delete Check rule "test-group - DiskFull" (alert removed from PrometheusRule shared-id)`)
}

// TestRunDryRun_JSON_PrometheusRuleAlertPartialDeletion pins the agent-mode
// JSON for the same case. The detail suffix is text-only (dryRunChangeJSON
// has no field for it), so the JSON's originOrId is the surviving CRD's
// identifier -- an agent must not read that as "the CRD is being deleted",
// which is why the change's name is the alert's composed check-rule name and
// its kind is CheckRule, not PrometheusRule.
func TestRunDryRun_JSON_PrometheusRuleAlertPartialDeletion(t *testing.T) {
	withAgentMode(t, true)

	documents := []assetDocument{
		{kind: "PrometheusRule", name: "test-rules", filePath: "rules.yaml", raw: []byte(prometheusRuleCRDForDryRun)},
	}
	dp := &deletionPlan{
		plan: gitutil.DeletionPlan{
			AlertsByName: []gitutil.AlertDeletion{
				{
					CRDIdentifier:       "shared-id",
					PrometheusAlertName: asset.PrometheusAlertName{GroupName: "test-group", AlertName: "DiskFull"},
				},
			},
		},
	}

	stdout := testutil.CaptureStdout(t, func() {
		require.NoError(t, runDryRun(documents, true, "dir", "HEAD~1", dp))
	})

	var out []dryRunFileJSON
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	require.Len(t, out, 1, "the alert deletion must join its surviving CRD's file entry")
	assert.Equal(t, "rules.yaml", out[0].Path)
	require.Len(t, out[0].Changes, 2)
	assert.Equal(t, dryRunChangeJSON{Op: "apply", Kind: "PrometheusRule", Name: "test-rules", OriginOrID: "shared-id"}, out[0].Changes[0])
	assert.Equal(t, dryRunChangeJSON{Op: "delete", Kind: "Check rule", Name: "test-group - DiskFull", OriginOrID: "shared-id", Since: "HEAD~1"}, out[0].Changes[1])
}
