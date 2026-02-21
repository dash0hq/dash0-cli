package tracing

import (
	"testing"

	"github.com/dash0hq/dash0-cli/internal/otlp"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSpansSendCmd() (*cobra.Command, *cobra.Command) {
	root := &cobra.Command{Use: "dash0"}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	spansCmd := NewSpansCmd()
	root.AddCommand(spansCmd)
	var sendCmd *cobra.Command
	for _, c := range spansCmd.Commands() {
		if c.Name() == "send" {
			sendCmd = c
			break
		}
	}
	return root, sendCmd
}

func TestSendRequiresExperimentalFlag(t *testing.T) {
	root, _ := newSpansSendCmd()
	root.SetArgs([]string{"spans", "send", "--name", "test"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental command")
}

func TestSendRequiresName(t *testing.T) {
	root, _ := newSpansSendCmd()
	root.SetArgs([]string{"-X", "spans", "send"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required flag")
	assert.Contains(t, err.Error(), "name")
}

func TestSendEndTimeAndDurationMutuallyExclusive(t *testing.T) {
	root, _ := newSpansSendCmd()
	root.SetArgs([]string{
		"-X", "spans", "send",
		"--name", "test",
		"--end-time", "2024-03-15T10:30:00Z",
		"--duration", "100ms",
	})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestSendInvalidKind(t *testing.T) {
	root, _ := newSpansSendCmd()
	root.SetArgs([]string{"-X", "spans", "send", "--name", "test", "--kind", "INVALID"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown span kind")
}

func TestSendInvalidStatusCode(t *testing.T) {
	root, _ := newSpansSendCmd()
	root.SetArgs([]string{"-X", "spans", "send", "--name", "test", "--status-code", "BAD"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown status code")
}

func TestParseSpanLink(t *testing.T) {
	t.Run("valid link without attributes", func(t *testing.T) {
		link, err := parseSpanLink("0af7651916cd43dd8448eb211c80319c:b7ad6b7169203331")
		require.NoError(t, err)
		tid, _ := otlp.ParseTraceID("0af7651916cd43dd8448eb211c80319c")
		sid, _ := otlp.ParseSpanID("b7ad6b7169203331")
		assert.Equal(t, tid, link.traceID)
		assert.Equal(t, sid, link.spanID)
		assert.Nil(t, link.attributes)
	})

	t.Run("valid link with attributes", func(t *testing.T) {
		link, err := parseSpanLink("0af7651916cd43dd8448eb211c80319c:b7ad6b7169203331,link.reason=follows-from")
		require.NoError(t, err)
		assert.Equal(t, "follows-from", link.attributes["link.reason"])
	})

	t.Run("invalid format", func(t *testing.T) {
		_, err := parseSpanLink("invalid")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected format")
	})

	t.Run("invalid trace id", func(t *testing.T) {
		_, err := parseSpanLink("short:b7ad6b7169203331")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "32 hex characters")
	})
}

func TestGenerateIDs(t *testing.T) {
	tid1 := generateTraceID()
	tid2 := generateTraceID()
	assert.NotEqual(t, tid1, tid2, "generated trace IDs should be unique")

	sid1 := generateSpanID()
	sid2 := generateSpanID()
	assert.NotEqual(t, sid1, sid2, "generated span IDs should be unique")
}
