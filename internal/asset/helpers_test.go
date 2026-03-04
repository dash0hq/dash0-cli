package asset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string {
	return &s
}

// assertJSONEqual marshals both values to JSON and compares them
// semantically. This is the right comparison for YAML round-trip tests
// because union types (json.RawMessage) may have different key ordering
// or whitespace after the roundtrip while still being semantically equal.
func assertJSONEqual(t *testing.T, expected, actual interface{}) {
	t.Helper()
	expectedJSON, err := json.Marshal(expected)
	require.NoError(t, err)
	actualJSON, err := json.Marshal(actual)
	require.NoError(t, err)
	assert.JSONEq(t, string(expectedJSON), string(actualJSON))
}

func readFixture(t *testing.T, relativePath string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testutil.FixturesDir(), relativePath))
	require.NoError(t, err)
	return data
}
