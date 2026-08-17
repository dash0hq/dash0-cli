package apply

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/agentmode"
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
	assert.Equal(t, dryRunChangeJSON{Op: "apply", Name: "Kept Dashboard", OriginOrID: "11111111-1111-1111-1111-111111111111"}, out[0].Changes[0])
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
	assert.Equal(t, dryRunChangeJSON{Op: "apply", Name: "error-logs-view", OriginOrID: "33333333-3333-3333-3333-333333333333"}, out[0].Changes[0])
	assert.Equal(t, dryRunChangeJSON{Op: "delete", Name: "High Error Rate", OriginOrID: "44444444-4444-4444-4444-444444444444"}, out[0].Changes[1])
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
