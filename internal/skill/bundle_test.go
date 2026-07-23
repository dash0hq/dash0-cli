package skill

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestEntriesMatchEmbeddedFiles(t *testing.T) {
	for _, entry := range Manifest {
		t.Run(entry.Topic, func(t *testing.T) {
			b, err := bundleFS.ReadFile("content/" + entry.RelPath)
			require.NoError(t, err, "Manifest entry %q references a file that isn't embedded", entry.RelPath)
			assert.NotEmpty(t, b)
			assert.Equal(t, "references/"+entry.Topic+".md", entry.RelPath)
		})
	}
}

// TestEmbeddedFilesAreInManifest is the reverse of
// TestManifestEntriesMatchEmbeddedFiles: it walks the embedded FS and asserts
// every references/*.md file maps back to a Manifest entry, so a stale file
// left after a topic rename does not compile in unreachable via TopicContent.
func TestEmbeddedFilesAreInManifest(t *testing.T) {
	inManifest := make(map[string]bool, len(Manifest))
	for _, entry := range Manifest {
		inManifest[entry.RelPath] = true
	}
	err := fs.WalkDir(bundleFS, "content/references", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel := strings.TrimPrefix(path, "content/")
		assert.True(t, inManifest[rel], "embedded reference %q has no matching Manifest entry — orphan file, likely a rename that missed cleanup", rel)
		return nil
	})
	require.NoError(t, err)
}

func TestManifestTopicsAreUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, entry := range Manifest {
		assert.False(t, seen[entry.Topic], "duplicate topic %q in Manifest", entry.Topic)
		seen[entry.Topic] = true
	}
}

func TestSkillMDIsEmbedded(t *testing.T) {
	content, err := SkillMD()
	require.NoError(t, err)
	assert.Contains(t, content, "name: dash0-cli")
}

func TestTopicContentUnknownTopicListsValidNames(t *testing.T) {
	_, err := TopicContent("does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown skill topic "does-not-exist"`)
	assert.Contains(t, err.Error(), "\nHint: valid topics are:")
	for _, name := range TopicNames() {
		assert.Contains(t, err.Error(), name)
	}
}

func TestIsInstalledFalseOnEmptyDir(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, IsInstalled(dir))
}

func TestIsInstalledTrueForEachSupportedHost(t *testing.T) {
	for _, host := range supportedHosts {
		t.Run(host.Slug, func(t *testing.T) {
			dir := t.TempDir()
			full := filepath.Join(dir, host.Dir)
			require.NoError(t, os.MkdirAll(filepath.Join(full, "references"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(full, "SKILL.md"), []byte("x"), 0o644))
			// IsInstalled requires SKILL.md AND a reference topic (uses
			// Manifest[0]) so partial installs don't silently suppress the hint.
			require.NoError(t, os.WriteFile(filepath.Join(full, Manifest[0].RelPath), []byte("x"), 0o644))
			assert.True(t, IsInstalled(dir))
		})
	}
}

// TestIsInstalledFalseWithOnlySkillMD verifies that a partial install
// (SKILL.md written, references directory missing) does NOT count as
// installed — otherwise the agent-mode hint pointing at `dash0 skill install`
// would be silently suppressed while the bundle is broken.
func TestIsInstalledFalseWithOnlySkillMD(t *testing.T) {
	dir := t.TempDir()
	full := filepath.Join(dir, supportedHosts[0].Dir)
	require.NoError(t, os.MkdirAll(full, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(full, "SKILL.md"), []byte("x"), 0o644))
	assert.False(t, IsInstalled(dir), "IsInstalled must reject a partial install with SKILL.md but no references")
}

// TestHostSupported spot-checks the exported HostSupported helper used by
// callers deciding whether nudging at `dash0 skill install` is meaningful.
func TestHostSupported(t *testing.T) {
	assert.True(t, HostSupported("claude-code"))
	assert.True(t, HostSupported("codex"))
	assert.True(t, HostSupported("cursor"))
	assert.True(t, HostSupported("copilot"))
	assert.False(t, HostSupported("aider"))
	assert.False(t, HostSupported(""))
}
