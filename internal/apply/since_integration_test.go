//go:build integration

package apply

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSinceTestCmd() *cobra.Command {
	cmd := NewApplyCmd()
	withExperimentalFlag(cmd)
	return cmd
}

func TestApply_Since_WholeFileDeletion(t *testing.T) {
	testutil.SetupTestEnv(t)

	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "dashboard.yaml", `apiVersion: dash0.com/v1alpha1
kind: Dashboard
metadata:
  name: my-dashboard
  dash0Extensions:
    id: a1b2c3d4-5678-90ab-cdef-1234567890ab
spec:
  display:
    name: My Dashboard
`)
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "add dashboard")
	before := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	require.NoError(t, os.Remove(filepath.Join(dir, "dashboard.yaml")))
	writeFileFixture(t, dir, "keep.yaml", "apiVersion: dash0.com/v1alpha1\nkind: View\nmetadata:\n  name: keep\n  labels:\n    dash0.com/id: keep-id\nspec:\n  query: \"true\"\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "remove dashboard")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureViewsNotFound,
	})
	server.WithViewsUpdate(testutil.FixtureViewsImportSuccess)
	server.OnPattern(http.MethodDelete, dashboardIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{
		"-f", dir, "--since", before, "--force", "--experimental",
		"--api-url", server.URL, "--auth-token", testAuthToken,
	})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "Dashboard")
	assert.Contains(t, output, "a1b2c3d4-5678-90ab-cdef-1234567890ab")
	assert.Contains(t, output, "deleted")
}

// TestApply_Since_AllFilesDeleted_DirectorySurvives is a regression test for
// a bug where --since found nothing to delete (in fact, failed the whole
// run outright) once every asset definition under -f's target had been
// removed and the (now-empty) directory itself survived: runApply's
// directory-discovery step (readDirectory) hard-failed with "no .yaml or
// .yml files found" before computeDeletionPlan ever got a chance to run, so
// --since's very purpose -- detecting an all-deletions run -- was
// unreachable for exactly the case it exists to handle. Unlike
// TestApply_Since_WholeFileDeletion, no "keep.yaml" survivor is written
// after the removal: the point of this test is that none is needed.
func TestApply_Since_AllFilesDeleted_DirectorySurvives(t *testing.T) {
	testutil.SetupTestEnv(t)

	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "dashboard.yaml", `apiVersion: dash0.com/v1alpha1
kind: Dashboard
metadata:
  name: my-dashboard
  dash0Extensions:
    id: a1b2c3d4-5678-90ab-cdef-1234567890ab
spec:
  display:
    name: My Dashboard
`)
	writeFileFixture(t, dir, "view.yaml", "apiVersion: dash0.com/v1alpha1\nkind: View\nmetadata:\n  name: my-view\n  labels:\n    dash0.com/id: b2c3d4e5-6789-01bc-def0-234567890abc\nspec:\n  query: \"true\"\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "add dashboard and view")
	before := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	require.NoError(t, os.Remove(filepath.Join(dir, "dashboard.yaml")))
	require.NoError(t, os.Remove(filepath.Join(dir, "view.yaml")))
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "remove both")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, dashboardIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodDelete, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{
		"-f", dir, "--since", before, "--force", "--experimental",
		"--api-url", server.URL, "--auth-token", testAuthToken,
	})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "a1b2c3d4-5678-90ab-cdef-1234567890ab")
	assert.Contains(t, output, "b2c3d4e5-6789-01bc-def0-234567890abc")
	assert.Contains(t, output, "deleted")
}

// TestApply_Since_AllFilesDeleted_TargetDirectoryRemoved is the same
// all-deletions scenario as TestApply_Since_AllFilesDeleted_DirectorySurvives,
// except the -f target directory was removed entirely along with its files
// (rather than surviving empty) -- a plausible outcome of the same "delete
// everything" cleanup, and a second, independent failure mode of the
// original bug: os.Stat(flags.File) itself failed before runApply could even
// decide whether to treat the target as a directory.
func TestApply_Since_AllFilesDeleted_TargetDirectoryRemoved(t *testing.T) {
	testutil.SetupTestEnv(t)

	repoRoot := t.TempDir()
	runGitCmd(t, repoRoot, "init", "-q", "-b", "main")
	runGitCmd(t, repoRoot, "config", "user.email", "test@example.com")
	runGitCmd(t, repoRoot, "config", "user.name", "Test")
	runGitCmd(t, repoRoot, "config", "commit.gpgsign", "false")

	writeFileFixture(t, repoRoot, "dashboards/dashboard.yaml", `apiVersion: dash0.com/v1alpha1
kind: Dashboard
metadata:
  name: my-dashboard
  dash0Extensions:
    id: a1b2c3d4-5678-90ab-cdef-1234567890ab
spec:
  display:
    name: My Dashboard
`)
	runGitCmd(t, repoRoot, "add", "-A")
	runGitCmd(t, repoRoot, "commit", "-q", "-m", "add dashboard")
	before := strings.TrimSpace(runGitCmd(t, repoRoot, "rev-parse", "HEAD"))

	target := filepath.Join(repoRoot, "dashboards")
	require.NoError(t, os.RemoveAll(target))
	runGitCmd(t, repoRoot, "add", "-A")
	runGitCmd(t, repoRoot, "commit", "-q", "-m", "remove dashboards directory entirely")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, dashboardIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{
		"-f", target, "--since", before, "--force", "--experimental",
		"--api-url", server.URL, "--auth-token", testAuthToken,
	})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "a1b2c3d4-5678-90ab-cdef-1234567890ab")
	assert.Contains(t, output, "deleted")
}

