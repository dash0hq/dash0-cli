package apply

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/asset"
	gitutil "github.com/dash0hq/dash0-cli/internal/git"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	documents := []asset.Document{
		{Kind: "dashboard", Name: "Kept Dashboard", ID: "11111111-1111-1111-1111-111111111111", FilePath: "dashboard.yaml"},
	}

	stdout := testutil.CaptureStdout(t, func() {
		require.NoError(t, runDryRun(documents, true, "dir", "", nil))
	})

	var out []dryRunFileJSON
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	require.Len(t, out, 1)
	assert.Equal(t, "dashboard.yaml", out[0].Path)
	require.Len(t, out[0].Changes, 1)
	assert.Equal(t, dryRunChangeJSON{Op: "apply", Name: "Kept Dashboard", OriginOrID: "11111111-1111-1111-1111-111111111111"}, out[0].Changes[0])
}

// TestRunDryRun_JSON_MergedWithDeletions pins the JSON shape when a file has
// both a surviving (apply) and a removed (delete) asset -- they must appear
// as two entries in the same file's changes array, not as separate file
// entries.
func TestRunDryRun_JSON_MergedWithDeletions(t *testing.T) {
	withAgentMode(t, true)

	documents := []asset.Document{
		{Kind: "view", Name: "error-logs-view", ID: "33333333-3333-3333-3333-333333333333", FilePath: "assets.yaml"},
	}
	dp := &gitutil.SincePlan{
		Plan: gitutil.DeletionPlan{
			ByIdentifier: []gitutil.Deletion{
				{Kind: "checkrule", Identifier: "44444444-4444-4444-4444-444444444444", Path: "assets.yaml#1"},
			},
		},
		Names: map[string]string{"assets.yaml#1": "High Error Rate"},
	}

	stdout := testutil.CaptureStdout(t, func() {
		require.NoError(t, runDryRun(documents, true, "dir", "abc123", dp))
	})

	var out []dryRunFileJSON
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	require.Len(t, out, 1)
	assert.Equal(t, "assets.yaml", out[0].Path)
	require.Len(t, out[0].Changes, 2)
	assert.Equal(t, dryRunChangeJSON{Op: "apply", Name: "error-logs-view", OriginOrID: "33333333-3333-3333-3333-333333333333"}, out[0].Changes[0])
	assert.Equal(t, dryRunChangeJSON{Op: "delete", Name: "High Error Rate", OriginOrID: "44444444-4444-4444-4444-444444444444"}, out[0].Changes[1])
}

// TestRunDryRun_JSON_MergedWithDeletions_SubdirectoryScope is a regression
// test for a bug where a -f target that is a subdirectory of the repo (not
// the repo root) grouped a file's surviving and deleted assets under two
// different keys: gitutil.Deletion.Path is always repo-root-relative (from
// git ls-tree), while asset.Document.FilePath is always relative to the -f
// target itself -- so "dashboards/removed.yaml" (deletion) and
// "removed.yaml" (had it survived) would never merge, and the deletion
// would render as a separate, inconsistently-prefixed file entry instead of
// joining its file's other row. dp.Scope must be stripped from the
// deletion's path before grouping.
func TestRunDryRun_JSON_MergedWithDeletions_SubdirectoryScope(t *testing.T) {
	withAgentMode(t, true)

	documents := []asset.Document{
		{Kind: "dashboard", Name: "Kept Dashboard", ID: "11111111-1111-1111-1111-111111111111", FilePath: "keep.yaml"},
	}
	dp := &gitutil.SincePlan{
		Plan: gitutil.DeletionPlan{
			ByIdentifier: []gitutil.Deletion{
				{Kind: "dashboard", Identifier: "22222222-2222-2222-2222-222222222222", Path: "dashboards/removed.yaml"},
			},
		},
		Names: map[string]string{"dashboards/removed.yaml": "Old Dashboard"},
		Scope: "dashboards",
	}

	stdout := testutil.CaptureStdout(t, func() {
		require.NoError(t, runDryRun(documents, true, "dashboards", "abc123", dp))
	})

	var out []dryRunFileJSON
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	require.Len(t, out, 2)
	paths := []string{out[0].Path, out[1].Path}
	assert.ElementsMatch(t, []string{"keep.yaml", "removed.yaml"}, paths, "deletion path must have dp.Scope stripped, matching validated documents' basis")
}

// TestRunDryRun_JSON_MergedWithDeletions_SameFileSubdirectoryScope covers
// the actual merge case under a subdirectory -f target: a multi-document
// file with one surviving and one deleted asset must still land in a single
// {path, changes} entry, not two, once dp.Scope is accounted for.
func TestRunDryRun_JSON_MergedWithDeletions_SameFileSubdirectoryScope(t *testing.T) {
	withAgentMode(t, true)

	documents := []asset.Document{
		{Kind: "view", Name: "error-logs-view", ID: "33333333-3333-3333-3333-333333333333", FilePath: "assets.yaml"},
	}
	dp := &gitutil.SincePlan{
		Plan: gitutil.DeletionPlan{
			ByIdentifier: []gitutil.Deletion{
				{Kind: "checkrule", Identifier: "44444444-4444-4444-4444-444444444444", Path: "dashboards/assets.yaml#1"},
			},
		},
		Names: map[string]string{"dashboards/assets.yaml#1": "High Error Rate"},
		Scope: "dashboards",
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

	documents := []asset.Document{
		{Kind: "dashboard", Name: "Solo Dashboard", ID: "id-1"},
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

	documents := []asset.Document{
		{Kind: "dashboard", Name: "Kept Dashboard", ID: "11111111-1111-1111-1111-111111111111", FilePath: "dashboard.yaml"},
	}

	stdout := testutil.CaptureStdout(t, func() {
		require.NoError(t, runDryRun(documents, true, "dir", "", nil))
	})

	assert.Contains(t, stdout, "Dry run: 1 document from 1 file validated")
	assert.Contains(t, stdout, `Apply Dashboard "Kept Dashboard" (11111111-1111-1111-1111-111111111111)`)
	assert.False(t, bytes.HasPrefix([]byte(stdout), []byte("[")), "text mode must not emit JSON")
}
