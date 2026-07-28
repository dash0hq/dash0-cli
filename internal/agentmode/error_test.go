package agentmode

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintJSONErrorSimple(t *testing.T) {
	var buf bytes.Buffer
	PrintJSONError(&buf, errors.New("something went wrong"))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Equal(t, "something went wrong", result["error"])
	assert.Empty(t, result["hint"])
}

func TestPrintJSONErrorWithHint(t *testing.T) {
	var buf bytes.Buffer
	PrintJSONError(&buf, errors.New("auth failed\nHint: check your token"))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Equal(t, "auth failed", result["error"])
	assert.Equal(t, "check your token", result["hint"])
}

// The output is JSON for a terminal or an agent, not an HTML page, so the
// encoder must not escape `<`, `>`, or `&` into `<`, `>`, `&`.
// The regression to guard against was `dash0 skill show <topic>` in the hint
// rendering as `dash0 skill show <topic>`.
func TestPrintJSONErrorDoesNotEscapeAngleBracketsAndAmpersand(t *testing.T) {
	var buf bytes.Buffer
	PrintJSONError(&buf, errors.New("bad request <foo> & <bar>\nHint: try `cmd <topic>` & retry"))

	raw := buf.String()
	// The escaped forms are the six-char sequences `<`, `>`, `&`
	// the encoder would emit if HTML escaping were on; we want the raw
	// characters through unchanged.
	assert.NotContains(t, raw, "\\u003c", "encoder should not HTML-escape `<`")
	assert.NotContains(t, raw, "\\u003e", "encoder should not HTML-escape `>`")
	assert.NotContains(t, raw, "\\u0026", "encoder should not HTML-escape `&`")
	assert.Contains(t, raw, "<foo>")
	assert.Contains(t, raw, "cmd <topic>")

	// Still valid JSON with the expected structure.
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Equal(t, "bad request <foo> & <bar>", result["error"])
	assert.Equal(t, "try `cmd <topic>` & retry", result["hint"])
}