// TestApply_Since_WholeFileDeletion_SubdirectoryScope is a regression test
// for a bug where -f pointed at a subdirectory of the repo (rather than the
// repo root) made --since silently report zero deletions: the git-side
// pathspec is always repo-root-relative, but the git plumbing calls were
// running with -C set to the scope directory itself, so the pathspec
// resolved to a nonexistent nested path (e.g. "dashboards/dashboards") and
// git ls-tree returned an empty (not an error) result. This is the CLI's own
// documented apply --since usage pattern (-f dashboards/), so it must work.
func TestApply_Since_WholeFileDeletion_SubdirectoryScope(t *testing.T) {
	testutil.SetupTestEnv(t)

	repoRoot := t.TempDir()
	runGitCmd(t, repoRoot, "init", "-q", "-b", "main")
	runGitCmd(t, repoRoot, "config", "user.email", "test@example.com")
	runGitCmd(t, repoRoot, "config", "user.name", "Test")
	runGitCmd(t, repoRoot, "config", "commit.gpgsign", "false")

	writeFileFixture(t, repoRoot, "dashboards/dashboard.yaml", `apiVersion: dash0.com/v1alpha1
kind: Dashboard
metadata:
  name: my-dashboard
  dash0Extensions:
    id: a1b2c3d4-5678-90ab-cdef-1234567890ab
spec:
  display:
    name: My Dashboard
`)
	runGitCmd(t, repoRoot, "add", "-A")
	runGitCmd(t, repoRoot, "commit", "-q", "-m", "add dashboard")
	before := strings.TrimSpace(runGitCmd(t, repoRoot, "rev-parse", "HEAD"))

	require.NoError(t, os.Remove(filepath.Join(repoRoot, "dashboards", "dashboard.yaml")))
	writeFileFixture(t, repoRoot, "dashboards/keep.yaml", "apiVersion: dash0.com/v1alpha1\nkind: View\nmetadata:\n  name: keep\n  labels:\n    dash0.com/id: keep-id\nspec:\n  query: \"true\"\n")
	runGitCmd(t, repoRoot, "add", "-A")
	runGitCmd(t, repoRoot, "commit", "-q", "-m", "remove dashboard")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureViewsNotFound,
	})
	server.WithViewsUpdate(testutil.FixtureViewsImportSuccess)
	server.OnPattern(http.MethodDelete, dashboardIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{
		"-f", filepath.Join(repoRoot, "dashboards"), "--since", before, "--force", "--experimental",
		"--api-url", server.URL, "--auth-token", testAuthToken,
	})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "Dashboard")
	assert.Contains(t, output, "a1b2c3d4-5678-90ab-cdef-1234567890ab")
	assert.Contains(t, output, "deleted")

	deleteReq := findRequest(server.Requests(), http.MethodDelete, "/api/dashboards/a1b2c3d4-5678-90ab-cdef-1234567890ab")
	require.NotNil(t, deleteReq, "expected a DELETE request for the dashboard removed from the subdirectory-scoped repo")
}

