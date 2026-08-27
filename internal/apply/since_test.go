package apply

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	gitutil "github.com/dash0hq/dash0-cli/internal/git"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withExperimentalFlag registers a local --experimental/-X bool flag on cmd,
// standing in for the persistent flag main.go registers on the real root
// command. Standalone tests that construct NewApplyCmd() directly (with no
// root parent) need this for experimental.RequireExperimentalFlag's
// cmd.Flags().GetBool("experimental") lookup to succeed instead of silently
// treating the flag as unregistered (=> always disabled).
func withExperimentalFlag(cmd *cobra.Command) {
	cmd.Flags().BoolP("experimental", "X", false, "Enable experimental features")
}

func TestApply_Since_RequiresExperimentalFlag(t *testing.T) {
	cmd := NewApplyCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"-f", "does-not-need-to-exist.yaml", "--since", "HEAD~1"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--since")
	assert.Contains(t, err.Error(), "--experimental")
}

func TestApply_Since_NotPassedIsUngated(t *testing.T) {
	// --since not passed at all: the gate must be a no-op, and the command
	// should fail for the ordinary "file not found" reason, never mentioning
	// --experimental.
	cmd := NewApplyCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"-f", "does-not-exist.yaml"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "--experimental")
}

func TestApply_Since_RejectsStdin(t *testing.T) {
	cmd := NewApplyCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"-f", "-", "--since", "HEAD~1", "--experimental"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stdin")
}

// TestApply_Since_ExplicitEmptyStringHitsRefEmptyError is a regression test
// for a bug where runApply gated computing the deletion plan on
// flags.Since != "", conflating "--since was never passed" with "--since was
// passed with an explicitly empty value". A CI script building
// --since="${{ github.event.before }}" can legitimately pass an empty string
// (e.g. on a workflow_dispatch/schedule trigger with no prior ref), and that
// must still surface the dedicated "no prior state to compare against"
// error, not silently fall through to an ordinary create/update apply.
func TestApply_Since_ExplicitEmptyStringHitsRefEmptyError(t *testing.T) {
	dir, _ := testSinceRepo(t)

	cmd := NewApplyCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"-f", dir, "--since", "", "--experimental"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no prior state to compare against")
}

// testSinceRepo creates a temp git repo with a dashboard file at ref
// "before", then removes it in a later commit ("after" / HEAD), returning
// the repo directory and the "before" ref's SHA.
func testSinceRepo(t *testing.T) (dir, beforeSHA string) {
	t.Helper()
	dir = t.TempDir()
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
	beforeSHA = strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	require.NoError(t, os.Remove(filepath.Join(dir, "dashboard.yaml")))
	// Leave an unrelated file so the directory isn't empty for readDirectory.
	writeFileFixture(t, dir, "unrelated.yaml", "kind: View\nmetadata:\n  name: unrelated\n  labels:\n    dash0.com/id: unrelated-id\nspec:\n  query: \"true\"\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "remove dashboard")

	return dir, beforeSHA
}

func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, out)
	return string(out)
}

