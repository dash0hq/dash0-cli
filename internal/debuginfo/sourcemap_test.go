package debuginfo

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourceMapBuildID(t *testing.T) {
	valid := map[string]string{
		"https://app.example.com/assets/app.js?v=3":        "app.example.com/assets/app.js",
		"https://app.example.com/assets/app.js#section":    "app.example.com/assets/app.js",
		"http://localhost:3000/static/main.js":             "localhost:3000/static/main.js",
		"https://app.example.com/_next/static/chunks/a.js": "app.example.com/_next/static/chunks/a.js",
	}
	for scriptURL, want := range valid {
		got, err := sourceMapBuildID(scriptURL)
		require.NoError(t, err, scriptURL)
		assert.Equal(t, want, got, scriptURL)
	}

	invalid := map[string]string{
		"ftp://app.example.com/app.js":             "scheme must be http or https",
		"app.example.com/app.js":                   "scheme must be http or https",
		"https://app.example.com":                  "must have a host and a path",
		"https://app.example.com/a%20b.js":         "character Dash0 cannot look up",
		"https://app.example.com/a~b.js":           "character Dash0 cannot look up",
		"https://app.example.com/assets/":          "not a valid relative path",
		"https://app.example.com/assets//app.js":   "not a valid relative path",
		"https://app.example.com/assets/./app.js":  "not a valid relative path",
		"https://app.example.com/assets/../app.js": "not a valid relative path",
	}
	for scriptURL, want := range invalid {
		_, err := sourceMapBuildID(scriptURL)
		require.Error(t, err, scriptURL)
		assert.Contains(t, err.Error(), want, scriptURL)
	}
}

func TestEncodeSourceMapBundle(t *testing.T) {
	script := []byte("console.log(1)")
	sourceMap := []byte(`{"version":3}`)

	bundle := encodeSourceMapBundle(script, sourceMap)

	require.Len(t, bundle, 16+len(script)+len(sourceMap))
	assert.Equal(t, uint64(len(script)), binary.LittleEndian.Uint64(bundle[0:8]))
	assert.Equal(t, uint64(len(sourceMap)), binary.LittleEndian.Uint64(bundle[8:16]))
	assert.Equal(t, script, bundle[16:16+len(script)])
	assert.Equal(t, sourceMap, bundle[16+len(script):])
}

func TestDiscoverSourceMaps(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "assets/app.js", "app")
	writeFile(t, dir, "assets/app.js.map", "{}")
	writeFile(t, dir, "chunks/vendor.mjs", "vendor")
	writeFile(t, dir, "chunks/vendor.mjs.map", "{}")
	writeFile(t, dir, "no-map.js", "x")
	writeFile(t, dir, "orphan.js.map", "{}")
	writeFile(t, dir, "styles.css", "body{}")

	artifacts, skipped, err := discoverSourceMaps(dir, "https://app.example.com/static")

	require.NoError(t, err)
	assert.Equal(t, []sourceMapArtifact{
		{
			BuildID:   "app.example.com/static/assets/app.js",
			ScriptURL: "https://app.example.com/static/assets/app.js",
			Script:    filepath.Join(dir, "assets/app.js"),
			SourceMap: filepath.Join(dir, "assets/app.js.map"),
		},
		{
			BuildID:   "app.example.com/static/chunks/vendor.mjs",
			ScriptURL: "https://app.example.com/static/chunks/vendor.mjs",
			Script:    filepath.Join(dir, "chunks/vendor.mjs"),
			SourceMap: filepath.Join(dir, "chunks/vendor.mjs.map"),
		},
	}, artifacts)
	require.Len(t, skipped, 1)
	assert.Equal(t, filepath.Join(dir, "no-map.js"), skipped[0].Script)
}

func TestDiscoverSourceMaps_FollowsSourceMappingURLComment(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "chunks/07g2mnivp-btw.js", "console.log(1)\n//# sourceMappingURL=21dgws7yamugy.js.map\n")
	writeFile(t, dir, "chunks/21dgws7yamugy.js.map", "{}")
	writeFile(t, dir, "chunks/07g2mnivp-btw.js.map", "{}")
	writeFile(t, dir, "chunks/nested.js", "x\n//# sourceMappingURL=../maps/nested.js.map")
	writeFile(t, dir, "maps/nested.js.map", "{}")

	artifacts, skipped, err := discoverSourceMaps(dir, "https://app.example.com/_next/static/")

	require.NoError(t, err)
	assert.Empty(t, skipped)
	require.Len(t, artifacts, 2)
	assert.Equal(t, "app.example.com/_next/static/chunks/07g2mnivp-btw.js", artifacts[0].BuildID)
	assert.Equal(t, filepath.Join(dir, "chunks/21dgws7yamugy.js.map"), artifacts[0].SourceMap)
	assert.Equal(t, filepath.Join(dir, "maps/nested.js.map"), artifacts[1].SourceMap)
}

func TestDiscoverSourceMaps_SkipsScriptsWhoseCommentCannotBeFollowed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "missing.js", "x\n//# sourceMappingURL=gone.js.map\n")
	writeFile(t, dir, "missing.js.map", "{}")
	writeFile(t, dir, "inline.js", "x\n//# sourceMappingURL=data:application/json;base64,e30=\n")
	writeFile(t, dir, "remote.js", "x\n//# sourceMappingURL=https://cdn.example.com/remote.js.map\n")

	artifacts, skipped, err := discoverSourceMaps(dir, "https://app.example.com/")

	require.NoError(t, err)
	assert.Empty(t, artifacts)
	reasons := map[string]string{}
	for _, script := range skipped {
		reasons[filepath.Base(script.Script)] = script.Reason
	}
	assert.Contains(t, reasons["missing.js"], "gone.js.map not found")
	assert.Contains(t, reasons["inline.js"], "inline (data: URL)")
	assert.Contains(t, reasons["remote.js"], "not a path relative to the script")
}

func TestDiscoverSourceMaps_RejectsFileNameDash0CannotLookUp(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app~1.js", "app")
	writeFile(t, dir, "app~1.js.map", "{}")

	_, _, err := discoverSourceMaps(dir, "https://app.example.com/")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "app~1.js")
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	fullPath := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
	require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
}
