package git

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListYAMLFilesAtRef_SkipsHiddenAndNonYAML(t *testing.T) {
	repo := testRepo(t)
	writeFile(t, repo.Dir, "dashboard.yaml", "kind: Dashboard\n")
	writeFile(t, repo.Dir, "notes.txt", "not yaml\n")
	writeFile(t, repo.Dir, ".hidden/secret.yaml", "kind: Dashboard\n")
	writeFile(t, repo.Dir, "nested/view.yml", "kind: View\n")
	commitAll(t, repo.Dir, "add files")

	files, err := repo.ListYAMLFilesAtRef(context.Background(), "HEAD", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"dashboard.yaml", "nested/view.yml"}, files)
}

func TestListYAMLFilesAtRef_ScopedToDirectory(t *testing.T) {
	repo := testRepo(t)
	writeFile(t, repo.Dir, "a/dashboard.yaml", "kind: Dashboard\n")
	writeFile(t, repo.Dir, "b/view.yaml", "kind: View\n")
	commitAll(t, repo.Dir, "add files")

	files, err := repo.ListYAMLFilesAtRef(context.Background(), "HEAD", "a")
	require.NoError(t, err)
	assert.Equal(t, []string{"a/dashboard.yaml"}, files)
}

// TestListYAMLFilesAtRef_SingleFileScopeIgnoresExtension is a regression
// test for a bug where a single-file -f target without a .yaml/.yml
// extension (e.g. -f config.json) was silently excluded from the git-ref
// side of a --since scan, even though apply's own single-file create/update
// path (readMultiDocumentYAML) has no extension check at all and would read
// the exact same file just fine.
func TestListYAMLFilesAtRef_SingleFileScopeIgnoresExtension(t *testing.T) {
	repo := testRepo(t)
	writeFile(t, repo.Dir, "config.json", "kind: Dashboard\n")
	commitAll(t, repo.Dir, "add config.json")

	files, err := repo.ListYAMLFilesAtRef(context.Background(), "HEAD", "config.json")
	require.NoError(t, err)
	assert.Equal(t, []string{"config.json"}, files, "a single-file scope must be scanned regardless of its extension")
}

// TestListYAMLFilesAtRef_DirectoryScopeStillFiltersExtension confirms the
// fix above didn't turn off extension filtering for directory scopes: only
// an *exact* scope match (a single-file target) is exempt.
func TestListYAMLFilesAtRef_DirectoryScopeStillFiltersExtension(t *testing.T) {
	repo := testRepo(t)
	writeFile(t, repo.Dir, "assets/dashboard.yaml", "kind: Dashboard\n")
	writeFile(t, repo.Dir, "assets/notes.txt", "not yaml\n")
	commitAll(t, repo.Dir, "add files")

	files, err := repo.ListYAMLFilesAtRef(context.Background(), "HEAD", "assets")
	require.NoError(t, err)
	assert.Equal(t, []string{"assets/dashboard.yaml"}, files)
}

// TestListYAMLFilesAtRef_ScopedToDotPrefixedDirectory is a regression test
// for a bug where scoping to a dot-prefixed directory (e.g. -f .dash0-assets/)
// always returned zero files: IsHiddenPath was applied to the full
// scope-prefixed repo-relative path, so the scope directory's own leading
// "." made every file inside it look hidden. The disk-side walker
// (FindNonHiddenYAMLFiles) exempts its walk root from the hidden check the
// same way; the git-side listing must match that, checking hidden-ness only
// for path components *below* scope.
func TestListYAMLFilesAtRef_ScopedToDotPrefixedDirectory(t *testing.T) {
	repo := testRepo(t)
	writeFile(t, repo.Dir, ".dash0-assets/dashboard.yaml", "kind: Dashboard\n")
	writeFile(t, repo.Dir, ".dash0-assets/.hidden/nested.yaml", "kind: View\n")
	commitAll(t, repo.Dir, "add files")

	files, err := repo.ListYAMLFilesAtRef(context.Background(), "HEAD", ".dash0-assets")
	require.NoError(t, err)
	assert.Equal(t, []string{".dash0-assets/dashboard.yaml"}, files, "the dot-prefixed scope itself must not hide its own contents, but a hidden directory nested inside it still must be skipped")
}

func TestReadFileAtRef(t *testing.T) {
	repo := testRepo(t)
	writeFile(t, repo.Dir, "dashboard.yaml", "kind: Dashboard\nname: v1\n")
	commitAll(t, repo.Dir, "v1")
	writeFile(t, repo.Dir, "dashboard.yaml", "kind: Dashboard\nname: v2\n")
	sha2 := commitAll(t, repo.Dir, "v2")

	content, err := repo.ReadFileAtRef(context.Background(), "HEAD~1", "dashboard.yaml")
	require.NoError(t, err)
	assert.Equal(t, "kind: Dashboard\nname: v1\n", string(content))

	content, err = repo.ReadFileAtRef(context.Background(), sha2, "dashboard.yaml")
	require.NoError(t, err)
	assert.Equal(t, "kind: Dashboard\nname: v2\n", string(content))
}

func TestIsAncestor(t *testing.T) {
	repo := testRepo(t)
	base := runGit(t, repo.Dir, "rev-parse", "HEAD")
	writeFile(t, repo.Dir, "f.yaml", "kind: Dashboard\n")
	head := commitAll(t, repo.Dir, "add file")

	isAncestor, err := repo.IsAncestor(context.Background(), base, head)
	require.NoError(t, err)
	assert.True(t, isAncestor)

	isAncestor, err = repo.IsAncestor(context.Background(), head, base)
	require.NoError(t, err)
	assert.False(t, isAncestor)
}