func writeFileFixture(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

func TestComputeDeletionPlan_WholeFileDeletion(t *testing.T) {
	dir, before := testSinceRepo(t)

	flags := &applyFlags{File: dir, Since: before}
	dp, err := computeDeletionPlan(context.Background(), flags)
	require.NoError(t, err)
	require.Len(t, dp.plan.ByIdentifier, 1)
	deletion := dp.plan.ByIdentifier[0]
	assert.Equal(t, "dashboard", deletion.Kind)
	assert.Equal(t, "a1b2c3d4-5678-90ab-cdef-1234567890ab", deletion.Identifier)
	assert.Empty(t, dp.warning)
	// dp.names is resolved from git history at the --since ref, not from
	// current disk contents (the file no longer exists on disk).
	assert.Equal(t, "My Dashboard", dp.names[deletion.Path])
}

// TestComputeDeletionPlan_SingleFileTargetStillExists is a regression test
// for a bug where -f pointing directly at a single file (rather than a
// directory) that still exists on disk made every --since invocation fail
// outright: computeDeletionPlan derived repoDir from the file's own path
// without ever taking its parent directory, so the subsequent `git -C
// <repoDir> rev-parse --show-toplevel` failed with "fatal: cannot change to
// '<file>': Not a directory" -- contradicting docs/commands.md's own claim
// that "-f's target must be inside a git repository (a single file or a
// directory both work)".
func TestComputeDeletionPlan_SingleFileTargetStillExists(t *testing.T) {
	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	target := filepath.Join(dir, "dashboard.yaml")
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

	flags := &applyFlags{File: target, Since: before}
	dp, err := computeDeletionPlan(context.Background(), flags)
	require.NoError(t, err, "a single-file -f target that still exists must not fail to locate its repository")
	assert.True(t, dp.plan.IsEmpty(), "the file is unchanged since before, so there is nothing to delete")
}

// testSinceRepoAllDeleted creates a temp git repo with two asset files at ref
// "before", then removes both of them (and nothing else) in a later commit,
// leaving the -f target directory itself still present on disk but with zero
// eligible YAML files -- the "all files deleted" scenario, as opposed to
// testSinceRepo's "one file survives" scenario.
func testSinceRepoAllDeleted(t *testing.T) (dir, beforeSHA string) {
	t.Helper()
	dir = t.TempDir()
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
	beforeSHA = strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	require.NoError(t, os.Remove(filepath.Join(dir, "dashboard.yaml")))
	require.NoError(t, os.Remove(filepath.Join(dir, "view.yaml")))
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "remove both")

	return dir, beforeSHA
}

// TestComputeDeletionPlan_AllFilesDeleted is a regression test for a bug
// where --since silently found nothing to delete once every asset
// definition under -f's target was removed and the (now-empty) directory
// itself survived: computeDeletionPlan itself has always tolerated an empty
// disk-side scope fine (BuildSnapshotFromDisk over an empty directory finds
// nothing to ingest, which is not an error) -- the actual bug lived one layer
// up, in runApply's directory-discovery step (readDirectory), which hard-
// failed with "no .yaml or .yml files found" before computeDeletionPlan ever
// ran. This test pins computeDeletionPlan's own (already-correct) contract;
// TestApply_Since_AllFilesDeleted_DirectorySurvives in
// since_integration_test.go covers the full runApply path that used to fail.
func TestComputeDeletionPlan_AllFilesDeleted(t *testing.T) {
	dir, before := testSinceRepoAllDeleted(t)

	flags := &applyFlags{File: dir, Since: before}
	dp, err := computeDeletionPlan(context.Background(), flags)
	require.NoError(t, err)
	require.Len(t, dp.plan.ByIdentifier, 2)

	identifiers := []string{dp.plan.ByIdentifier[0].Identifier, dp.plan.ByIdentifier[1].Identifier}
	assert.ElementsMatch(t, []string{"a1b2c3d4-5678-90ab-cdef-1234567890ab", "b2c3d4e5-6789-01bc-def0-234567890abc"}, identifiers)
	assert.Empty(t, dp.warning)
}

