package query

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintExplorerURL(t *testing.T) {
	var buf bytes.Buffer
	PrintExplorerURL(&buf, ExplorerLabelQuery, "https://app.dash0.com/goto/logs?from=now-15m&to=now")

	out := buf.String()
	assert.Equal(t,
		"\nOpen this query in Dash0:\n    https://app.dash0.com/goto/logs?from=now-15m&to=now\n",
		out,
		"the link is preceded by a blank line and indented by four spaces")
}

func TestPrintExplorerURL_TraceLabel(t *testing.T) {
	var buf bytes.Buffer
	PrintExplorerURL(&buf, ExplorerLabelTrace, "https://app.dash0.com/goto/traces/explorer?trace_id=abc")

	assert.Contains(t, buf.String(), "Open this trace in Dash0:")
}

// An empty URL is what the builders return when the API URL is unset or cannot
// be parsed. Nothing must be written then — not even the leading blank line,
// which would otherwise add a stray line to every such invocation.
func TestPrintExplorerURL_EmptyWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	PrintExplorerURL(&buf, ExplorerLabelQuery, "")

	assert.Empty(t, buf.String())
}

func TestAddExplorerURL(t *testing.T) {
	wrapper := AddExplorerURL(map[string]any{"resourceLogs": []any{}}, "https://app.dash0.com/goto/logs")

	assert.Equal(t, "https://app.dash0.com/goto/logs", wrapper[ExplorerURLKey])
	assert.Contains(t, wrapper, "resourceLogs", "the payload itself is left untouched")
}

// The key must be absent rather than present-and-empty, so a consumer can treat
// its presence as "a link is available".
func TestAddExplorerURL_EmptyOmitsKey(t *testing.T) {
	wrapper := AddExplorerURL(map[string]any{"resourceLogs": []any{}}, "")

	assert.NotContains(t, wrapper, ExplorerURLKey)
}

// The key sits alongside the OTLP payload rather than wrapping it, so existing
// consumers reading `.resourceLogs` keep working.
func TestAddExplorerURL_IsSiblingOfPayload(t *testing.T) {
	wrapper := AddExplorerURL(map[string]any{"resourceLogs": []any{}}, "https://app.dash0.com/goto/logs")

	encoded, err := json.Marshal(wrapper)
	require.NoError(t, err)

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &decoded))

	assert.Contains(t, decoded, "resourceLogs")
	assert.Contains(t, decoded, ExplorerURLKey)
}

// The key is namespaced because it is a Dash0 extension to an otherwise
// OTLP/JSON document.
func TestExplorerURLKeyIsNamespaced(t *testing.T) {
	assert.True(t, strings.HasPrefix(ExplorerURLKey, "dash0"),
		"an unprefixed key could collide with a field OTLP introduces later")
}
