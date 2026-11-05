package debuginfo

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Characters that the object store client of the Dash0 query path percent-encodes in a key, while the
// debuginfo service stores the key as uploaded, so a build ID containing one can never be read back.
// https://github.com/apache/arrow-rs-object-store/blob/v0.13.2/src/path/parts.rs
const objectStoreEncodedCharacters = "\\{^}%`]\">[~<#|*?\r\n"

// Must match how Dash0 keys a browser profile frame at read time: the host plus the escaped path of the
// script URL, without scheme, query, or fragment. Any drift makes uploaded maps unreachable.
func sourceMapBuildID(scriptURL string) (string, error) {
	parsed, err := url.Parse(scriptURL)
	if err != nil {
		return "", fmt.Errorf("invalid script URL %q:\n  %w", scriptURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid script URL %q: the scheme must be http or https", scriptURL)
	}
	if parsed.Host == "" || parsed.Path == "" {
		return "", fmt.Errorf("invalid script URL %q: it must have a host and a path", scriptURL)
	}

	buildID := parsed.Host + parsed.EscapedPath()
	if strings.ContainsAny(buildID, objectStoreEncodedCharacters) {
		return "", fmt.Errorf("script URL %q gives the build ID %q, which contains a character Dash0 cannot look up (one of %q); rename the file or serve it from a URL without these characters", scriptURL, buildID, objectStoreEncodedCharacters)
	}
	if !fs.ValidPath(buildID) {
		return "", fmt.Errorf("script URL %q gives the build ID %q, which is not a valid relative path (empty, \".\", or \"..\" segments are not allowed)", scriptURL, buildID)
	}
	return buildID, nil
}

// encodeSourceMapBundle builds the body Dash0 reads a source map from:
// [script length, u64 little endian][map length, u64 little endian][script bytes][map bytes].
func encodeSourceMapBundle(script, sourceMap []byte) []byte {
	var buf bytes.Buffer
	buf.Grow(16 + len(script) + len(sourceMap))
	_ = binary.Write(&buf, binary.LittleEndian, uint64(len(script)))
	_ = binary.Write(&buf, binary.LittleEndian, uint64(len(sourceMap)))
	buf.Write(script)
	buf.Write(sourceMap)
	return buf.Bytes()
}

type sourceMapArtifact struct {
	BuildID   string `json:"buildId"`
	ScriptURL string `json:"scriptUrl"`
	Script    string `json:"script"`
	SourceMap string `json:"sourceMap"`
}

func (a sourceMapArtifact) readBundle() ([]byte, error) {
	script, err := os.ReadFile(a.Script)
	if err != nil {
		return nil, fmt.Errorf("failed to read script:\n  %w", err)
	}
	if len(script) == 0 {
		return nil, fmt.Errorf("script %s is empty", a.Script)
	}
	sourceMap, err := os.ReadFile(a.SourceMap)
	if err != nil {
		return nil, fmt.Errorf("failed to read source map:\n  %w", err)
	}
	if !json.Valid(sourceMap) {
		return nil, fmt.Errorf("source map %s is not valid JSON", a.SourceMap)
	}
	return encodeSourceMapBundle(script, sourceMap), nil
}

var scriptExtensions = []string{".js", ".mjs", ".cjs"}

func isScript(name string) bool {
	for _, ext := range scriptExtensions {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

func joinBaseURL(baseURL, relativePath string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid --base-url %q:\n  %w", baseURL, err)
	}
	if base.RawQuery != "" || base.Fragment != "" {
		return "", fmt.Errorf("invalid --base-url %q: it must not have a query or fragment", baseURL)
	}
	base.Path = path.Join("/", base.Path, relativePath)
	base.RawPath = ""
	return base.String(), nil
}

// discoverSourceMaps walks dir and pairs every script with the source map next to it (<script>.map).
// It returns the pairs, sorted by build ID, and the scripts that have no source map.
func discoverSourceMaps(dir, baseURL string) ([]sourceMapArtifact, []string, error) {
	var artifacts []sourceMapArtifact
	var unpaired []string
	err := filepath.WalkDir(dir, func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !isScript(entry.Name()) {
			return nil
		}
		mapPath := filePath + ".map"
		if info, statErr := os.Stat(mapPath); statErr != nil || info.IsDir() {
			unpaired = append(unpaired, filePath)
			return nil
		}
		relativePath, err := filepath.Rel(dir, filePath)
		if err != nil {
			return err
		}
		scriptURL, err := joinBaseURL(baseURL, filepath.ToSlash(relativePath))
		if err != nil {
			return err
		}
		buildID, err := sourceMapBuildID(scriptURL)
		if err != nil {
			return fmt.Errorf("%s:\n  %w", filePath, err)
		}
		artifacts = append(artifacts, sourceMapArtifact{BuildID: buildID, ScriptURL: scriptURL, Script: filePath, SourceMap: mapPath})
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].BuildID < artifacts[j].BuildID })
	return artifacts, unpaired, nil
}
