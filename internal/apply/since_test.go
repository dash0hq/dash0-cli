package apply

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
	deletions := []gitutil.Deletion{
		{Kind: "view", Identifier: "view-id", Path: "assets.yaml"},
		{Kind: "checkrule", Identifier: "rule-id", Path: "assets.yaml#1"},
	}
	names := resolveDeletionNames(context.Background(), repo, "HEAD", deletions)
	assert.Equal(t, "first-view", names["assets.yaml"])
	assert.Equal(t, "Second Document Rule", names["assets.yaml#1"])
}

// TestResolveDeletionNames_LookupFailureIsNonFatal is a regression test
// pinning that a git-read failure (an unresolvable ref, a path that never
// existed) only omits that entry from the returned map -- it must never
// panic or error the caller, since name resolution is display polish, not
// something --since's actual deletion dispatch depends on.
func TestResolveDeletionNames_LookupFailureIsNonFatal(t *testing.T) {
	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")
	writeFileFixture(t, dir, "placeholder.yaml", "kind: View\nmetadata:\n  name: x\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "seed")

	repo := gitutil.Repo{Dir: dir}
	deletions := []gitutil.Deletion{
		{Kind: "dashboard", Identifier: "gone-id", Path: "never-existed.yaml"},
	}
	names := resolveDeletionNames(context.Background(), repo, "HEAD", deletions)
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

