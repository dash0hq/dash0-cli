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
