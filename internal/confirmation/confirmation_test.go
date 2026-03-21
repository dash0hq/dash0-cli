package confirmation

import (
	"io"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withStdin(t *testing.T, input string) {
	t.Helper()
	prev := reader
	reader = strings.NewReader(input)
	t.Cleanup(func() { reader = prev })
}

func withAgentMode(t *testing.T, enabled bool) {
	t.Helper()
	prev := agentmode.Enabled
	agentmode.Enabled = enabled
	t.Cleanup(func() { agentmode.Enabled = prev })
}

func TestForceSkipsPrompt(t *testing.T) {
	confirmed, err := ConfirmDestructiveOperation("delete?", true)
	require.NoError(t, err)
	assert.True(t, confirmed)
}

func TestAgentModeSkipsPrompt(t *testing.T) {
	withAgentMode(t, true)

	confirmed, err := ConfirmDestructiveOperation("delete?", false)
	require.NoError(t, err)
	assert.True(t, confirmed)
}

func TestConfirmsWithY(t *testing.T) {
	withAgentMode(t, false)
	withStdin(t, "y\n")

	confirmed, err := ConfirmDestructiveOperation("delete?", false)
	require.NoError(t, err)
	assert.True(t, confirmed)
}

func TestConfirmsWithYes(t *testing.T) {
	withAgentMode(t, false)
	withStdin(t, "yes\n")

	confirmed, err := ConfirmDestructiveOperation("delete?", false)
	require.NoError(t, err)
	assert.True(t, confirmed)
}

func TestConfirmsCaseInsensitive(t *testing.T) {
	withAgentMode(t, false)
	withStdin(t, "YES\n")

	confirmed, err := ConfirmDestructiveOperation("delete?", false)
	require.NoError(t, err)
	assert.True(t, confirmed)
}

func TestDeclinesWithN(t *testing.T) {
	withAgentMode(t, false)
	withStdin(t, "n\n")

	confirmed, err := ConfirmDestructiveOperation("delete?", false)
	require.NoError(t, err)
	assert.False(t, confirmed)
}

func TestDeclinesWithEmptyInput(t *testing.T) {
	withAgentMode(t, false)
	withStdin(t, "\n")

	confirmed, err := ConfirmDestructiveOperation("delete?", false)
	require.NoError(t, err)
	assert.False(t, confirmed)
}

func TestReturnsErrorOnReadFailure(t *testing.T) {
	withAgentMode(t, false)
	prev := reader
	reader = &errReader{}
	t.Cleanup(func() { reader = prev })

	_, err := ConfirmDestructiveOperation("delete?", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read response")
}

type errReader struct{}

func (e *errReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
