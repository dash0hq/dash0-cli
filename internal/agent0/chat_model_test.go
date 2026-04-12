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

	m.textarea.SetValue("Hello agent0")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(chatModel)

	require.Len(t, m.messages, 1)
	assert.Equal(t, RoleHuman, m.messages[0].role)
	assert.Equal(t, "Hello agent0", m.messages[0].content)
	assert.Empty(t, m.messages[0].apiID, "user-submitted messages have no API ID")
	assert.True(t, m.streaming)
	assert.Equal(t, "Sending...", m.statusText)
	assert.Empty(t, m.textarea.Value())
	assert.NotNil(t, cmd)
}

func TestChatModelSubmitEmptyIgnored(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})

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

	mockStream := NewSSEStream(http.NoBody)
	cancelCalled := false
	cancel := func() { cancelCalled = true }

	updated, cmd := m.Update(sseStreamOpenedMsg{stream: mockStream, cancel: cancel})
	m = updated.(chatModel)

	assert.Equal(t, mockStream, m.activeStream)
	assert.NotNil(t, cmd, "should return cmd to read next event")
	assert.False(t, cancelCalled)
}

func TestChatModelSnapshotNoDuplicateHumanMessage(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})

	// User sends a message
	m.appendUserMessage("Hello")
	m.streaming = true
	m.activeStream = NewSSEStream(http.NoBody)

	// SSE snapshot contains the human message + assistant response
	snapshot := &InvokeResponse{
		Thread: Thread{ID: "thread-1"},
		Messages: []Message{
			{Role: RoleHuman, ID: "h1", Hash: "hh1", Content: "Hello"},
			{Role: RoleAssistant, ID: "a1", Hash: "ah1", Content: "Hi there!"},
		},
	}

	updated, _ := m.Update(sseSnapshotMsg{resp: snapshot})
	m = updated.(chatModel)

	// Should have exactly 2 messages: the user's original + assistant.
	// Human from SSE is skipped.
	require.Len(t, m.messages, 2, "human message should not be duplicated")
	assert.Equal(t, RoleHuman, m.messages[0].role)
	assert.Equal(t, RoleAssistant, m.messages[1].role)
	assert.Equal(t, "Hi there!", m.messages[1].content)
}

func TestChatModelSnapshotUpdatesAssistantInPlace(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})
	m.streaming = true
	m.activeStream = NewSSEStream(http.NoBody)

	// First snapshot: partial assistant message
	snap1 := &InvokeResponse{
		Thread: Thread{ID: "t1"},
		Messages: []Message{
			{Role: RoleHuman, ID: "h1", Hash: "hh1", Content: "Hello"},
			{Role: RoleAssistant, ID: "a1", Hash: "ah1", Content: "Hi"},
		},
	}
	updated, _ := m.Update(sseSnapshotMsg{resp: snap1})
	m = updated.(chatModel)
	require.Len(t, m.messages, 1) // Only assistant (human skipped)
	assert.Equal(t, "Hi", m.messages[0].content)
	assert.Equal(t, "a1", m.messages[0].apiID)

	// Second snapshot: assistant content grew, hash changed (content-based)
	snap2 := &InvokeResponse{
		Thread: Thread{ID: "t1"},
		Messages: []Message{
			{Role: RoleHuman, ID: "h1", Hash: "hh1", Content: "Hello"},
			{Role: RoleAssistant, ID: "a1", Hash: "ah2-changed", Content: "Hi there, how can I help?"},
		},
	}
	updated, _ = m.Update(sseSnapshotMsg{resp: snap2})
	m = updated.(chatModel)

	// Still 1 message — updated in place by ID, not duplicated.
	require.Len(t, m.messages, 1, "assistant message should be updated in place, not duplicated")
	assert.Equal(t, "Hi there, how can I help?", m.messages[0].content)
}

func TestChatModelSnapshotMultipleUpdatesNoDuplication(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})
	m.streaming = true
	m.activeStream = NewSSEStream(http.NoBody)

	// Simulate 5 progressive snapshots with growing content and changing hashes
	for i := 1; i <= 5; i++ {
		content := fmt.Sprintf("Response part %d", i)
		hash := fmt.Sprintf("hash-%d", i)
		snap := &InvokeResponse{
			Thread: Thread{ID: "t1"},
			Messages: []Message{
				{Role: RoleHuman, ID: "h1", Hash: "hh1", Content: "Hello"},
				{Role: RoleAssistant, ID: "a1", Hash: hash, Content: content},
			},
		}
		updated, _ := m.Update(sseSnapshotMsg{resp: snap})
		m = updated.(chatModel)
	}

	// Should still be exactly 1 assistant message, with the latest content.
	require.Len(t, m.messages, 1, "should have exactly 1 message after 5 updates")
	assert.Equal(t, "Response part 5", m.messages[0].content)
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
	assert.Nil(t, cmd)
}

func TestChatModelCtrlCWhenIdleQuits(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = updated.(chatModel)

	assert.True(t, m.quitting)
	assert.NotNil(t, cmd)
}

func TestChatModelCtrlDQuits(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = updated.(chatModel)

	assert.True(t, m.quitting)
	assert.NotNil(t, cmd)
}