func TestApply_Since_PrometheusRuleAlertPartialDeletion(t *testing.T) {
	testutil.SetupTestEnv(t)

	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "rules.yaml", `apiVersion: monitoring.coreos.com/v1
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
        - alert: DiskFull
          expr: disk > 0.9
`)
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "add rules with two alerts")
	before := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	// Remove the DiskFull alert; the CRD (and its shared identifier) survives.
	writeFileFixture(t, dir, "rules.yaml", `apiVersion: monitoring.coreos.com/v1
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
`)
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "remove DiskFull alert")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	// The surviving alert (HighErrorRate) shares the CRD's dash0.com/id label
	// (existing product behavior — see docs/commands.md's PrometheusRule
	// note on multi-alert CRDs sharing one identifier), so it upserts via
	// PUT to that id rather than going through the GET-404-then-POST path.
	server.OnPattern(http.MethodGet, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureCheckRulesNotFound,
	})
	server.WithCheckRulesUpdate(testutil.FixtureCheckRulesImportSuccess)
	// The removed alert (DiskFull) must be resolved to a check rule by name,
	// since the CRD's shared identifier can't distinguish between its alerts.
	server.On(http.MethodGet, apiPathCheckRules, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body: []map[string]any{
			{"dataset": "default", "id": "disk-full-check-rule-id", "name": "test-group - DiskFull"},
		},
		Validator: testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodDelete, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{
		"-f", dir, "--since", before, "--force", "--experimental",
		"--api-url", server.URL, "--auth-token", testAuthToken,
	})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "test-group - DiskFull")
	assert.Contains(t, output, "deleted")

	deleteReq := findRequest(server.Requests(), http.MethodDelete, "/api/alerting/check-rules/disk-full-check-rule-id")
	require.NotNil(t, deleteReq, "expected a DELETE request for the removed alert's resolved check rule id")
}

// TestApply_Since_PersesDashboardAlreadyDeletedPreservesCanonicalKindName is a
// regression test for deleteAssetByKindAndIdentifier's client.ErrorContext
// construction: it must pass asset.KindDisplayName's canonical form straight
// through (no case transform at all), so a compound kind name like
// "PersesDashboard" is never mangled into "persesdashboard" (a plain
// strings.ToLower) or an invented hybrid like "persesDashboard" (an earlier,
// since-reverted lowerFirst-only-first-rune attempt) — both are stand-ins for
// a kind name that doesn't correspond to anything real. Exercised via the
// --force "already deleted" idempotent-delete message
// (client.IsAlreadyDeleted -> capitalizeFirst(ectx.AssetType)), which is
// idempotent no matter the input casing, making it the cleanest place to
// observe AssetType's actual value.
func TestApply_Since_PersesDashboardAlreadyDeletedPreservesCanonicalKindName(t *testing.T) {
	testutil.SetupTestEnv(t)

	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "dashboard.yaml", `apiVersion: perses.dev/v1alpha1
kind: PersesDashboard
metadata:
  name: my-perses-dashboard
  labels:
    dash0.com/id: perses-id
spec:
  display:
    name: My Perses Dashboard
  duration: 5m
  panels: {}
`)
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "add perses dashboard")
	before := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	require.NoError(t, os.Remove(filepath.Join(dir, "dashboard.yaml")))
	writeFileFixture(t, dir, "keep.yaml", "apiVersion: dash0.com/v1alpha1\nkind: View\nmetadata:\n  name: keep\n  labels:\n    dash0.com/id: keep-id\nspec:\n  query: \"true\"\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "remove perses dashboard")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureViewsNotFound,
	})
	server.WithViewsUpdate(testutil.FixtureViewsImportSuccess)
	// The live asset is already gone: DELETE 404s. With --force this must be
	// treated as an idempotent success (client.IsAlreadyDeleted), printing
	// "PersesDashboard ... was already deleted" to stderr.
	server.OnPattern(http.MethodDelete, dashboardIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureDashboardsNotFound,
	})

	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{
		"-f", dir, "--since", before, "--force", "--experimental",
		"--api-url", server.URL, "--auth-token", testAuthToken,
	})

	var cmdErr error
	stderr := testutil.CaptureStderr(t, func() {
		testutil.CaptureStdout(t, func() {
			cmdErr = cmd.Execute()
		})
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, stderr, "PersesDashboard \"perses-id\" was already deleted")
	assert.NotContains(t, stderr, "persesdashboard", "the whole kind name must never be force-lowercased into an unreadable compound word")
	assert.NotContains(t, stderr, "persesDashboard", "the kind name must never be turned into an invented hybrid casing either")
}

