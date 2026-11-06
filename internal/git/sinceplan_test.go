package git

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStripScope is a table test for the deletion-path/validated-document
// path basis mismatch under a subdirectory -f target: Deletion.Path
// is repo-root-relative, asset.Document.filePath is -f-target-relative, and
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
		assert.Equal(t, c.want, StripScope(c.path, c.scope), "path %q scope %q", c.path, c.scope)
	}
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
		base, idx := SplitMultiDocPath(c.path)
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
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")

	writeFile(t, dir, "assets.yaml", `apiVersion: dash0.com/v1alpha1
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
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "seed")

	repo := Repo{Dir: dir}
	before, err := BuildSnapshotFromRef(context.Background(), repo, "HEAD", "")
	require.NoError(t, err)

	deletions := []Deletion{
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
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	writeFile(t, dir, "dashboard.yaml", `kind: Dashboard
metadata:
  name: my-dashboard
  dash0Extensions:
    id: gone-id
spec:
  display:
    name: My Dashboard
`)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "seed")

	repo := Repo{Dir: dir}
	before, err := BuildSnapshotFromRef(context.Background(), repo, "HEAD", "")
	require.NoError(t, err)

	// The repository (and thus any chance of a second git read succeeding)
	// is gone by the time resolveDeletionNames runs.
	require.NoError(t, os.RemoveAll(dir))

	deletions := []Deletion{
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
	before := Snapshot{RawContent: map[string][]byte{}}
	deletions := []Deletion{
		{Kind: "dashboard", Identifier: "gone-id", Path: "never-captured.yaml"},
	}
	names := resolveDeletionNames(before, deletions)
	assert.Empty(t, names)
}