func TestChatModelScrollKeysRouteToViewport(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})

	// Add enough content to make viewport scrollable
	for i := 0; i < 30; i++ {
		m.appendMessage(RoleAssistant, fmt.Sprintf("Line %d of response", i), time.Now())
	}
	m.updateViewportContent()

	// PgUp/PgDown should work
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m = updated.(chatModel)
	assert.True(t, m.ready)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = updated.(chatModel)
	assert.True(t, m.ready)

	// Up/Down should scroll when textarea is empty
	assert.Empty(t, m.textarea.Value())
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(chatModel)
	assert.True(t, m.ready)
}

func TestChatModelArrowKeysGoToTextareaWhenTyping(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})
	m.textarea.SetValue("some text")

	// Up/Down should go to textarea (not viewport) when textarea has content
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(chatModel)
	// The key was NOT consumed by scroll (it went to textarea)
	assert.True(t, m.ready)
}

func TestChatModelScrollKeysDuringStreaming(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})
	m.streaming = true

	// All scroll keys should work during streaming
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m = updated.(chatModel)
	assert.True(t, m.streaming, "scrolling should not affect streaming state")

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(chatModel)
	assert.True(t, m.streaming, "arrow scroll should work during streaming")
}

func TestChatModelThreadLoaded(t *testing.T) {
	m := initModel(t, &mockAgent0Client{}, chatConfig{dataset: "default"})

	resp := &ThreadResponse{
		Thread: Thread{ID: "t1", Name: "Test thread"},
		Messages: []Message{
			{Role: RoleHuman, ID: "h1", Hash: "hh1", Content: "Hello"},
			{Role: RoleAssistant, ID: "a1", Hash: "ah1", Content: "Hi!"},
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		fmt.Fprint(w, "event: status\ndata: {\"status\":\"preparing\"}\n\n")
		flusher.Flush()

		fmt.Fprint(w, "data: {\"thread\":{\"id\":\"t-e2e\"},\"messages\":[{\"role\":\"human\",\"id\":\"h1\",\"hash\":\"hh1\",\"content\":\"test\"},{\"role\":\"assistant\",\"id\":\"a1\",\"hash\":\"ah1\",\"content\":\"Hello from mock!\"}]}\n\n")
		flusher.Flush()

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
	require.Len(t, m.messages, 1, "should have user message")
	assert.True(t, m.streaming)
	require.NotNil(t, cmd)

	// Execute cmd -> sseStreamOpenedMsg
	msg := executeCmd(t, cmd)
	openMsg, ok := msg.(sseStreamOpenedMsg)
	require.True(t, ok, "expected sseStreamOpenedMsg, got %T", msg)

	// Handle stream opened
	updated, cmd = m.Update(openMsg)
	m = updated.(chatModel)
	require.NotNil(t, cmd)

	// Read next -> status
	msg = executeCmd(t, cmd)
	statusMsg, ok := msg.(sseStatusMsg)
	require.True(t, ok, "expected sseStatusMsg, got %T", msg)

	updated, cmd = m.Update(statusMsg)
	m = updated.(chatModel)
	assert.Equal(t, "Preparing sandbox...", m.statusText)

	// Read next -> snapshot
	msg = executeCmd(t, cmd)
	snapMsg, ok := msg.(sseSnapshotMsg)
	require.True(t, ok, "expected sseSnapshotMsg, got %T", msg)

	updated, cmd = m.Update(snapMsg)
	m = updated.(chatModel)
	assert.Equal(t, "t-e2e", m.threadID)
	require.Len(t, m.messages, 2, "should have user + assistant")
	assert.Equal(t, RoleHuman, m.messages[0].role)
	assert.Equal(t, RoleAssistant, m.messages[1].role)
	assert.Equal(t, "Hello from mock!", m.messages[1].content)

	// Read next -> done
	msg = executeCmd(t, cmd)
	_, ok = msg.(sseDoneMsg)
	require.True(t, ok, "expected sseDoneMsg, got %T", msg)

	updated, _ = m.Update(sseDoneMsg{})
	m = updated.(chatModel)
	assert.False(t, m.streaming)
	assert.Contains(t, m.statusText, "t-e2e")
}

func TestExtractToolActivityInProgress(t *testing.T) {
	messages := []Message{
		{Role: RoleHuman, ID: "h1", Content: "test"},
		{Role: RoleTool, ID: "t1", Why: "Find error logs in api service", StartedAt: timePtr(time.Now())},
	}
	assert.Equal(t, "Find error logs in api service", extractToolActivity(messages))
}

func TestExtractToolActivityCompleted(t *testing.T) {
	now := time.Now()
	messages := []Message{
		{Role: RoleTool, ID: "t1", Why: "Resolve time range", StartedAt: &now, EndedAt: &now},
	}
	assert.Equal(t, "Resolve time range ✓", extractToolActivity(messages))
}

func TestExtractToolActivityPrefersInProgress(t *testing.T) {
	now := time.Now()
	messages := []Message{
		{Role: RoleTool, ID: "t1", Why: "Resolve time range", StartedAt: &now, EndedAt: &now},
		{Role: RoleTool, ID: "t2", Why: "Find error logs", StartedAt: &now},
	}
	assert.Equal(t, "Find error logs", extractToolActivity(messages))
}

func TestExtractToolActivityNoTools(t *testing.T) {
	messages := []Message{
		{Role: RoleHuman, ID: "h1", Content: "test"},
		{Role: RoleAssistant, ID: "a1", Content: "response"},
	}
	assert.Equal(t, "", extractToolActivity(messages))
}

func timePtr(t time.Time) *time.Time { return &t }

// executeCmd runs a tea.Cmd synchronously and returns the result message.
func executeCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()

	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				result := c()
				if result != nil {
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