// TestApply_Since_PrometheusRuleWholeCRDDeletion_RecordDroppedBeforeDeletion_StillDeletesRecordingRule
// is a regression test for a bug where deleting a whole PrometheusRule CRD
// undercounted which endpoints to clean up, based solely on the CRD's
// content at --since's own ref -- a single point in time, not a history of
// everything the identifier has ever used. A CRD that starts out mixed
// (alert + record), then has its record entry dropped in an earlier commit
// while keeping the alert, then is deleted entirely, shows --since a ref
// where the file only ever had an alert: the old code carried that stale
// "alerting-only" signal straight into the delete dispatch and never called
// DELETE on the recording-rules endpoint at all, permanently orphaning the
// recording rule created back when the file was still mixed -- nothing
// after the file is gone can ever recover that fact from git. The fix
// always attempts both endpoints (tolerating a 404 from whichever wasn't
// actually used), so the orphaned recording rule is cleaned up too.
func TestApply_Since_PrometheusRuleWholeCRDDeletion_RecordDroppedBeforeDeletion_StillDeletesRecordingRule(t *testing.T) {
	testutil.SetupTestEnv(t)

	const id = "app-rules-id"

	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "rules.yaml", `apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: app-rules
  labels:
    dash0.com/id: `+id+`
spec:
  groups:
    - name: test-group
      rules:
        - alert: HighErrorRate
          expr: sum(rate(errors[5m])) > 0.1
        - record: my_record
          expr: rate(x[5m])
`)
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "add mixed rules")

	writeFileFixture(t, dir, "rules.yaml", `apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: app-rules
  labels:
    dash0.com/id: `+id+`
spec:
  groups:
    - name: test-group
      rules:
        - alert: HighErrorRate
          expr: sum(rate(errors[5m])) > 0.1
`)
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "drop the record, keep the alert")
	before := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	require.NoError(t, os.Remove(filepath.Join(dir, "rules.yaml")))
	writeFileFixture(t, dir, "keep.yaml", "apiVersion: dash0.com/v1alpha1\nkind: View\nmetadata:\n  name: keep\n  labels:\n    dash0.com/id: keep-id\nspec:\n  query: \"true\"\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "remove rules.yaml entirely")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureViewsNotFound,
	})
	server.WithViewsUpdate(testutil.FixtureViewsImportSuccess)
	server.OnPattern(http.MethodDelete, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})
	// The recording rule created back when rules.yaml was still mixed --
	// still live server-side, even though the ref --since compares against
	// only ever shows the file as alerting-only.
	server.OnPattern(http.MethodDelete, recordingRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{
		"-f", dir, "--since", before, "--force", "--experimental",
		"--api-url", server.URL, "--auth-token", testAuthToken,
	})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "PrometheusRule")
	assert.Contains(t, output, id)
	assert.Contains(t, output, "deleted")

	require.NotNil(t, findRequest(server.Requests(), http.MethodDelete, "/api/alerting/check-rules/"+id), "expected the CRD's check rule to be deleted")
	require.NotNil(t, findRequest(server.Requests(), http.MethodDelete, "/api/recording-rules/"+id), "expected the recording rule orphaned by dropping the record entry to be deleted too, even though --since's ref only ever showed the file as alerting-only")
}

// TestApply_Since_PrometheusRuleWholeCRDDeletion_ToleratesRecordingRule404
// pins the safety side of the fix above: an alerting-only CRD that never had
// a recording rule still has DELETE attempted against the recording-rules
// endpoint (since the code no longer knows, or needs to know, whether it
// ever used it) -- that attempt must 404 harmlessly and not fail the run.
func TestApply_Since_PrometheusRuleWholeCRDDeletion_ToleratesRecordingRule404(t *testing.T) {
	testutil.SetupTestEnv(t)

	const id = "alerting-only-id"

	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "rules.yaml", `apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: alerting-only-rules
  labels:
    dash0.com/id: `+id+`
spec:
  groups:
    - name: test-group
      rules:
        - alert: HighErrorRate
          expr: sum(rate(errors[5m])) > 0.1
`)
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "add alerting-only rules")
	before := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	require.NoError(t, os.Remove(filepath.Join(dir, "rules.yaml")))
	writeFileFixture(t, dir, "keep.yaml", "apiVersion: dash0.com/v1alpha1\nkind: View\nmetadata:\n  name: keep\n  labels:\n    dash0.com/id: keep-id\nspec:\n  query: \"true\"\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "remove alerting-only rules")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureViewsNotFound,
	})
	server.WithViewsUpdate(testutil.FixtureViewsImportSuccess)
	server.OnPattern(http.MethodDelete, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodDelete, recordingRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureRecordingRulesNotFound,
	})

	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{
		"-f", dir, "--since", before, "--force", "--experimental",
		"--api-url", server.URL, "--auth-token", testAuthToken,
	})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "PrometheusRule")
	assert.Contains(t, output, id)
	assert.Contains(t, output, "deleted")

	require.NotNil(t, findRequest(server.Requests(), http.MethodDelete, "/api/alerting/check-rules/"+id))
	require.NotNil(t, findRequest(server.Requests(), http.MethodDelete, "/api/recording-rules/"+id), "the recording-rules endpoint must still be attempted even for a CRD that never had a recording rule")
}

