package testutil

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// GitScenariosDir returns the absolute path to the git-scenarios fixtures
// directory.
func GitScenariosDir() string {
	return filepath.Join(FixturesDir(), "git-scenarios")
}

// GitRepoFixture declaratively describes a git repository's history for a
// `--since` test scenario. See internal/testutil/fixtures/git-scenarios/
// for examples, and BuildGitScenario for how one is turned into a real repo.
type GitRepoFixture struct {
	Kind string             `yaml:"kind"`
	Spec GitRepoFixtureSpec `yaml:"spec"`
}

// GitRepoFixtureSpec is a GitRepoFixture's body.
type GitRepoFixtureSpec struct {
	// SinceRef is the --since value this scenario is designed to be tested
	// with: either the Label of one of Repo.Commits, or a literal ref value
	// (e.g. git's all-zeros sentinel for a "first push" scenario) used
	// as-is when it doesn't match any commit's label.
	SinceRef string  `yaml:"sinceRef"`
	Repo     GitRepo `yaml:"repo"`
}

// GitRepo is an ordered list of commits to apply to a fresh repository.
type GitRepo struct {
	Commits []GitCommit `yaml:"commits"`
}

// GitCommit describes one commit: an ordered list of file changes applied
// to the working tree, then committed.
type GitCommit struct {
	// Label optionally names this commit so SinceRef or a later commit's
	// ResetTo can refer back to it.
	Label string `yaml:"label,omitempty"`
	// ResetTo, if set, hard-resets the working tree to the labeled commit
	// before applying this commit's Changes -- simulating a force-push/
	// history rewrite that leaves earlier commits orphaned (still
	// resolvable by SHA, but no longer an ancestor of HEAD).
	ResetTo string      `yaml:"resetTo,omitempty"`
	Message string      `yaml:"message"`
	Changes []GitChange `yaml:"changes,omitempty"`
}

// GitChangeOp names what a GitChange does to a file.
type GitChangeOp string

const (
	GitChangeAdd    GitChangeOp = "add"
	GitChangeModify GitChangeOp = "modify"
	GitChangeDelete GitChangeOp = "delete"
)

// GitChange is one file-level change within a commit. Op is explicit rather
// than inferred (e.g. from the same Name reappearing with different Content
// in a later commit) so a fixture's history reads correctly on its own,
// without cross-referencing earlier commits, and so BuildGitScenario can
// catch an inconsistent fixture (e.g. "add" on a file that already exists)
// at build time.
type GitChange struct {
	Op   GitChangeOp `yaml:"op"`
	Name string      `yaml:"name"`
	// Content is the file's new full content. Required for "add" and
	// "modify"; must be empty for "delete".
	Content string `yaml:"content,omitempty"`
}

// BuildGitScenario reads the named git-scenario fixture
// (internal/testutil/fixtures/git-scenarios/<name>.yml) and replays its
// commit history into a fresh repository under t.TempDir(). It returns the
// repository directory and the ref the scenario wants tests to pass as
// --since.
//
// This is the one thing every test tier (unit, integration, e2e) calls — no
// tier re-derives or hand-rolls its own git repo setup, so all three build
// the exact same repo history per scenario. Building the repo fresh from a
// declarative description (rather than unpacking a checked-in binary
// artifact) keeps each scenario's history readable and diffable in review,
// with nothing to regenerate when a scenario's shape changes.
func BuildGitScenario(t *testing.T, name string) (repoDir, ref string) {
	t.Helper()

	fixturePath := filepath.Join(GitScenariosDir(), name+".yml")
	data, err := os.ReadFile(fixturePath)
	require.NoErrorf(t, err, "failed to read git scenario fixture %s", fixturePath)

	var fixture GitRepoFixture
	require.NoErrorf(t, yaml.Unmarshal(data, &fixture), "failed to parse git scenario fixture %s", fixturePath)
	require.Equalf(t, "GitRepoFixture", fixture.Kind, "%s: unexpected kind %q", fixturePath, fixture.Kind)

	repoDir = filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))

	runGit(t, repoDir, "init", "-q", "-b", "main")
	runGit(t, repoDir, "config", "user.email", "fixtures@dash0.example")
	runGit(t, repoDir, "config", "user.name", "Dash0 Fixture")
	// This is the scenario's own throwaway repo (under t.TempDir()), not the
	// developer's real repo or its global git config.
	runGit(t, repoDir, "config", "commit.gpgsign", "false")

	labels := map[string]string{}
	// existingFiles tracks which file names are present in the working tree
	// as commits are replayed, so an "add"/"modify"/"delete" that doesn't
	// match reality (e.g. "add" on a file that's already there) fails loudly
	// here rather than silently doing the wrong thing. fileSets snapshots
	// this set at each labeled commit, so a later ResetTo restores the set
	// as it stood then, matching the real `git reset --hard`.
	existingFiles := map[string]bool{}
	fileSets := map[string]map[string]bool{}

	for _, commit := range fixture.Spec.Repo.Commits {
		if commit.ResetTo != "" {
			sha, ok := labels[commit.ResetTo]
			require.Truef(t, ok, "%s: commit %q resets to unknown label %q", fixturePath, commit.Message, commit.ResetTo)
			runGit(t, repoDir, "reset", "-q", "--hard", sha)
			existingFiles = cloneFileSet(fileSets[commit.ResetTo])
		}

		for _, change := range commit.Changes {
			target := filepath.Join(repoDir, change.Name)
			switch change.Op {
			case GitChangeAdd:
				require.Falsef(t, existingFiles[change.Name], "%s: commit %q: op %q on %q, which already exists (use %q?)", fixturePath, commit.Message, change.Op, change.Name, GitChangeModify)
				require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
				require.NoError(t, os.WriteFile(target, []byte(change.Content), 0o644))
				existingFiles[change.Name] = true
			case GitChangeModify:
				require.Truef(t, existingFiles[change.Name], "%s: commit %q: op %q on %q, which does not exist yet (use %q?)", fixturePath, commit.Message, change.Op, change.Name, GitChangeAdd)
				require.NoError(t, os.WriteFile(target, []byte(change.Content), 0o644))
			case GitChangeDelete:
				require.Truef(t, existingFiles[change.Name], "%s: commit %q: op %q on %q, which does not exist", fixturePath, commit.Message, change.Op, change.Name)
				require.NoError(t, os.Remove(target))
				delete(existingFiles, change.Name)
			default:
				t.Fatalf("%s: commit %q: unknown op %q for %q", fixturePath, commit.Message, change.Op, change.Name)
			}
		}

		runGit(t, repoDir, "add", "-A")
		runGit(t, repoDir, "commit", "-q", "-m", commit.Message)

		if commit.Label != "" {
			labels[commit.Label] = runGit(t, repoDir, "rev-parse", "HEAD")
			fileSets[commit.Label] = cloneFileSet(existingFiles)
		}
	}

	ref = fixture.Spec.SinceRef
	if sha, ok := labels[ref]; ok {
		ref = sha
	}
	return repoDir, ref
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, out)
	return strings.TrimSpace(string(out))
}

func cloneFileSet(set map[string]bool) map[string]bool {
	clone := make(map[string]bool, len(set))
	maps.Copy(clone, set)
	return clone
}