// TestComputeDeletionPlan_TargetDirectoryRemoved is a regression test for a
// bug where computeDeletionPlan itself couldn't run at all once the -f
// target directory was removed entirely (not just emptied): filepath.
// EvalSymlinks and os.Stat both require the path to exist, and both were
// called directly on the target before this fix. This exercises the case
// where the target is a subdirectory of the repo (not the repo root itself,
// which can never be "removed" while still being a git worktree) that no
// longer exists on disk at all.
func TestComputeDeletionPlan_TargetDirectoryRemoved(t *testing.T) {
	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "dashboards/dashboard.yaml", `apiVersion: dash0.com/v1alpha1
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

	target := filepath.Join(dir, "dashboards")
	require.NoError(t, os.RemoveAll(target))
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "remove dashboards directory entirely")

	flags := &applyFlags{File: target, Since: before}
	dp, err := computeDeletionPlan(context.Background(), flags)
	require.NoError(t, err)
	require.Len(t, dp.plan.ByIdentifier, 1)
	assert.Equal(t, "a1b2c3d4-5678-90ab-cdef-1234567890ab", dp.plan.ByIdentifier[0].Identifier)
	assert.Empty(t, dp.warning)
}

// TestComputeDeletionPlan_DanglingSymlinkTargetIsTreatedAsMissing is a
// regression test for a bug where a --since target that is a dangling
// symlink (the symlink itself exists, but whatever it points at doesn't)
// was treated inconsistently across the three places that ask "does -f's
// target exist": runApply's os.Stat(flags.File) and BuildSnapshotFromDisk's
// os.Stat(scope) both follow symlinks, so both see it as "gone" and route
// to the all-deletions-tolerant path -- but nearestExistingAncestor used
// os.Lstat, which sees the dangling symlink itself as "existing" and stops
// there, so computeDeletionPlan then failed outright trying to resolve it
// as a real path instead of taking the same tolerant path the other two
// checks would.
func TestComputeDeletionPlan_DanglingSymlinkTargetIsTreatedAsMissing(t *testing.T) {
	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "dashboards/dashboard.yaml", `apiVersion: dash0.com/v1alpha1
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

	target := filepath.Join(dir, "dashboards")
	require.NoError(t, os.RemoveAll(target))
	// A dangling symlink at the target's own path -- the symlink itself
	// exists, but its destination never did.
	require.NoError(t, os.Symlink(filepath.Join(dir, "never-existed"), target))
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "replace dashboards with a dangling symlink")

	flags := &applyFlags{File: target, Since: before}
	dp, err := computeDeletionPlan(context.Background(), flags)
	require.NoError(t, err, "a dangling symlink target must be treated the same as a missing one, not fail to resolve")
	require.Len(t, dp.plan.ByIdentifier, 1)
	assert.Equal(t, "a1b2c3d4-5678-90ab-cdef-1234567890ab", dp.plan.ByIdentifier[0].Identifier)
}

