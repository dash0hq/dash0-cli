//go:build unix

package authtokens

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenSecretFile_Mode0600UnderUmask0(t *testing.T) {
	previous := syscall.Umask(0)
	t.Cleanup(func() { syscall.Umask(previous) })
	path := filepath.Join(t.TempDir(), "token.secret")

	file, err := openSecretFile(context.Background(), path)
	require.NoError(t, err)
	require.NoError(t, writeSecretFile(file, secret{value: canary}))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, canary+"\n", string(content))
}

func TestOpenSecretFile_RefusesExistingPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.secret")
	require.NoError(t, os.WriteFile(path, []byte("keep me"), 0o600))

	_, err := openSecretFile(context.Background(), path)

	require.ErrorContains(t, err, "already exists")
	content, _ := os.ReadFile(path)
	assert.Equal(t, "keep me", string(content))
}

func TestOpenSecretFile_RefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "elsewhere")
	path := filepath.Join(dir, "token.secret")
	require.NoError(t, os.Symlink(target, path))

	_, err := openSecretFile(context.Background(), path)

	require.ErrorContains(t, err, "already exists")
	_, statErr := os.Lstat(target)
	assert.True(t, os.IsNotExist(statErr), "the symlink target must not be created")
}

func TestOpenSecretFile_GitWorkTree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	runGitCommand(t, repo, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("*.secret\n"), 0o644))

	_, err := openSecretFile(context.Background(), filepath.Join(repo, "token.txt"))
	require.ErrorContains(t, err, "git does not ignore it")
	_, statErr := os.Lstat(filepath.Join(repo, "token.txt"))
	assert.True(t, os.IsNotExist(statErr), "a refused path must not be created")

	file, err := openSecretFile(context.Background(), filepath.Join(repo, "token.secret"))
	require.NoError(t, err)
	discardSecretFile(file)
}

func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}
