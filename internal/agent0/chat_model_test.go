package agent0

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAgent0Client implements Agent0Client for testing.
type mockAgent0Client struct {
	invokeHandler func(ctx context.Context, req *InvokeRequest) (*http.Response, error)
	getThread     func(ctx context.Context, threadID string) (*ThreadResponse, error)
}

func (c *mockAgent0Client) InvokeAgent0(ctx context.Context, req *InvokeRequest) (*http.Response, error) {
	if c.invokeHandler != nil {
		return c.invokeHandler(ctx, req)
	}
	return nil, fmt.Errorf("not implemented")
}

func (c *mockAgent0Client) GetAgent0Thread(ctx context.Context, threadID string) (*ThreadResponse, error) {
	if c.getThread != nil {
		return c.getThread(ctx, threadID)
	}
	return nil, fmt.Errorf("not implemented")
}

func (c *mockAgent0Client) CancelAgent0(_ context.Context, _ string) error {
	return nil
}

// initModel creates a chat model and sends a WindowSizeMsg to initialize it.
func initModel(t *testing.T, client Agent0Client, cfg chatConfig) chatModel {
	t.Helper()
	m := newChatModel(client, cfg)
	// Simulate initial window size
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return updated.(chatModel)
}

func TestChatModelInitialState(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})

	assert.True(t, m.ready)
	assert.False(t, m.streaming)
	assert.Equal(t, "Ready", m.statusText)
	assert.Empty(t, m.messages)
	assert.Equal(t, 80, m.width)
	assert.Equal(t, 24, m.height)
}

func TestChatModelResize(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})

	// Resize to smaller
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	m = updated.(chatModel)

	assert.Equal(t, 40, m.width)
	assert.Equal(t, 12, m.height)
	assert.True(t, m.ready)
}

func TestChatModelSubmitAddsUserMessage(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{
		dataset:      "default",
		networkLevel: "trusted_only",
	})

	// Type a message
	m.textarea.SetValue("Hello agent0")

	// Press Enter
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(chatModel)

	// User message should be displayed
	require.Len(t, m.messages, 1)
	assert.Equal(t, RoleHuman, m.messages[0].role)
	assert.Equal(t, "Hello agent0", m.messages[0].content)

	// Should be streaming
	assert.True(t, m.streaming)
	assert.Equal(t, "Sending...", m.statusText)

	// Textarea should be cleared
	assert.Empty(t, m.textarea.Value())

	// A command should be returned (starts SSE stream + spinner)
	assert.NotNil(t, cmd)
}

func TestChatModelSubmitEmptyIgnored(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})

	// Press Enter with empty textarea
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(chatModel)

	assert.Empty(t, m.messages)
	assert.False(t, m.streaming)
}

func TestChatModelSubmitWhileStreamingIgnored(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})
	m.streaming = true
	m.textarea.SetValue("another question")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(chatModel)

	assert.Empty(t, m.messages)
	assert.True(t, m.streaming)
}

func TestChatModelSSEStreamOpened(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})
	m.streaming = true

	// Simulate stream opened
	mockStream := NewSSEStream(http.NoBody)
	cancelCalled := false
	cancel := func() { cancelCalled = true }

	updated, cmd := m.Update(sseStreamOpenedMsg{stream: mockStream, cancel: cancel})
	m = updated.(chatModel)

	assert.Equal(t, mockStream, m.activeStream)
	assert.NotNil(t, cmd, "should return cmd to read next event")
	assert.False(t, cancelCalled)
}

func TestChatModelSSESnapshotNoDuplicateHumanMessage(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})

	// User sends a message (adds human message to display)
	m.appendMessage(RoleHuman, "Hello", time.Now())
	m.streaming = true
	m.activeStream = NewSSEStream(http.NoBody) // Dummy for readNextSSEEvent

	// SSE snapshot arrives containing the same human message + assistant response
	snapshot := &InvokeResponse{
		Thread: Thread{ID: "thread-1"},
		Messages: []Message{
			{Role: RoleHuman, Hash: "h1", Content: "Hello"},
			{Role: RoleAssistant, Hash: "a1", Content: "Hi there!"},
		},
	}

	updated, _ := m.Update(sseSnapshotMsg{resp: snapshot})
	m = updated.(chatModel)

	// Should have exactly 2 messages: the user's and the assistant's.
	// The human message from SSE should NOT create a duplicate.
	require.Len(t, m.messages, 2, "human message should not be duplicated")
	assert.Equal(t, RoleHuman, m.messages[0].role)
	assert.Equal(t, "Hello", m.messages[0].content)
	assert.Equal(t, RoleAssistant, m.messages[1].role)
	assert.Equal(t, "Hi there!", m.messages[1].content)
}

