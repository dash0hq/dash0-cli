package debuginfo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// The same namespace as @polarsignals/sourcemap-cli, so both tools give one build the same debug ID
// and a source map uploaded by either one matches.
// https://github.com/polarsignals/sourcemap-upload-plugins/blob/main/packages/core/src/utils/debug-id.ts
var debugIDNamespace = uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")

// The OpenTelemetry eBPF profiler reads the debug ID with this pattern from the last 1024 bytes of a script,
// so the CLI accepts and writes only what the profiler can read.
// https://github.com/parca-dev/opentelemetry-ebpf-profiler/blob/main/interpreter/nodev8/debugid.go
var profilerDebugIDComment = regexp.MustCompile(`(?m)^//# debugId=([a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12})$`)

const profilerDebugIDTailSize = 1024

var anyDebugIDComment = regexp.MustCompile(`(?m)^[ \t]*//[#@][ \t]*debugId=.*$`)

// debugIDOfSourceMap matches generateDebugId of @polarsignals/sourcemap-cli: a UUID v5 of the hex SHA-256 of the
// source map, computed before the debug ID is injected into it.
func debugIDOfSourceMap(sourceMap []byte) string {
	digest := sha256.Sum256(sourceMap)
	return uuid.NewSHA1(debugIDNamespace, []byte(hex.EncodeToString(digest[:]))).String()
}

// scriptDebugID returns the lowercase debug ID that the profiler reads from the script, or "" when it has none.
func scriptDebugID(scriptPath string, script []byte) (string, error) {
	tail := script[max(0, len(script)-profilerDebugIDTailSize):]
	if matches := profilerDebugIDComment.FindAllSubmatch(tail, -1); len(matches) > 0 {
		return strings.ToLower(string(matches[len(matches)-1][1])), nil
	}
	if comment := anyDebugIDComment.Find(script); comment != nil {
		return "", fmt.Errorf("%s has the debug ID comment %q, which the profiler cannot read: it reads only a line \"//# debugId=<uuid>\" with no spaces or carriage return, in the last %d bytes of the script", scriptPath, strings.TrimSpace(string(comment)), profilerDebugIDTailSize)
	}
	return "", nil
}

// sourceMapDebugID returns the lowercase debug ID of the source map, or "" when it has none. Rollup and the
// ECMA-426 proposal write "debugId"; older Sentry tooling writes "debug_id".
func sourceMapDebugID(sourceMapPath string, sourceMap []byte) (string, error) {
	var fields struct {
		DebugID      string `json:"debugId"`
		DebugIDSnake string `json:"debug_id"`
	}
	if err := json.Unmarshal(sourceMap, &fields); err != nil {
		return "", fmt.Errorf("source map %s is not a JSON object:\n  %w", sourceMapPath, err)
	}
	debugID := fields.DebugID
	if debugID == "" {
		debugID = fields.DebugIDSnake
	}
	if debugID == "" {
		return "", nil
	}
	parsed, err := uuid.Parse(debugID)
	if err != nil {
		return "", fmt.Errorf("source map %s has the debug ID %q, which is not a UUID", sourceMapPath, debugID)
	}
	return parsed.String(), nil
}

type fileWrite struct {
	Path    string
	Content []byte
}