// TestComputeDeletionPlan_SubdirectoryRenamedWithinScope pins the documented
// contract that deletion detection is by identifier, never by file path
// (see "Deletion detection is by identifier" in docs/commands.md's --since
// section): renaming a subdirectory *within* -f's scanned scope must not be
// reported as a deletion, since the asset's identifier survives under the
// new path. TestDiff_NoChangeWhenIdentifierSurvives in internal/git/diff_test.go
// already pins this at the pure Snapshot/Diff level with hand-built
// Snapshots; this test exercises the same contract through computeDeletionPlan
// end to end, against a real git repo with an actual `git mv` of a directory
// (not just a single file), which is what a user restructuring a dashboards/
// tree by team or environment would actually do.
func TestComputeDeletionPlan_SubdirectoryRenamedWithinScope(t *testing.T) {
	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "dashboards/team-a/dashboard.yaml", `apiVersion: dash0.com/v1alpha1
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
	runGitCmd(t, dir, "commit", "-q", "-m", "add dashboard under team-a")
	before := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	// -f itself (the "dashboards" directory below) is unaffected by this
	// rename -- only the subdirectory nested inside it moves.
	runGitCmd(t, dir, "mv", "dashboards/team-a", "dashboards/team-b")
	runGitCmd(t, dir, "commit", "-q", "-m", "rename team-a to team-b")

	flags := &applyFlags{File: filepath.Join(dir, "dashboards"), Since: before}
	dp, err := computeDeletionPlan(context.Background(), flags)
	require.NoError(t, err)
	assert.True(t, dp.plan.IsEmpty(), "renaming a subdirectory within the scanned scope must not be reported as a deletion")
	assert.Empty(t, dp.warning)
}

// TestComputeDeletionPlan_TargetItselfRenamedIsReportedAsDeletion documents
// the necessary counterpart to
// TestComputeDeletionPlan_SubdirectoryRenamedWithinScope: identifier-based
// matching only reaches as far as -f's own scope. If -f's *own* target
// directory is what gets renamed (as opposed to something nested inside it)
// and the caller keeps pointing -f at the old, now-gone path, every asset
// that used to live there is correctly reported as deleted -- from that
// fixed scope's perspective, it genuinely no longer has anything, the exact
// case TestComputeDeletionPlan_TargetDirectoryRemoved and
// TestComputeDeletionPlan_AllFilesDeleted exist to detect. Re-pointing -f at
// the new path instead (not exercised here) reports zero deletions, since
// the "before" snapshot for that new scope has nothing to diff against --
// the identifier simply becomes a new create/update, business as usual.
func TestComputeDeletionPlan_TargetItselfRenamedIsReportedAsDeletion(t *testing.T) {
	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "dashboards/dashboard.yaml", `apiVersion: dash0.com/v1alpha1
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

	runGitCmd(t, dir, "mv", "dashboards", "dashboards-v2")
	runGitCmd(t, dir, "commit", "-q", "-m", "rename dashboards to dashboards-v2")

	// -f still names the old path, which the rename left nonexistent.
	flags := &applyFlags{File: filepath.Join(dir, "dashboards"), Since: before}
	dp, err := computeDeletionPlan(context.Background(), flags)
	require.NoError(t, err)
	require.Len(t, dp.plan.ByIdentifier, 1)
	assert.Equal(t, "a1b2c3d4-5678-90ab-cdef-1234567890ab", dp.plan.ByIdentifier[0].Identifier)
	assert.Empty(t, dp.warning)
}

// TestSplitMultiDocPath is a table test for the "#<index>" suffix
// internal/git/snapshot.go appends to the second and later documents' paths
// in a multi-document file.
func TestSplitMultiDocPath(t *testing.T) {
	cases := []struct {
		path         string
		wantBase     string
		wantDocIndex int
	}{
		{"assets.yaml", "assets.yaml", 0},
		{"assets.yaml#1", "assets.yaml", 1},
		{"assets.yaml#12", "assets.yaml", 12},
		// A literal "#" not followed by digits is not a multi-document
		// suffix -- treat the whole string as the path.
		{"weird#name.yaml", "weird#name.yaml", 0},
	}
	for _, c := range cases {
		base, idx := splitMultiDocPath(c.path)
		assert.Equal(t, c.wantBase, base, "path %q", c.path)
		assert.Equal(t, c.wantDocIndex, idx, "path %q", c.path)
	}
}

// TestCheckRuleIDsOccupiedByCRD_SingleAlertUsesLiteralIdentifier pins that a
// CRD with zero or one alerting rule keeps using the CRD's own literal
// identifier, unchanged: composePrometheusRuleNames never derives a distinct
// id for it either, so it lives at that literal id.
func TestCheckRuleIDsOccupiedByCRD_SingleAlertUsesLiteralIdentifier(t *testing.T) {
	assert.Equal(t, []string{"shared-id"}, asset.CheckRuleIDsOccupiedByCRD("shared-id", nil))
	assert.Equal(t, []string{"shared-id"}, asset.CheckRuleIDsOccupiedByCRD("shared-id", []asset.PrometheusAlertName{
		{GroupName: "g", AlertName: "A"},
	}))
}