func TestChatModelSSESnapshotUpdatesAssistant(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})
	m.streaming = true
	m.activeStream = NewSSEStream(http.NoBody)

	// First snapshot: partial assistant message
	snap1 := &InvokeResponse{
		Thread: Thread{ID: "t1"},
		Messages: []Message{
			{Role: RoleHuman, Hash: "h1", Content: "Hello"},
			{Role: RoleAssistant, Hash: "a1", Content: "Hi"},
		},
	}
	updated, _ := m.Update(sseSnapshotMsg{resp: snap1})
	m = updated.(chatModel)
	require.Len(t, m.messages, 1) // Only assistant (human skipped)
	assert.Equal(t, "Hi", m.messages[0].content)

	// Second snapshot: assistant message grew
	snap2 := &InvokeResponse{
		Thread: Thread{ID: "t1"},
		Messages: []Message{
			{Role: RoleHuman, Hash: "h1", Content: "Hello"},
			{Role: RoleAssistant, Hash: "a1", Content: "Hi there, how can I help?"},
		},
	}
	updated, _ = m.Update(sseSnapshotMsg{resp: snap2})
	m = updated.(chatModel)

	// Should still have 1 message, but with updated content
	require.Len(t, m.messages, 1)
	assert.Equal(t, "Hi there, how can I help?", m.messages[0].content)
}

func TestChatModelSSEDoneStopsStreaming(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})
	m.streaming = true
	m.threadID = "t1"
	m.activeStream = NewSSEStream(http.NoBody)

	updated, _ := m.Update(sseDoneMsg{})
	m = updated.(chatModel)

	assert.False(t, m.streaming)
	assert.Nil(t, m.activeStream)
	assert.Contains(t, m.statusText, "Thread: t1")
}

func TestChatModelSSEErrorStopsStreaming(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})
	m.streaming = true
	m.activeStream = NewSSEStream(http.NoBody)

	updated, _ := m.Update(sseErrorMsg{err: fmt.Errorf("connection lost")})
	m = updated.(chatModel)

	assert.False(t, m.streaming)
	assert.Nil(t, m.activeStream)
	assert.Equal(t, "Error", m.statusText)
	require.Len(t, m.messages, 1)
	assert.Equal(t, RoleError, m.messages[0].role)
	assert.Contains(t, m.messages[0].content, "connection lost")
}

func TestChatModelCtrlCDuringStreamingCancels(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})
	m.streaming = true
	cancelCalled := false
	m.streamCancel = func() { cancelCalled = true }
	m.activeStream = NewSSEStream(http.NoBody)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = updated.(chatModel)

	assert.False(t, m.streaming)
	assert.Equal(t, "Cancelled", m.statusText)
	assert.True(t, cancelCalled)
	assert.Nil(t, cmd, "should not return a quit command")
}

func TestChatModelCtrlCWhenIdleQuits(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	// Should return tea.Quit
	assert.NotNil(t, cmd)
}

func TestChatModelCtrlDQuits(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = updated.(chatModel)

	assert.True(t, m.quitting)
	assert.NotNil(t, cmd)
}

func TestChatModelThreadLoaded(t *testing.T) {
	now := time.Now()
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})

	resp := &ThreadResponse{
		Thread: Thread{ID: "t1", Name: "Test thread"},
		Messages: []Message{
			{Role: RoleHuman, Hash: "h1", Content: "Hello", StartedAt: &now},
			{Role: RoleAssistant, Hash: "a1", Content: "Hi!", StartedAt: &now},
		},
	}

	updated, _ := m.Update(threadLoadedMsg{resp: resp})
	m = updated.(chatModel)

	assert.Equal(t, "t1", m.threadID)
	require.Len(t, m.messages, 2)
	assert.Equal(t, RoleHuman, m.messages[0].role)
	assert.Equal(t, RoleAssistant, m.messages[1].role)
}