func TestApply_Since_MultiDocumentPartialDeletion(t *testing.T) {
	testutil.SetupTestEnv(t)

	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "combined.yaml", `apiVersion: dash0.com/v1alpha1
kind: Dashboard
metadata:
  name: my-dashboard
  dash0Extensions:
    id: a1b2c3d4-5678-90ab-cdef-1234567890ab
spec:
  display:
    name: My Dashboard
---
apiVersion: dash0.com/v1alpha1
kind: View
metadata:
  name: my-view
  labels:
    dash0.com/id: my-view-id
spec:
  query: "true"
`)
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "add combined file")
	before := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	// Remove the View document; the Dashboard document (and the file) survives.
	writeFileFixture(t, dir, "combined.yaml", `apiVersion: dash0.com/v1alpha1
kind: Dashboard
metadata:
  name: my-dashboard
  dash0Extensions:
    id: a1b2c3d4-5678-90ab-cdef-1234567890ab
spec:
  display:
    name: My Dashboard
`)
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "remove view document")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, dashboardIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureDashboardsNotFound,
	})
	server.WithDashboardsUpdate(testutil.FixtureDashboardsImportSuccess)
	server.OnPattern(http.MethodDelete, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{
		"-f", dir, "--since", before, "--force", "--experimental",
		"--api-url", server.URL, "--auth-token", testAuthToken,
	})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "View")
	assert.Contains(t, output, "my-view-id")
	assert.Contains(t, output, "deleted")

	deleteReq := findRequest(server.Requests(), http.MethodDelete, "/api/views/my-view-id")
	require.NotNil(t, deleteReq, "expected a DELETE request for the view document removed from the surviving file")
}

