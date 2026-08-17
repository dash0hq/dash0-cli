package asset

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsYAMLFile(t *testing.T) {
	assert.True(t, IsYAMLFile("dashboard.yaml"))
	assert.True(t, IsYAMLFile("dashboard.yml"))
	assert.True(t, IsYAMLFile("DASHBOARD.YAML"))
	assert.True(t, IsYAMLFile("nested/dir/dashboard.yaml"))
	assert.False(t, IsYAMLFile("dashboard.json"))
	assert.False(t, IsYAMLFile("dashboard.txt"))
	assert.False(t, IsYAMLFile("dashboard"))
}

func TestIsHiddenPath(t *testing.T) {
	assert.True(t, IsHiddenPath(".hidden"))
	assert.True(t, IsHiddenPath(".hidden/dashboard.yaml"))
	assert.True(t, IsHiddenPath("dir/.hidden/dashboard.yaml"))
	assert.True(t, IsHiddenPath("dir/.hidden"))
	assert.False(t, IsHiddenPath("dashboard.yaml"))
	assert.False(t, IsHiddenPath("dir/dashboard.yaml"))
	assert.False(t, IsHiddenPath(""))
}

func writeTestFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

func TestFindNonHiddenYAMLFiles_FlatDirectory(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.yaml", "kind: Dashboard\n")
	writeTestFile(t, dir, "b.yml", "kind: View\n")
	writeTestFile(t, dir, "notes.txt", "not yaml")

	var files []string
	var sawNestedDir bool
	require.NoError(t, filepath.WalkDir(dir, FindNonHiddenYAMLFiles(dir, &files, &sawNestedDir)))
	assert.False(t, sawNestedDir)
	assert.ElementsMatch(t, []string{filepath.Join(dir, "a.yaml"), filepath.Join(dir, "b.yml")}, files)
}

func TestFindNonHiddenYAMLFiles_NestedDirs(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "sub/nested.yaml", "kind: Dashboard\n")

	var files []string
	var sawNestedDir bool
	require.NoError(t, filepath.WalkDir(dir, FindNonHiddenYAMLFiles(dir, &files, &sawNestedDir)))
	assert.True(t, sawNestedDir)
	assert.Equal(t, []string{filepath.Join(dir, "sub/nested.yaml")}, files)
}

func TestFindNonHiddenYAMLFiles_SkipsHiddenFilesAndDirs(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "visible.yaml", "kind: Dashboard\n")
	writeTestFile(t, dir, ".hidden.yaml", "kind: Dashboard\n")
	writeTestFile(t, dir, ".hidden/inside.yaml", "kind: Dashboard\n")

	var files []string
	var sawNestedDir bool
	require.NoError(t, filepath.WalkDir(dir, FindNonHiddenYAMLFiles(dir, &files, &sawNestedDir)))
	assert.False(t, sawNestedDir, "the only subdirectory is hidden, so it must not count as a nested dir")
	assert.Equal(t, []string{filepath.Join(dir, "visible.yaml")}, files)
}

func TestFindNonHiddenYAMLFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	var files []string
	var sawNestedDir bool
	require.NoError(t, filepath.WalkDir(dir, FindNonHiddenYAMLFiles(dir, &files, &sawNestedDir)))
	assert.False(t, sawNestedDir)
	assert.Empty(t, files)
}

func TestFindNonHiddenYAMLFiles_NilSawNestedDir(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "sub/nested.yaml", "kind: Dashboard\n")

	var files []string
	require.NoError(t, filepath.WalkDir(dir, FindNonHiddenYAMLFiles(dir, &files, nil)))
	assert.Equal(t, []string{filepath.Join(dir, "sub/nested.yaml")}, files)
}

// TestFindNonHiddenYAMLFiles_DotPrefixedRootIsNotHidden pins a deliberate
// behavior: a dot-prefixed root (e.g. -f .dash0-assets/, explicitly named by
// the user) is exempt from the hidden-name check, even though every
// component of an ordinary path is otherwise checked. Only the root itself
// is exempt — a hidden entry *within* it is still skipped. This must stay in
// sync with ListYAMLFilesAtRef's equivalent git-ref-side exemption
// (internal/git/plumbing.go), which mirrors this rule so the two scan sides
// agree on what a dot-prefixed -f target means.
func TestFindNonHiddenYAMLFiles_DotPrefixedRootIsNotHidden(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, ".dash0-assets")
	writeTestFile(t, dir, "dashboard.yaml", "kind: Dashboard\n")
	writeTestFile(t, dir, ".hidden/inside.yaml", "kind: Dashboard\n")

	var files []string
	require.NoError(t, filepath.WalkDir(dir, FindNonHiddenYAMLFiles(dir, &files, nil)))
	assert.Equal(t, []string{filepath.Join(dir, "dashboard.yaml")}, files, "the dot-prefixed root itself must not be treated as hidden, but a hidden directory nested inside it still must be skipped")
}