// planDebugID decides the debug ID of a script and its source map, and the writes that put it into both.
// An ID that is already in the script wins, because a script that is already deployed carries it.
func planDebugID(scriptPath, sourceMapPath string) (string, []fileWrite, error) {
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read script:\n  %w", err)
	}
	if len(script) == 0 {
		return "", nil, fmt.Errorf("script %s is empty", scriptPath)
	}
	sourceMap, err := os.ReadFile(sourceMapPath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read source map:\n  %w", err)
	}
	inScript, err := scriptDebugID(scriptPath, script)
	if err != nil {
		return "", nil, err
	}
	inSourceMap, err := sourceMapDebugID(sourceMapPath, sourceMap)
	if err != nil {
		return "", nil, err
	}

	switch {
	case inScript != "" && inSourceMap != "" && inScript != inSourceMap:
		return "", nil, fmt.Errorf("%s has the debug ID %s, but its source map %s has %s; the source map belongs to another build, so rebuild both", scriptPath, inScript, sourceMapPath, inSourceMap)
	case inScript != "":
		return inScript, nil, nil
	case inSourceMap != "":
		return inSourceMap, []fileWrite{{scriptPath, appendDebugIDComment(script, inSourceMap)}}, nil
	}

	debugID := debugIDOfSourceMap(sourceMap)
	withID, err := insertDebugIDField(sourceMapPath, sourceMap, debugID)
	if err != nil {
		return "", nil, err
	}
	// The source map goes first: when a run stops between the two writes, the next run reuses its ID.
	return debugID, []fileWrite{{sourceMapPath, withID}, {scriptPath, appendDebugIDComment(script, debugID)}}, nil
}

// appendDebugIDComment adds the comment as the last line, where the profiler looks for it. The line ends
// with LF even in a CRLF file, because the profiler's pattern does not match a carriage return.
func appendDebugIDComment(script []byte, debugID string) []byte {
	result := bytes.Clone(script)
	if !bytes.HasSuffix(result, []byte("\n")) {
		result = append(result, '\n')
	}
	return append(result, "//# debugId="+debugID+"\n"...)
}

// insertDebugIDField adds the field right after the opening brace instead of encoding the map again, so the
// rest of the file keeps its bytes.
func insertDebugIDField(sourceMapPath string, sourceMap []byte, debugID string) ([]byte, error) {
	start := bytes.IndexByte(sourceMap, '{')
	if start < 0 || len(bytes.TrimSpace(sourceMap[:start])) > 0 {
		return nil, fmt.Errorf("source map %s is not a JSON object", sourceMapPath)
	}
	field := `"debugId":"` + debugID + `"`
	if rest := bytes.TrimLeft(sourceMap[start+1:], " \t\r\n"); len(rest) == 0 || rest[0] != '}' {
		field += ","
	}
	result := make([]byte, 0, len(sourceMap)+len(field))
	result = append(result, sourceMap[:start+1]...)
	result = append(result, field...)
	return append(result, sourceMap[start+1:]...), nil
}

// checkWritable fails before the first write when any file cannot be written, so that a read-only build
// output does not end up half injected.
func checkWritable(writes []fileWrite) error {
	for _, write := range writes {
		target, err := filepath.EvalSymlinks(write.Path)
		if err != nil {
			return fmt.Errorf("failed to resolve %s:\n  %w", write.Path, err)
		}
		info, err := os.Stat(target)
		if err != nil {
			return fmt.Errorf("failed to read %s:\n  %w", write.Path, err)
		}
		if info.Mode().Perm()&0o200 == 0 {
			return fmt.Errorf("%s is read-only, and the debug ID must be written into it; copy the build output to a writable directory first", write.Path)
		}
	}
	return nil
}

// writeFileAtomically replaces the file a path resolves to, keeping its permissions, so that a crash never
// leaves a truncated script or source map behind.
func writeFileAtomically(write fileWrite) error {
	target, err := filepath.EvalSymlinks(write.Path)
	if err != nil {
		return fmt.Errorf("failed to resolve %s:\n  %w", write.Path, err)
	}
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("failed to read %s:\n  %w", write.Path, err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".*")
	if err != nil {
		return fmt.Errorf("failed to write %s:\n  %w", write.Path, err)
	}
	defer func() { _ = os.Remove(temporary.Name()) }()
	_, err = temporary.Write(write.Content)
	if err == nil {
		err = temporary.Chmod(info.Mode().Perm())
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temporary.Name(), target)
	}
	if err != nil {
		return fmt.Errorf("failed to write %s:\n  %w", write.Path, err)
	}
	return nil
}