func TestRoot(t *testing.T) {
	repo := testRepo(t)
	writeFile(t, repo.Dir, "sub/dashboard.yaml", "kind: Dashboard\n")
	commitAll(t, repo.Dir, "add file")

	wantRoot, err := filepath.EvalSymlinks(repo.Dir)
	require.NoError(t, err)

	subRepo := Repo{Dir: repo.Dir + "/sub"}
	root, err := subRepo.Root(context.Background())
	require.NoError(t, err)
	gotRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	assert.Equal(t, wantRoot, gotRoot, "Root must resolve to the repo's top level even when Dir is a subdirectory")
}

func TestIsTreeAtRef_RepoRoot(t *testing.T) {
	repo := testRepo(t)
	isTree, err := repo.IsTreeAtRef(context.Background(), "HEAD", "")
	require.NoError(t, err)
	assert.True(t, isTree, "the repository root itself is always a tree")
}

func TestIsTreeAtRef_Directory(t *testing.T) {
	repo := testRepo(t)
	writeFile(t, repo.Dir, "dashboards/one.yaml", "kind: Dashboard\n")
	commitAll(t, repo.Dir, "add directory")

	isTree, err := repo.IsTreeAtRef(context.Background(), "HEAD", "dashboards")
	require.NoError(t, err)
	assert.True(t, isTree)
}

func TestIsTreeAtRef_File(t *testing.T) {
	repo := testRepo(t)
	writeFile(t, repo.Dir, "dashboard.yaml", "kind: Dashboard\n")
	commitAll(t, repo.Dir, "add file")

	isTree, err := repo.IsTreeAtRef(context.Background(), "HEAD", "dashboard.yaml")
	require.NoError(t, err)
	assert.False(t, isTree)
}

func TestIsTreeAtRef_NonexistentPathIsError(t *testing.T) {
	repo := testRepo(t)
	_, err := repo.IsTreeAtRef(context.Background(), "HEAD", "never-existed.yaml")
	require.Error(t, err)
}

func TestIsAncestor_UnknownRefIsError(t *testing.T) {
	repo := testRepo(t)
	_, err := repo.IsAncestor(context.Background(), "does-not-exist", "HEAD")
	require.Error(t, err)
}

// TestRoot_NotAGitRepository pins IsNotAGitRepository's contract: Root's
// error, run against a plain directory that was never a git repository at
// all, must be recognized by IsNotAGitRepository so callers can build a
// clean, single-line message instead of propagating git's own nested
// "failed to determine repository root for ...: git rev-parse
// --show-toplevel: exit status 128 (stderr: fatal: not a git repository
// ...)" chain verbatim.
func TestRoot_NotAGitRepository(t *testing.T) {
	notARepo := Repo{Dir: t.TempDir()}
	_, err := notARepo.Root(context.Background())
	require.Error(t, err)
	assert.True(t, IsNotAGitRepository(err), "expected IsNotAGitRepository to recognize %v", err)
}

// TestIsNotAGitRepository_OtherErrorsReturnFalse pins that
// IsNotAGitRepository doesn't fire on an unrelated error, so callers don't
// mistakenly swap in the "not a git repository" message for some other
// infrastructure failure.
func TestIsNotAGitRepository_OtherErrorsReturnFalse(t *testing.T) {
	assert.False(t, IsNotAGitRepository(nil))
	assert.False(t, IsNotAGitRepository(errors.New("some other failure")))
}

// TestListYAMLFilesAtRef_PathsGitWouldMangle covers the paths that only
// survive with -z. Non-ASCII is the reported regression: git C-quoted it, the
// trailing quote failed the extension check, and the deletion went undetected.
// The other two are why -z beats core.quotePath=false, which fixes neither.
//
// ListYAMLFilesAtRef sorts, so each case's files double as its expectation.
func TestListYAMLFilesAtRef_PathsGitWouldMangle(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
	}{
		{"non-ASCII bytes, which git C-quotes", []string{"café.yaml", "plain.yaml", "日本語/ビュー.yaml"}},
		{"a newline, which splits one path into two", []string{"we\nird.yaml"}},
		{"a leading space, which per-line trimming eats", []string{" lead.yaml"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := testRepo(t)
			for _, f := range tc.files {
				writeFile(t, repo.Dir, f, "kind: View\n")
			}
			commitAll(t, repo.Dir, "add files")

			files, err := repo.ListYAMLFilesAtRef(context.Background(), "HEAD", "")
			require.NoError(t, err)
			assert.Equal(t, tc.files, files)
		})
	}
}

// TestHasSkipWorktreeFiles checks per scope, not repo-wide: a sparse cone
// elsewhere in a monorepo must not block a --since run whose own target is
// fully materialized.
func TestHasSkipWorktreeFiles(t *testing.T) {
	for _, tc := range []struct {
		scope string
		want  bool
	}{
		{"", true},
		{"drop", true},
		{"keep", false},
	} {
		t.Run("scope="+tc.scope, func(t *testing.T) {
			repo := testRepo(t)
			writeFile(t, repo.Dir, "keep/k.yaml", "kind: View\n")
			writeFile(t, repo.Dir, "drop/d.yaml", "kind: View\n")
			commitAll(t, repo.Dir, "add files")
			runGit(t, repo.Dir, "update-index", "--skip-worktree", "drop/d.yaml")

			got, err := repo.HasSkipWorktreeFiles(context.Background(), tc.scope)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