func TestChatModelThreadLoadError(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})

	updated, _ := m.Update(threadLoadErrMsg{err: fmt.Errorf("not found")})
	m = updated.(chatModel)

	require.Len(t, m.messages, 1)
	assert.Equal(t, RoleError, m.messages[0].role)
	assert.Contains(t, m.messages[0].content, "not found")
}

// TestChatModelE2EWithMockServer tests the full flow: submit -> SSE stream -> response.
func TestChatModelE2EWithMockServer(t *testing.T) {
	// Set up a mock SSE server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		// Status event
		fmt.Fprint(w, "event: status\ndata: {\"status\":\"preparing\"}\n\n")
		flusher.Flush()

		// Snapshot with human + assistant
		fmt.Fprint(w, "data: {\"thread\":{\"id\":\"t-e2e\"},\"messages\":[{\"role\":\"human\",\"hash\":\"h1\",\"content\":\"test\",\"id\":\"1\"},{\"role\":\"assistant\",\"hash\":\"a1\",\"content\":\"Hello from mock!\",\"id\":\"2\"}]}\n\n")
		flusher.Flush()

		// Done
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	client := &httpAgent0Client{
		apiURL:    server.URL,
		authToken: "test-token",
	}

	m := initModel(t, client, chatConfig{
		dataset:      "default",
		networkLevel: "trusted_only",
	})

	// User types and submits
	m.textarea.SetValue("test")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(chatModel)

	require.Len(t, m.messages, 1) // User message
	assert.True(t, m.streaming)
	require.NotNil(t, cmd)

	// Execute the command (opens SSE stream)
	msg := executeCmd(t, cmd)

	// Should be sseStreamOpenedMsg
	openMsg, ok := msg.(sseStreamOpenedMsg)
	require.True(t, ok, "expected sseStreamOpenedMsg, got %T", msg)
	require.NotNil(t, openMsg.stream)

	// Handle stream opened
	updated, cmd = m.Update(openMsg)
	m = updated.(chatModel)
	require.NotNil(t, cmd, "should return cmd to read next event")

	// Read next event (status)
	msg = executeCmd(t, cmd)
	statusMsg, ok := msg.(sseStatusMsg)
	require.True(t, ok, "expected sseStatusMsg, got %T", msg)
	assert.Equal(t, "preparing", statusMsg.status)

	// Handle status
	updated, cmd = m.Update(statusMsg)
	m = updated.(chatModel)
	assert.Equal(t, "Preparing sandbox...", m.statusText)

	// Read next event (snapshot)
	msg = executeCmd(t, cmd)
	snapMsg, ok := msg.(sseSnapshotMsg)
	require.True(t, ok, "expected sseSnapshotMsg, got %T", msg)

	// Handle snapshot
	updated, cmd = m.Update(snapMsg)
	m = updated.(chatModel)
	assert.Equal(t, "t-e2e", m.threadID)
	// Should have user message + assistant message (human from SSE is skipped)
	require.Len(t, m.messages, 2)
	assert.Equal(t, RoleHuman, m.messages[0].role)
	assert.Equal(t, RoleAssistant, m.messages[1].role)
	assert.Equal(t, "Hello from mock!", m.messages[1].content)

	// Read next event (done)
	msg = executeCmd(t, cmd)
	_, ok = msg.(sseDoneMsg)
	require.True(t, ok, "expected sseDoneMsg, got %T", msg)

	// Handle done
	updated, _ = m.Update(sseDoneMsg{})
	m = updated.(chatModel)
	assert.False(t, m.streaming)
	assert.Contains(t, m.statusText, "t-e2e")
}

// executeCmd runs a tea.Cmd synchronously and returns the result message.
// Handles tea.BatchMsg by finding the first non-nil message from the batch.
func executeCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()

	// tea.Batch returns a special type; unwrap it
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				result := c()
				if result != nil {
					// Return the first meaningful result, skip spinner ticks
					switch result.(type) {
					case sseStreamOpenedMsg, sseSnapshotMsg, sseStatusMsg, sseDoneMsg, sseErrorMsg:
						return result
					}
				}
			}
		}
		t.Fatal("batch contained no meaningful messages")
	}

	return msg
}