// TestCheckRuleIDsOccupiedByCRD_MultiAlertUsesDerivedIDs is a regression
// test for a bug where a whole-CRD deletion for a multi-alert CRD only ever
// targeted the CRD's literal identifier, which -- once
// composePrometheusRuleNames derives a distinct id per alert for 2+ alerts
// -- was never where any of the real check rules actually lived. Each
// alert's own derived id must be targeted instead.
func TestCheckRuleIDsOccupiedByCRD_MultiAlertUsesDerivedIDs(t *testing.T) {
	ids := asset.CheckRuleIDsOccupiedByCRD("shared-id", []asset.PrometheusAlertName{
		{GroupName: "g", AlertName: "A"},
		{GroupName: "g", AlertName: "B"},
	})
	assert.Equal(t, []string{
		asset.DeriveAlertCheckRuleID("shared-id", "g - A"),
		asset.DeriveAlertCheckRuleID("shared-id", "g - B"),
	}, ids)
	assert.NotContains(t, ids, "shared-id", "a multi-alert CRD's alerts never live at the literal shared id")
}

// TestResolveDeletionNames_MultiDocumentFile is a regression test for a bug
// where a deletion candidate from the second (or later) document in a
// multi-document file failed to resolve a name at all, because its Path
// carries a "#<index>" suffix that doesn't match any real git blob path —
// resolveDeletionNames must strip the suffix to read the file, then use the
// index to pick the right document out of the file's content.
func TestResolveDeletionNames_MultiDocumentFile(t *testing.T) {
	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "assets.yaml", `apiVersion: dash0.com/v1alpha1
kind: View
metadata:
  name: first-view
  labels:
    dash0.com/id: view-id
spec:
  query: "true"
---
apiVersion: dash0.com/v1alpha1
kind: CheckRule
id: rule-id
name: Second Document Rule
expression: up == 0
`)
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "seed")

	repo := gitutil.Repo{Dir: dir}
	before, err := gitutil.BuildSnapshotFromRef(context.Background(), repo, "HEAD", "")
	require.NoError(t, err)

	deletions := []gitutil.Deletion{
		{Kind: "view", Identifier: "view-id", Path: "assets.yaml"},
		{Kind: "checkrule", Identifier: "rule-id", Path: "assets.yaml#1"},
	}
	names := resolveDeletionNames(before, deletions)
	assert.Equal(t, "first-view", names["assets.yaml"])
	assert.Equal(t, "Second Document Rule", names["assets.yaml#1"])
}

// TestResolveDeletionNames_ReusesBeforeSnapshotContent is a regression test
// pinning that resolveDeletionNames never reads git again: it must resolve
// every name from before.RawContent, which BuildSnapshotFromRef already
// populated while building the "before" snapshot. Deleting the git repo
// entirely (but keeping before, already built beforehand) proves this --
// a version of resolveDeletionNames that shelled out to git a second time
// would find nothing to read and return an empty map, not the real names.
func TestResolveDeletionNames_ReusesBeforeSnapshotContent(t *testing.T) {
	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")
	writeFileFixture(t, dir, "dashboard.yaml", `kind: Dashboard
metadata:
  name: my-dashboard
  dash0Extensions:
    id: gone-id
spec:
  display:
    name: My Dashboard
`)
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "seed")

	repo := gitutil.Repo{Dir: dir}
	before, err := gitutil.BuildSnapshotFromRef(context.Background(), repo, "HEAD", "")
	require.NoError(t, err)

	// The repository (and thus any chance of a second git read succeeding)
	// is gone by the time resolveDeletionNames runs.
	require.NoError(t, os.RemoveAll(dir))

	deletions := []gitutil.Deletion{
		{Kind: "dashboard", Identifier: "gone-id", Path: "dashboard.yaml"},
	}
	names := resolveDeletionNames(before, deletions)
	assert.Equal(t, "My Dashboard", names["dashboard.yaml"])
}

