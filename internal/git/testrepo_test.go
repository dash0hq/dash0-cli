package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// testRepo creates a real temporary git repository with an initial empty
// commit, so HEAD always resolves. It returns a Repo pointed at it.
//
// This is an interim measure: once the checked-in zipped git-scenario
// fixtures (see openspec/changes/add-diff-and-since-flag/tasks.md, section
// 2) exist, these tests should migrate to testutil.UnzipGitScenario so every
// test tier shares one canonical repo state per scenario instead of building
// ad hoc repos inline.
func testRepo(t *testing.T) Repo {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	// This is the test's own throwaway repo (t.TempDir()), not the user's
	// real repo or global git config: disable commit signing locally so
	// tests don't depend on the machine's signing setup (e.g. a
	// passphrase-protected SSH key with commit.gpgsign=true globally).
	runGit(t, dir, "config", "commit.gpgsign", "false")
	runGit(t, dir, "commit", "-q", "--allow-empty", "-m", "initial commit")
	return Repo{Dir: dir}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, out)
	return strings.TrimSpace(string(out))
}

// writeFile writes content to a repo-relative path inside dir, creating
// parent directories as needed.
func writeFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

// commitAll stages every change in dir and commits it.
func commitAll(t *testing.T, dir, message string) string {
	t.Helper()
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", message)
	return runGit(t, dir, "rev-parse", "HEAD")
}
