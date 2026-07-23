package skill

import (
	"os"
	"path/filepath"
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
			assert.NotEmpty(t, entry.Description)
			assert.Equal(t, "references/"+entry.Topic+".md", entry.RelPath)
		})
	}
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
	for slug, hostDir := range supportedHosts {
		t.Run(slug, func(t *testing.T) {
			dir := t.TempDir()
			full := filepath.Join(dir, hostDir)
			require.NoError(t, os.MkdirAll(full, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(full, "SKILL.md"), []byte("x"), 0o644))
			assert.True(t, IsInstalled(dir))
		})
	}
}
