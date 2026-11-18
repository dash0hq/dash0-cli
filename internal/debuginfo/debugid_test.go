package debuginfo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Computed with uuid.v5 of the hex SHA-256 of the map, which is what generateDebugId of @polarsignals/sourcemap-cli does.
func TestDebugIDOfSourceMap_MatchesPolarSignals(t *testing.T) {
	assert.Equal(t, "6b3ca909-8819-5a53-a52f-3f0decdb8b90", debugIDOfSourceMap([]byte(`{}`)))
	assert.Equal(t, "10427d43-f372-51e7-a4b0-5d39b07f9c24", debugIDOfSourceMap([]byte(`{"version":3,"sources":["app.ts"],"mappings":""}`)))
}

// The cases of TestScriptURLBuildID in the Great Lakes exporter (dash0 components/collector/exporter/
// dash0greatlakesexporter/internal/profiles/otlp_test.go), where "" means the exporter keys no frame.
// A source map the CLI accepts under a key the exporter never produces would never resolve a frame.
func TestSourceMapBuildID_MatchesTheExporter(t *testing.T) {
	for scriptURL, want := range map[string]string{
		"https://app.example.com/assets/app.js":          "app.example.com/assets/app.js",
		"http://app.example.com/assets/app.js":           "app.example.com/assets/app.js",
		"https://app.example.com:8443/app.js":            "app.example.com:8443/app.js",
		"https://app.example.com/assets/app.js?v=3#main": "app.example.com/assets/app.js",
		"https://user:secret@app.example.com/app.js":     "app.example.com/app.js",
		"https://app.example.com/":                       "",
		"https://app.example.com":                        "",
		"https://app.example.com//app.js":                "",
		"https://app.example.com/a/../app.js":            "",
		"https://app.example.com/my%20app.js":            "",
		"https://app.example.com/~team/app.js":           "",
		"file:///home/me/app.js":                         "",
		"webpack:///src/app.ts":                          "",
		"/assets/app.js":                                 "",
		"app.js":                                         "",
		"":                                               "",
	} {
		got, err := sourceMapBuildID(scriptURL)
		if want == "" {
			assert.Error(t, err, scriptURL)
			continue
		}
		require.NoError(t, err, scriptURL)
		assert.Equal(t, want, got, scriptURL)
	}
}

const (
	idA = "aaaaaaaa-1111-5222-8333-444444444444"
	idB = "bbbbbbbb-1111-5222-8333-444444444444"
)

func TestPlanDebugID(t *testing.T) {
	const sourceMap = `{"version":3,"mappings":""}`
	computed := debugIDOfSourceMap([]byte(sourceMap))
	cases := map[string]struct {
		script, sourceMap string
		wantID            string
		wantWrites        []string
		wantErr           string
	}{
		"neither has one":           {script: "x\n", sourceMap: sourceMap, wantID: computed, wantWrites: []string{"app.js.map", "app.js"}},
		"only the map has one":      {script: "x\n", sourceMap: `{"debugId":"` + idA + `"}`, wantID: idA, wantWrites: []string{"app.js"}},
		"map uses debug_id":         {script: "x\n", sourceMap: `{"debug_id":"` + idA + `"}`, wantID: idA, wantWrites: []string{"app.js"}},
		"only the script has one":   {script: "x\n//# debugId=" + idA + "\n", sourceMap: sourceMap, wantID: idA},
		"both have the same one":    {script: "x\n//# debugId=" + idA, sourceMap: `{"debugId":"` + idA + `"}`, wantID: idA},
		"uppercase is reused lower": {script: "x\n//# debugId=" + strings.ToUpper(idA) + "\n", sourceMap: sourceMap, wantID: idA},
		"the two disagree":          {script: "x\n//# debugId=" + idA + "\n", sourceMap: `{"debugId":"` + idB + `"}`, wantErr: "belongs to another build"},
		"map ID is not a UUID":      {script: "x\n", sourceMap: `{"debugId":"build-42"}`, wantErr: "is not a UUID"},
		"map is not an object":      {script: "x\n", sourceMap: `[]`, wantErr: "is not a JSON object"},
		"indented comment":          {script: "x\n  //# debugId=" + idA + "\n", sourceMap: sourceMap, wantErr: "the profiler cannot read"},
		"legacy //@ comment":        {script: "x\n//@ debugId=" + idA + "\n", sourceMap: sourceMap, wantErr: "the profiler cannot read"},
		"comment ends with CR":      {script: "x\r\n//# debugId=" + idA + "\r\n", sourceMap: sourceMap, wantErr: "the profiler cannot read"},
		"comment not near the end":  {script: "//# debugId=" + idA + "\n" + strings.Repeat("x", 2000) + "\n", sourceMap: sourceMap, wantErr: "the profiler cannot read"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "app.js", tc.script)
			writeFile(t, dir, "app.js.map", tc.sourceMap)

			debugID, writes, err := planDebugID(filepath.Join(dir, "app.js"), filepath.Join(dir, "app.js.map"))

			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantID, debugID)
			var written []string
			for _, write := range writes {
				written = append(written, filepath.Base(write.Path))
			}
			assert.Equal(t, tc.wantWrites, written)
		})
	}
}

