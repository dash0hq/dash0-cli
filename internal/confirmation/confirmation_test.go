package confirmation

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

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
	confirmed, err := ConfirmDestructiveOperation(context.Background(), "delete?", true)
	require.NoError(t, err)
	assert.True(t, confirmed)
}

func TestAgentModeSkipsPrompt(t *testing.T) {
	withAgentMode(t, true)

	confirmed, err := ConfirmDestructiveOperation(context.Background(), "delete?", false)
	require.NoError(t, err)
	assert.True(t, confirmed)
}

func TestConfirmsWithY(t *testing.T) {
	withAgentMode(t, false)
	withStdin(t, "y\n")

	confirmed, err := ConfirmDestructiveOperation(context.Background(), "delete?", false)
	require.NoError(t, err)
	assert.True(t, confirmed)
}

func TestConfirmsWithYes(t *testing.T) {
	withAgentMode(t, false)
	withStdin(t, "yes\n")

	confirmed, err := ConfirmDestructiveOperation(context.Background(), "delete?", false)
	require.NoError(t, err)
	assert.True(t, confirmed)
}

func TestConfirmsCaseInsensitive(t *testing.T) {
	withAgentMode(t, false)
	withStdin(t, "YES\n")

	confirmed, err := ConfirmDestructiveOperation(context.Background(), "delete?", false)
	require.NoError(t, err)
	assert.True(t, confirmed)
}

func TestDeclinesWithN(t *testing.T) {
	withAgentMode(t, false)
	withStdin(t, "n\n")

	confirmed, err := ConfirmDestructiveOperation(context.Background(), "delete?", false)
	require.NoError(t, err)
	assert.False(t, confirmed)
}

func TestDeclinesWithEmptyInput(t *testing.T) {
	withAgentMode(t, false)
	withStdin(t, "\n")

	confirmed, err := ConfirmDestructiveOperation(context.Background(), "delete?", false)
	require.NoError(t, err)
	assert.False(t, confirmed)
}

func TestReturnsErrorOnReadFailure(t *testing.T) {
	withAgentMode(t, false)
	prev := reader
	reader = &errReader{}
	t.Cleanup(func() { reader = prev })

	_, err := ConfirmDestructiveOperation(context.Background(), "delete?", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read response")
}

// TestCtxCancelInterruptsPrompt verifies the round-3 regression fix: a
// cancelled context unblocks the prompt instead of leaving the reader
// stuck forever. Pre-fix, signal.NotifyContext would catch SIGINT but the
// blocking bufio read would keep waiting until SIGKILL.
func TestCtxCancelInterruptsPrompt(t *testing.T) {
	withAgentMode(t, false)
	// blockingReader never returns from Read until closed; mimics stdin
	// waiting on the user.
	prev := reader
	bd := newBlockingReader()
	reader = bd
	t.Cleanup(func() {
		bd.Close()
		reader = prev
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var (
		confirmed bool
		gotErr    error
	)
	go func() {
		confirmed, gotErr = ConfirmDestructiveOperation(ctx, "delete?", false)
		close(done)
	}()

	// Give the goroutine a moment to enter the select.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ConfirmDestructiveOperation did not return after ctx cancel")
	}

	assert.False(t, confirmed)
	assert.True(t, errors.Is(gotErr, context.Canceled), "expected context.Canceled, got %v", gotErr)
}

type errReader struct{}

func (e *errReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

// blockingReader simulates os.Stdin: Read blocks until Close is called.
type blockingReader struct {
	closed chan struct{}
}

func newBlockingReader() *blockingReader {
	return &blockingReader{closed: make(chan struct{})}
}

func (b *blockingReader) Read(p []byte) (int, error) {
	<-b.closed
	return 0, io.EOF
}

func (b *blockingReader) Close() { close(b.closed) }