// TestResolveDeletionNames_MissingRawContentIsNonFatal is a regression test
// pinning that a deletion candidate whose path was never captured in
// before.RawContent (e.g. a since-rewritten blob a differently-scoped
// snapshot never read) only omits that entry from the returned map -- it
// must never panic or error the caller, since name resolution is display
// polish, not something --since's actual deletion dispatch depends on.
func TestResolveDeletionNames_MissingRawContentIsNonFatal(t *testing.T) {
	before := gitutil.Snapshot{RawContent: map[string][]byte{}}
	deletions := []gitutil.Deletion{
		{Kind: "dashboard", Identifier: "gone-id", Path: "never-captured.yaml"},
	}
	names := resolveDeletionNames(before, deletions)
	assert.Empty(t, names)
}

func TestComputeDeletionPlan_EmptyRef(t *testing.T) {
	dir, _ := testSinceRepo(t)
	flags := &applyFlags{File: dir, Since: ""}
	_, err := computeDeletionPlan(context.Background(), flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty ref")
}

func TestComputeDeletionPlan_AllZerosRef(t *testing.T) {
	dir, _ := testSinceRepo(t)
	flags := &applyFlags{File: dir, Since: "0000000000000000000000000000000000000000"}
	_, err := computeDeletionPlan(context.Background(), flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all-zeros")
}

func TestComputeDeletionPlan_UnresolvableRef(t *testing.T) {
	dir, _ := testSinceRepo(t)
	flags := &applyFlags{File: dir, Since: "totally-bogus-ref"}
	_, err := computeDeletionPlan(context.Background(), flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not be resolved")
	assert.Contains(t, err.Error(), "\nHint: check the ref for a typo", "a genuine typo must still get the generic suggestions, as an agent-mode-parseable hint")
}

// TestComputeDeletionPlan_NotAGitRepository is a regression test for a bug
// where running --since against a directory that was never a git
// repository at all produced a deeply nested, hard-to-read error: "--since
// '<ref>' requires <dir> to be inside a git repository: failed to
// determine repository root for <dir>: git rev-parse --show-toplevel: exit
// status 128 (stderr: fatal: not a git repository (or any of the parent
// directories): .git)". The common case now gets one clean sentence
// instead of three layers of wrapped git plumbing errors.
func TestComputeDeletionPlan_NotAGitRepository(t *testing.T) {
	dir := t.TempDir() // deliberately never `git init`-ed
	flags := &applyFlags{File: dir, Since: "HEAD~1"}
	_, err := computeDeletionPlan(context.Background(), flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "to be inside a git repository, but it is not")
	assert.NotContains(t, err.Error(), "failed to determine repository root", "the nested git-plumbing wrapping must not leak into the message")
	assert.NotContains(t, err.Error(), "exit status", "the raw git exec error must not leak into the message")
}

// TestComputeDeletionPlan_InsufficientHistory is a regression test for a
// bug where --since HEAD~1 against a fresh, single-commit repository --
// the first thing many people try when setting up a --since test or demo
// repo -- was told to check for a typo or a too-shallow clone, neither of
// which applies: there is simply no earlier commit yet.
func TestComputeDeletionPlan_InsufficientHistory(t *testing.T) {
	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")
	writeFileFixture(t, dir, "keep.yaml", "apiVersion: dash0.com/v1alpha1\nkind: View\nmetadata:\n  name: keep\n  labels:\n    dash0.com/id: keep-id\nspec:\n  query: \"true\"\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "only commit")

	flags := &applyFlags{File: dir, Since: "HEAD~1"}
	_, err := computeDeletionPlan(context.Background(), flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1 commit")
	assert.NotContains(t, err.Error(), "check the ref for a typo", "the generic suggestions don't apply here and must not be shown")
}

func TestComputeDeletionPlan_NoIdentifierHardFails(t *testing.T) {
	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "orphan.yaml", "kind: Dashboard\nmetadata:\n  name: no-id\n")
	writeFileFixture(t, dir, "keep.yaml", "kind: View\nmetadata:\n  name: keep\n  labels:\n    dash0.com/id: keep-id\nspec:\n  query: \"true\"\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "add files")
	before := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	require.NoError(t, os.Remove(filepath.Join(dir, "orphan.yaml")))
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "remove orphan")

	flags := &applyFlags{File: dir, Since: before}
	_, err := computeDeletionPlan(context.Background(), flags)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no dash0.com/id or dash0.com/origin label")
	assert.Contains(t, err.Error(), "orphan.yaml")
}

// TestComputeDeletionPlan_NonAncestorRef_NeverPromptsOrErrors documents
// computeDeletionPlan's contract after the fix for a bug where its own
// confirmation prompt for a non-ancestor ref aborted the entire apply run —
// including ordinary creates/updates unrelated to --since — before any
// document was even processed. computeDeletionPlan itself no longer prompts
// at all: it always returns the plan plus a warning for the caller to act
// on. The confirmation now lives in runApply, gated to run only right before
// the deletion phase, after every other document has already been applied —
// see TestApply_Since_NonAncestorRef_DeclinedDeletionDoesNotBlockCreates and
// TestApply_Since_NonAncestorRef_NoTerminalDoesNotBlockCreates in
// since_integration_test.go for that behavior.
//
// No reader override is installed here: if computeDeletionPlan tried to
// prompt, reading from the real os.Stdin in a test process would hang or
// fail — getting NoError without one is exactly what proves it never does.
func TestComputeDeletionPlan_NonAncestorRef_NeverPromptsOrErrors(t *testing.T) {
	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")
	runGitCmd(t, dir, "commit", "-q", "--allow-empty", "-m", "initial")

	runGitCmd(t, dir, "checkout", "-q", "-b", "branch-a")
	writeFileFixture(t, dir, "a.yaml", "kind: View\nmetadata:\n  name: a\n  labels:\n    dash0.com/id: a-id\nspec:\n  query: \"true\"\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "branch a commit")
	branchA := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	runGitCmd(t, dir, "checkout", "-q", "main")
	writeFileFixture(t, dir, "b.yaml", "kind: View\nmetadata:\n  name: b\n  labels:\n    dash0.com/id: b-id\nspec:\n  query: \"true\"\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "main commit")

	flags := &applyFlags{File: dir, Since: branchA}
	dp, err := computeDeletionPlan(context.Background(), flags)
	require.NoError(t, err)
	assert.Contains(t, dp.warning, "not an ancestor of HEAD")
}

// TestApplyDeletions_PrometheusRuleConfirmationPromptUsesConsistentCasing is
// a regression test for a bug where applyDeletions lowercased
// asset.KindDisplayName's entire output (strings.ToLower) specifically for
// the confirmation prompt text (printed to stdout by
// confirmation.ConfirmDestructiveOperation via fmt.Print, not the "declined"
// message on stderr, which was never affected), mangling a compound proper
// noun like "PrometheusRule" into unreadable "prometheusrule" — while the
// success message for the exact same asset used the correctly-cased
// "PrometheusRule". No API call happens on the declined path, so this
// exercises the message text directly without needing a mock server.
func TestApplyDeletions_PrometheusRuleConfirmationPromptUsesConsistentCasing(t *testing.T) {
	restore := confirmation.SetReaderForTest(strings.NewReader("n\n"))
	defer restore()

	dp := &deletionPlan{
		plan: gitutil.DeletionPlan{
			ByIdentifier: []gitutil.Deletion{
				{Kind: "prometheusrule", Identifier: "shared-id"},
			},
		},
	}

	stdout := testutil.CaptureStdout(t, func() {
		declined, err := applyDeletions(context.Background(), nil, nil, dp, false)
		require.NoError(t, err)
		assert.Equal(t, 1, declined)
	})

	assert.Contains(t, stdout, "Are you sure you want to delete PrometheusRule \"<name>\" (shared-id)")
	assert.NotContains(t, stdout, "prometheusrule", "the whole display name must never be force-lowercased into an unreadable compound word")
}

// TestCheckRuleNameIndex_Resolve covers resolve's three outcomes. Ambiguity
// matters most: the previous first-exact-match lookup silently deleted
// whichever check rule the API happened to list first.
func TestCheckRuleNameIndex_Resolve(t *testing.T) {
	const name = "group - Alert"

	t.Run("single deletable match resolves", func(t *testing.T) {
		index := checkRuleNameIndex{name: {{id: "id-1"}}}
		id, err := index.resolve(name)
		require.NoError(t, err)
		assert.Equal(t, "id-1", id)
	})

	t.Run("no match resolves to empty, not an error", func(t *testing.T) {
		id, err := checkRuleNameIndex{}.resolve(name)
		require.NoError(t, err)
		assert.Empty(t, id, "an absent check rule is already in the desired end state")
	})

	t.Run("two deletable matches error instead of guessing", func(t *testing.T) {
		index := checkRuleNameIndex{name: {{id: "id-2"}, {id: "id-1"}}}
		_, err := index.resolve(name)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ambiguous")
		assert.Contains(t, err.Error(), "id-1, id-2", "both candidates must be named, sorted")
		assert.Contains(t, err.Error(), "\nHint:")
	})

	t.Run("a foreign-owned match is not deletable", func(t *testing.T) {
		for _, source := range []string{"ui", "terraform", "operator", "platform"} {
			index := checkRuleNameIndex{name: {{id: "id-1", source: source}}}
			id, err := index.resolve(name)
			require.NoError(t, err)
			assert.Empty(t, id, "a check rule managed by %s is a different asset that merely collides", source)
		}
	})

	t.Run("a foreign match does not make its deletable sibling ambiguous", func(t *testing.T) {
		index := checkRuleNameIndex{name: {{id: "id-terraform", source: "terraform"}, {id: "id-1"}}}
		id, err := index.resolve(name)
		require.NoError(t, err)
		assert.Equal(t, "id-1", id)
	})

	t.Run("api, dash0-cli and an unrecognized source stay deletable", func(t *testing.T) {
		// CrdSource's contract is to treat an unknown value as "api", and a
		// CLI-applied check rule carries no origin of its own at all.
		for _, source := range []string{"", "api", "dash0-cli", "something-new"} {
			index := checkRuleNameIndex{name: {{id: "id-1", source: source}}}
			id, err := index.resolve(name)
			require.NoError(t, err)
			assert.Equal(t, "id-1", id, "source %q must not be treated as foreign", source)
		}
	})
}

// TestComputeDeletionPlan_SparseCheckoutRefuses is a regression test for
// silent data loss: the ref side of the diff comes from `git ls-tree`, which
// enumerates the whole commit, while the disk side walks only what is
// materialized. Under a sparse checkout every tracked-but-absent asset became
// a deletion candidate, and --force deleted assets git still declares.
func TestComputeDeletionPlan_SparseCheckoutRefuses(t *testing.T) {
	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")
	writeFileFixture(t, dir, "keep.yaml", "kind: View\nmetadata:\n  name: keep\n  labels:\n    dash0.com/id: keep-id\nspec:\n  query: \"true\"\n")
	writeFileFixture(t, dir, "hidden.yaml", "kind: View\nmetadata:\n  name: hidden\n  labels:\n    dash0.com/id: hidden-id\nspec:\n  query: \"true\"\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "add both")
	before := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))
	runGitCmd(t, dir, "update-index", "--skip-worktree", "hidden.yaml")
	require.NoError(t, os.Remove(filepath.Join(dir, "hidden.yaml")))

	_, err := computeDeletionPlan(context.Background(), &applyFlags{File: dir, Since: before})
	require.Error(t, err, "hidden.yaml is still declared in git and must never become a deletion candidate")
	assert.Contains(t, err.Error(), "sparse checkout")
	assert.Contains(t, err.Error(), "\nHint:")
}