func TestPlanDebugID_WritesWhatTheProfilerReads(t *testing.T) {
	for name, script := range map[string]string{
		"LF":                  "console.log(1)\n//# sourceMappingURL=app.js.map\n",
		"no trailing newline": "console.log(1)\n//# sourceMappingURL=app.js.map",
		"CRLF":                "console.log(1)\r\n//# sourceMappingURL=app.js.map\r\n",
		"large script":        strings.Repeat("x", 5000) + "\n//# sourceMappingURL=app.js.map\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "app.js", script)
			writeFile(t, dir, "app.js.map", `{"version":3}`)

			debugID, writes, err := planDebugID(filepath.Join(dir, "app.js"), filepath.Join(dir, "app.js.map"))
			require.NoError(t, err)
			require.Len(t, writes, 2)

			newScript := writes[1].Content
			assert.True(t, strings.HasPrefix(string(newScript), script), "the script keeps its bytes")
			tail := newScript[max(0, len(newScript)-profilerDebugIDTailSize):]
			matches := profilerDebugIDComment.FindSubmatch(tail)
			require.NotNil(t, matches)
			assert.Equal(t, debugID, string(matches[1]))
		})
	}
}

func TestInsertDebugIDField(t *testing.T) {
	cases := map[string]string{
		`{"version":3,"names":["aé"]}`: `{"debugId":"` + idA + `","version":3,"names":["aé"]}`,
		"{\n  \"version\": 3\n}\n":     "{\"debugId\":\"" + idA + "\",\n  \"version\": 3\n}\n",
		`{}`:                           `{"debugId":"` + idA + `"}`,
		"{ \n }":                       `{"debugId":"` + idA + "\" \n }",
	}
	for sourceMap, want := range cases {
		got, err := insertDebugIDField("app.js.map", []byte(sourceMap), idA)
		require.NoError(t, err, sourceMap)
		assert.Equal(t, want, string(got), sourceMap)
		assert.True(t, json.Valid(got), sourceMap)
	}
}

func TestWriteFileAtomically_KeepsModeAndFollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "real/app.js", "old")
	target := filepath.Join(dir, "real/app.js")
	require.NoError(t, os.Chmod(target, 0o640))
	link := filepath.Join(dir, "app.js")
	require.NoError(t, os.Symlink(target, link))

	require.NoError(t, writeFileAtomically(fileWrite{Path: link, Content: []byte("new")}))

	content, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new", string(content))
	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.Equal(t, os.ModeSymlink, info.Mode().Type(), "the link stays a link")
	info, err = os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
	entries, err := os.ReadDir(filepath.Join(dir, "real"))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temporary file is left behind")
}

func TestDiscoverSourceMaps_KeysByDebugIDAndSkipsNodeModules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "server.js", "x\n//# debugId="+idA+"\n")
	writeFile(t, dir, "server.js.map", "{}")
	writeFile(t, dir, "node_modules/dep/index.js", "y")
	writeFile(t, dir, "node_modules/dep/index.js.map", "{}")

	artifacts, skipped, err := discoverSourceMaps(dir, "")

	require.NoError(t, err)
	assert.Empty(t, skipped)
	require.Len(t, artifacts, 1)
	assert.Equal(t, idA, artifacts[0].BuildID)
	assert.Empty(t, artifacts[0].ScriptURL)
}