func TestApply_Since_PrometheusRecordingRulePartialRemovalIsNotADeletion(t *testing.T) {
	testutil.SetupTestEnv(t)

	const ruleID = "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "rules.yaml", `apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: mixed-rules
  labels:
    dash0.com/id: `+ruleID+`
spec:
  groups:
    - name: mixed-group
      interval: 1m
      rules:
        - alert: HighErrorRate
          expr: sum(rate(errors[5m])) > 0.1
        - record: instance:cpu_usage:avg5m
          expr: avg without(cpu) (rate(node_cpu_seconds_total{mode!="idle"}[5m]))
`)
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "add mixed rules")
	before := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	// Remove the recording rule; the alert (and the CRD's shared identifier)
	// survives. This is not tracked as a deletion at all — no per-record
	// identity exists to diff, so it is a plain update to the surviving CRD.
	writeFileFixture(t, dir, "rules.yaml", `apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: mixed-rules
  labels:
    dash0.com/id: `+ruleID+`
spec:
  groups:
    - name: mixed-group
      interval: 1m
      rules:
        - alert: HighErrorRate
          expr: sum(rate(errors[5m])) > 0.1
`)
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "remove recording rule")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureCheckRulesNotFound,
	})
	server.WithCheckRulesUpdate(testutil.FixtureCheckRulesImportSuccess)
	// No recording-rules route registered at all: the removed record entry
	// must never trigger a call to that endpoint, delete or otherwise.

	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{
		"-f", dir, "--since", before, "--force", "--experimental",
		"--api-url", server.URL, "--auth-token", testAuthToken,
	})

	var cmdErr error
	testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Nil(t, findRequest(server.Requests(), http.MethodDelete, apiPathRecordingRules+"/"+ruleID), "removing a record entry from a surviving CRD must not be treated as a deletion")
	require.NotNil(t, findRequest(server.Requests(), http.MethodPut, apiPathCheckRules+"/"+ruleID), "the surviving alert must still go through the ordinary update path")
}

func TestApply_Since_UnresolvableRef(t *testing.T) {
	testutil.SetupTestEnv(t)

	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")
	writeFileFixture(t, dir, "keep.yaml", "apiVersion: dash0.com/v1alpha1\nkind: View\nmetadata:\n  name: keep\n  labels:\n    dash0.com/id: keep-id\nspec:\n  query: \"true\"\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "add file")

	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{"-f", dir, "--since", "totally-bogus-ref", "--experimental"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not be resolved")
}

// setUpNonAncestorRefRepo builds a repo where "branch-a" (containing a View)
// diverges from "main" (which never merges it back in) — a stand-in for a
// force-pushed/rewritten history, the same construction
// TestClassifyRef_ResolvedNonAncestor uses. main also gets an unrelated
// Dashboard with no user-defined id, present only on main, so a plain create
// is expected regardless of what --since decides about the View.
func setUpNonAncestorRefRepo(t *testing.T) (dir, branchA string) {
	t.Helper()
	dir = t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")
	runGitCmd(t, dir, "commit", "-q", "--allow-empty", "-m", "initial")

	runGitCmd(t, dir, "checkout", "-q", "-b", "branch-a")
	writeFileFixture(t, dir, "assets/a.yaml", "apiVersion: dash0.com/v1alpha1\nkind: View\nmetadata:\n  name: a\n  labels:\n    dash0.com/id: a-id\nspec:\n  query: \"true\"\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "branch a commit")
	branchA = strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	runGitCmd(t, dir, "checkout", "-q", "main")
	writeFileFixture(t, dir, "assets/keep-dashboard.yaml", `apiVersion: dash0.com/v1alpha1
kind: Dashboard
metadata:
  name: keep-dashboard
spec:
  display:
    name: Keep Dashboard
`)
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "main commit")

	return dir, branchA
}

// TestApply_Since_NonAncestorRef_DeclinedDeletionDoesNotBlockCreates is a
// regression test for a bug where a declined (or unconfirmable)
// confirmation for a non-ancestor --since ref aborted the entire apply run
// before any document was processed — including ordinary creates/updates
// that have nothing to do with --since's ancestry check. The confirmation
// must gate only the deletion phase, after every other document has already
// gone through.
func TestApply_Since_NonAncestorRef_DeclinedDeletionDoesNotBlockCreates(t *testing.T) {
	testutil.SetupTestEnv(t)
	dir, branchA := setUpNonAncestorRefRepo(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, dashboardIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureDashboardsNotFound,
	})
	server.WithDashboardsCreate(testutil.FixtureDashboardsImportSuccess)
	// No DELETE route registered for views: the decline must prevent any
	// delete call from ever being attempted.

	restore := confirmation.SetReaderForTest(strings.NewReader("n\n"))
	defer restore()

	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{
		"-f", filepath.Join(dir, "assets"), "--since", branchA, "--experimental",
		"--api-url", server.URL, "--auth-token", testAuthToken,
	})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.Error(t, cmdErr, "the run must still report a non-zero exit for the skipped deletion")
	assert.Contains(t, cmdErr.Error(), "not confirmed for deletion")
	assert.Contains(t, output, "Dashboard")
	assert.Contains(t, output, "created", "the unrelated create must succeed despite the declined deletion confirmation")

	createReq := findRequest(server.Requests(), http.MethodPost, apiPathDashboards)
	require.NotNil(t, createReq, "expected the unrelated dashboard to still be created")
	assert.Nil(t, findRequest(server.Requests(), http.MethodDelete, apiPathViews), "no delete call must be attempted once the ref confirmation is declined")
}

// TestApply_Since_NonAncestorRef_NoTerminalDoesNotBlockCreates mirrors
// TestApply_Since_NonAncestorRef_DeclinedDeletionDoesNotBlockCreates for the
// no-terminal-available case (stdin closed immediately, simulating a
// non-interactive CI run without --force) — it must fail the same way a
// decline does, not hang or silently proceed with deletions.
func TestApply_Since_NonAncestorRef_NoTerminalDoesNotBlockCreates(t *testing.T) {
	testutil.SetupTestEnv(t)
	dir, branchA := setUpNonAncestorRefRepo(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, dashboardIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureDashboardsNotFound,
	})
	server.WithDashboardsCreate(testutil.FixtureDashboardsImportSuccess)

	restore := confirmation.SetReaderForTest(strings.NewReader(""))
	defer restore()

	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{
		"-f", filepath.Join(dir, "assets"), "--since", branchA, "--experimental",
		"--api-url", server.URL, "--auth-token", testAuthToken,
	})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.Error(t, cmdErr)
	assert.Contains(t, output, "Dashboard")
	assert.Contains(t, output, "created", "the unrelated create must succeed even when the deletion confirmation can't be obtained")
	assert.Nil(t, findRequest(server.Requests(), http.MethodDelete, apiPathViews), "no delete call must be attempted when confirmation can't be obtained")
}

// TestApply_Since_NonAncestorRef_ForceDeletesAndWarnsOnce is a regression
// test for a bug where the non-ancestor warning was printed twice: once as
// part of the confirmation prompt (now removed — see the 4.5 deviation note
// in tasks.md), and again unconditionally at the top of applyDeletions. With
// --force set, the prompt is skipped entirely, so this specifically checks
// that a run which actually goes on to perform the deletion still only ever
// shows the warning once.
func TestApply_Since_NonAncestorRef_ForceDeletesAndWarnsOnce(t *testing.T) {
	testutil.SetupTestEnv(t)
	dir, branchA := setUpNonAncestorRefRepo(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, dashboardIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureDashboardsNotFound,
	})
	server.WithDashboardsCreate(testutil.FixtureDashboardsImportSuccess)
	server.OnPattern(http.MethodDelete, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{
		"-f", filepath.Join(dir, "assets"), "--since", branchA, "--force", "--experimental",
		"--api-url", server.URL, "--auth-token", testAuthToken,
	})

	var cmdErr error
	var stdout string
	stderr := testutil.CaptureStderr(t, func() {
		stdout = testutil.CaptureStdout(t, func() {
			cmdErr = cmd.Execute()
		})
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, stdout, "Dashboard")
	assert.Contains(t, stdout, "created")
	assert.Contains(t, stdout, "View")
	assert.Contains(t, stdout, "deleted")
	assert.Equal(t, 1, strings.Count(stderr, "not an ancestor of HEAD"), "the non-ancestor warning must be printed exactly once, not once for the prompt and once again in applyDeletions")

	deleteReq := findRequest(server.Requests(), http.MethodDelete, "/api/views/a-id")
	require.NotNil(t, deleteReq, "expected the view removed since branchA to be deleted with --force")
}

func TestApply_Since_DeclinedDeletionFailsCommand(t *testing.T) {
	testutil.SetupTestEnv(t)

	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "dashboard.yaml", `apiVersion: dash0.com/v1alpha1
kind: Dashboard
metadata:
  name: my-dashboard
  dash0Extensions:
    id: a1b2c3d4-5678-90ab-cdef-1234567890ab
spec:
  display:
    name: My Dashboard
`)
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "add dashboard")
	before := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	require.NoError(t, os.Remove(filepath.Join(dir, "dashboard.yaml")))
	writeFileFixture(t, dir, "keep.yaml", "apiVersion: dash0.com/v1alpha1\nkind: View\nmetadata:\n  name: keep\n  labels:\n    dash0.com/id: keep-id\nspec:\n  query: \"true\"\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "remove dashboard")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureViewsNotFound,
	})
	server.WithViewsUpdate(testutil.FixtureViewsImportSuccess)
	// No DELETE route registered: the deletion must be declined before any
	// delete call is attempted.

	restore := confirmation.SetReaderForTest(strings.NewReader("n\n"))
	defer restore()

	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{
		"-f", dir, "--since", before, "--experimental",
		"--api-url", server.URL, "--auth-token", testAuthToken,
	})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "declined")
}

func TestApply_Since_DryRunPreviewDoesNotDelete(t *testing.T) {
	testutil.SetupTestEnv(t)

	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "dashboard.yaml", `apiVersion: dash0.com/v1alpha1
kind: Dashboard
metadata:
  name: my-dashboard
  dash0Extensions:
    id: a1b2c3d4-5678-90ab-cdef-1234567890ab
spec:
  display:
    name: My Dashboard
`)
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "add dashboard")
	before := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	require.NoError(t, os.Remove(filepath.Join(dir, "dashboard.yaml")))
	writeFileFixture(t, dir, "keep.yaml", "apiVersion: dash0.com/v1alpha1\nkind: View\nmetadata:\n  name: keep\n  labels:\n    dash0.com/id: keep-id\nspec:\n  query: \"true\"\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "remove dashboard")

	// No mock server at all: --dry-run --since must never make an API call.
	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{"-f", dir, "--since", before, "--dry-run", "--experimental"})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, fmt.Sprintf("pending due to --since '%s'", before))
	assert.Contains(t, output, "a1b2c3d4-5678-90ab-cdef-1234567890ab")
	assert.Contains(t, output, "Delete Dashboard")
}

// TestApply_Since_DryRunPreview_SubdirectoryScopeGroupsConsistently is a
// regression test for a bug where a -f target that is a subdirectory of the
// repo (not the repo root) rendered a deletion under its repo-root-relative
// path while validated documents rendered under their -f-target-relative
// path -- producing inconsistent, mismatched file-path prefixing in the same
// listing (e.g. "keep.yaml" next to "dashboards/removed.yaml") instead of
// both files rendering on the same (-f-target-relative) basis.
func TestApply_Since_DryRunPreview_SubdirectoryScopeGroupsConsistently(t *testing.T) {
	testutil.SetupTestEnv(t)

	repoRoot := t.TempDir()
	runGitCmd(t, repoRoot, "init", "-q", "-b", "main")
	runGitCmd(t, repoRoot, "config", "user.email", "test@example.com")
	runGitCmd(t, repoRoot, "config", "user.name", "Test")
	runGitCmd(t, repoRoot, "config", "commit.gpgsign", "false")

	writeFileFixture(t, repoRoot, "dashboards/keep.yaml", `apiVersion: dash0.com/v1alpha1
kind: Dashboard
metadata:
  name: keep-dashboard
  dash0Extensions:
    id: a1b2c3d4-5678-90ab-cdef-1234567890ab
spec:
  display:
    name: Keep Dashboard
`)
	writeFileFixture(t, repoRoot, "dashboards/removed.yaml", `apiVersion: dash0.com/v1alpha1
kind: Dashboard
metadata:
  name: removed-dashboard
  dash0Extensions:
    id: b2c3d4e5-6789-01bc-def0-234567890abc
spec:
  display:
    name: Removed Dashboard
`)
	runGitCmd(t, repoRoot, "add", "-A")
	runGitCmd(t, repoRoot, "commit", "-q", "-m", "add dashboards")
	before := strings.TrimSpace(runGitCmd(t, repoRoot, "rev-parse", "HEAD"))

	require.NoError(t, os.Remove(filepath.Join(repoRoot, "dashboards", "removed.yaml")))
	runGitCmd(t, repoRoot, "add", "-A")
	runGitCmd(t, repoRoot, "commit", "-q", "-m", "remove dashboard")

	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{"-f", filepath.Join(repoRoot, "dashboards"), "--since", before, "--dry-run", "--experimental"})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "  keep.yaml\n")
	assert.Contains(t, output, "  removed.yaml\n")
	assert.NotContains(t, output, "dashboards/keep.yaml")
	assert.NotContains(t, output, "dashboards/removed.yaml")
}

// TestApply_Since_SpamFilterIDOnlyDeletionWarns is a regression test for a
// gap where deleting a spam filter identified by dash0.com/id alone gave no
// indication that the id recorded in git history might no longer match the
// filter's actual live id (the server reassigns an ID-only filter's id on
// its first PUT — see asset.ImportSpamFilter) — the delete could silently
// miss the real live filter with no diagnostic at all.
func TestApply_Since_SpamFilterIDOnlyDeletionWarns(t *testing.T) {
	testutil.SetupTestEnv(t)

	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "filter.yaml", `apiVersion: v1alpha1
kind: Dash0SpamFilter
metadata:
  name: Drop noisy health checks
  labels:
    dash0.com/id: spam-id-only
spec:
  contexts:
    - log
  filter:
    - key: http.target
      operator: ends_with
      value: /healthz
`)
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "add spam filter")
	before := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	require.NoError(t, os.Remove(filepath.Join(dir, "filter.yaml")))
	writeFileFixture(t, dir, "keep.yaml", "apiVersion: dash0.com/v1alpha1\nkind: View\nmetadata:\n  name: keep\n  labels:\n    dash0.com/id: keep-id\nspec:\n  query: \"true\"\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "remove spam filter")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureViewsNotFound,
	})
	server.WithViewsUpdate(testutil.FixtureViewsImportSuccess)
	server.On(http.MethodDelete, "/api/spam-filters/spam-id-only", testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{
		"-f", dir, "--since", before, "--force", "--experimental",
		"--api-url", server.URL, "--auth-token", testAuthToken,
	})

	var cmdErr error
	stderr := testutil.CaptureStderr(t, func() {
		testutil.CaptureStdout(t, func() {
			cmdErr = cmd.Execute()
		})
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, stderr, "spam filter \"Drop noisy health checks\" (spam-id-only) was identified by dash0.com/id alone")
}

// TestApply_Since_SpamFilterOriginDeletionDoesNotWarn confirms the warning
// above is precise: a spam filter identified by dash0.com/origin (which is
// never reassigned server-side) must not trigger it.
func TestApply_Since_SpamFilterOriginDeletionDoesNotWarn(t *testing.T) {
	testutil.SetupTestEnv(t)

	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "filter.yaml", `apiVersion: v1alpha1
kind: Dash0SpamFilter
metadata:
  name: Drop noisy health checks
  labels:
    dash0.com/origin: spam-origin
spec:
  contexts:
    - log
  filter:
    - key: http.target
      operator: ends_with
      value: /healthz
`)
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "add spam filter")
	before := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	require.NoError(t, os.Remove(filepath.Join(dir, "filter.yaml")))
	writeFileFixture(t, dir, "keep.yaml", "apiVersion: dash0.com/v1alpha1\nkind: View\nmetadata:\n  name: keep\n  labels:\n    dash0.com/id: keep-id\nspec:\n  query: \"true\"\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "remove spam filter")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureViewsNotFound,
	})
	server.WithViewsUpdate(testutil.FixtureViewsImportSuccess)
	server.On(http.MethodDelete, "/api/spam-filters/spam-origin", testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSinceTestCmd()
	cmd.SetArgs([]string{
		"-f", dir, "--since", before, "--force", "--experimental",
		"--api-url", server.URL, "--auth-token", testAuthToken,
	})

	var cmdErr error
	stderr := testutil.CaptureStderr(t, func() {
		testutil.CaptureStdout(t, func() {
			cmdErr = cmd.Execute()
		})
	})

	require.NoError(t, cmdErr)
	assert.NotContains(t, stderr, "identified by dash0.com/id alone", "an origin-identified spam filter's id is never reassigned, so no warning is needed")
}
