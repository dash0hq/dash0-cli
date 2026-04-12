package agent0

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
)

// Agent0Client is the interface for agent0 API calls used by the chat model.
type Agent0Client interface {
	// InvokeAgent0 sends a query and returns the raw SSE HTTP response.
	InvokeAgent0(ctx context.Context, req *InvokeRequest) (*http.Response, error)
	// GetAgent0Thread retrieves a thread by ID.
	GetAgent0Thread(ctx context.Context, threadID string) (*ThreadResponse, error)
	// CancelAgent0 cancels an active session.
	CancelAgent0(ctx context.Context, threadID string) error
}

// chatConfig holds resolved configuration for the chat session.
type chatConfig struct {
	apiURL       string
	authToken    string
	dataset      string
	threadID     string
	networkLevel string
	verbose      bool
}

// displayMessage holds a rendered message for the viewport.
type displayMessage struct {
	role      string
	content   string
	rendered  string
	timestamp time.Time
}

// chatModel is the bubbletea model for the interactive chat TUI.
type chatModel struct {
	// Conversation state
	messages     []displayMessage
	seenHashes   map[string]bool // Tracks message hashes we've already displayed
	threadID     string
	streaming    bool
	lastSnapshot *InvokeResponse
	activeStream *SSEStream
	streamCancel context.CancelFunc

	// UI components
	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model
	width    int
	height   int

	// Dependencies
	client     Agent0Client
	mdRenderer *glamour.TermRenderer
	cfg        chatConfig

	// State
	quitting   bool
	statusText string
	ready      bool // True once initial WindowSizeMsg is received
}

// newChatModel creates a new chat model with the given client and config.
func newChatModel(client Agent0Client, cfg chatConfig) chatModel {
	ta := textarea.New()
	ta.Placeholder = "Ask Agent0..."
	ta.CharLimit = 4096
	ta.SetHeight(3)
	ta.Focus()
	ta.ShowLineNumbers = false

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return chatModel{
		textarea:   ta,
		spinner:    sp,
		seenHashes: make(map[string]bool),
		client:     client,
		cfg:        cfg,
		statusText: "Ready",
	}
}

// -- Bubbletea message types --

type sseSnapshotMsg struct{ resp *InvokeResponse }
type sseStatusMsg struct{ status string }
type sseDoneMsg struct{}
type sseErrorMsg struct{ err error }
type threadLoadedMsg struct{ resp *ThreadResponse }
type threadLoadErrMsg struct{ err error }

// sseStreamOpenedMsg is sent when the SSE HTTP connection is established.
// It carries the stream and cancel func so they can be stored in the model
// inside the Update handler (not in a cmd closure, which would be lost).
type sseStreamOpenedMsg struct {
	stream *SSEStream
	cancel context.CancelFunc
}

// -- tea.Model interface --

func (m chatModel) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink}
	if m.cfg.threadID != "" {
		cmds = append(cmds, m.loadExistingThread())
	}
	return tea.Batch(cmds...)
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		return m.handleResize(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		if m.streaming {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case sseStreamOpenedMsg:
		// Store the stream and cancel func in the model (this is the canonical
		// place to mutate state — inside Update, not inside a tea.Cmd closure).
		m.activeStream = msg.stream
		m.streamCancel = msg.cancel
		return m, m.readNextSSEEvent()

	case sseStatusMsg:
		m.statusText = formatStatus(msg.status)
		return m, m.readNextSSEEvent()

	case sseSnapshotMsg:
		deltas := DiffSnapshots(m.lastSnapshot, msg.resp)
		m.lastSnapshot = msg.resp

		if m.threadID == "" && msg.resp.Thread.ID != "" {
			m.threadID = msg.resp.Thread.ID
		}

		for _, d := range deltas {
			m.processDelta(d)
		}
		m.updateViewportContent()
		m.statusText = "Thinking..."
		return m, m.readNextSSEEvent()

	case sseDoneMsg:
		m.streaming = false
		m.activeStream = nil
		if m.threadID != "" {
			m.statusText = fmt.Sprintf("Thread: %s", m.threadID)
		} else {
			m.statusText = "Ready"
		}
		m.textarea.Focus()
		m.updateViewportContent()
		return m, nil

	case sseErrorMsg:
		m.streaming = false
		if m.activeStream != nil {
			m.activeStream.Close()
			m.activeStream = nil
		}
		m.appendMessage(RoleError, msg.err.Error(), time.Now())
		m.statusText = "Error"
		m.textarea.Focus()
		m.updateViewportContent()
		return m, nil

	case threadLoadedMsg:
		for _, apiMsg := range msg.resp.Messages {
			m.renderAPIMessage(apiMsg)
		}
		m.threadID = msg.resp.Thread.ID
		m.statusText = fmt.Sprintf("Thread: %s", m.threadID)
		m.updateViewportContent()
		return m, nil

	case threadLoadErrMsg:
		m.appendMessage(RoleError, msg.err.Error(), time.Now())
		m.updateViewportContent()
		return m, nil
	}

	// Pass viewport scroll events
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m chatModel) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	headerH := 1
	textareaH := 3
	statusH := 1
	vpH := m.height - headerH - textareaH - statusH
	if vpH < 1 {
		vpH = 1
	}

	if !m.ready {
		m.viewport = viewport.New(m.width, vpH)
		m.viewport.SetContent("")
		m.ready = true
	} else {
		m.viewport.Width = m.width
		m.viewport.Height = vpH
	}

	m.textarea.SetWidth(m.width)

	rendererWidth := m.width - 4
	if rendererWidth < 20 {
		rendererWidth = 20
	}
	m.mdRenderer = newMarkdownRenderer(rendererWidth)

	m.reRenderAllMessages()
	return m, nil
}

func (m chatModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, chatKeys.Quit):
		m.quitting = true
		return m, tea.Quit

	case key.Matches(msg, chatKeys.Cancel):
		if m.streaming {
			m.cancelStream()
			m.streaming = false
			m.statusText = "Cancelled"
			m.textarea.Focus()
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit

	case key.Matches(msg, chatKeys.Submit):
		if m.streaming {
			return m, nil
		}
		query := strings.TrimSpace(m.textarea.Value())
		if query == "" {
			return m, nil
		}
		m.appendMessage(RoleHuman, query, time.Now())
		m.textarea.Reset()
		m.textarea.Blur()
		m.streaming = true
		m.statusText = "Sending..."
		m.updateViewportContent()
		// Start spinner animation alongside SSE stream
		return m, tea.Batch(m.startSSEStream(query), m.spinner.Tick)

	default:
		if !m.streaming {
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			return m, cmd
		}
		return m, nil
	}
}

// -- Helpers --

func (m *chatModel) appendMessage(role, content string, ts time.Time) {
	rendered := m.renderContent(role, content)
	m.messages = append(m.messages, displayMessage{
		role:      role,
		content:   content,
		rendered:  rendered,
		timestamp: ts,
	})
}

func (m *chatModel) renderContent(role, content string) string {
	switch role {
	case RoleAssistant:
		return renderMarkdown(m.mdRenderer, content)
	case RoleHuman:
		return styleUserMessage(content)
	case RoleError:
		return styleError(content)
	default:
		return content
	}
}

func (m *chatModel) processDelta(d ContentDelta) {
	if !m.shouldShowMessage(d.Role) {
		return
	}

	// Skip human messages from SSE — we already display them when the user presses Enter.
	if d.Role == RoleHuman {
		return
	}

	if d.IsNew {
		ts := time.Now()
		rendered := m.renderContent(d.Role, d.Content)
		m.messages = append(m.messages, displayMessage{
			role:      d.Role,
			content:   d.Content,
			rendered:  rendered,
			timestamp: ts,
		})
		return
	}

	// Updated existing message: find last message with same role and update it.
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].role == d.Role {
			m.messages[i].content = d.Content
			m.messages[i].rendered = m.renderContent(d.Role, d.Content)
			return
		}
	}
}

func (m *chatModel) shouldShowMessage(role string) bool {
	switch role {
	case RoleHuman, RoleAssistant, RoleError, RoleCancel:
		return true
	case RoleMetadata:
		return true
	case RoleThinking, RoleTool, RoleAgentSelection, RoleSubAgentThread:
		return m.cfg.verbose
	default:
		return false
	}
}

func (m *chatModel) renderAPIMessage(msg Message) {
	if !m.shouldShowMessage(msg.Role) {
		return
	}
	ts := time.Now()
	if msg.StartedAt != nil {
		ts = *msg.StartedAt
	}
	m.appendMessage(msg.Role, msg.Content, ts)
}

func (m *chatModel) reRenderAllMessages() {
	for i := range m.messages {
		m.messages[i].rendered = m.renderContent(m.messages[i].role, m.messages[i].content)
	}
	m.updateViewportContent()
}

func (m *chatModel) updateViewportContent() {
	var sb strings.Builder
	for i, msg := range m.messages {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(msg.rendered)
	}
	m.viewport.SetContent(sb.String())
	m.viewport.GotoBottom()
}

// -- SSE stream management --

// startSSEStream opens the SSE connection and returns an sseStreamOpenedMsg.
// The stream and cancel func are NOT stored in the model here (cmd closures
// cannot mutate the canonical model). Instead, they're passed via the message
// and stored in Update's sseStreamOpenedMsg handler.
func (m chatModel) startSSEStream(query string) tea.Cmd {
	client := m.client
	cfg := m.cfg
	threadID := m.threadID
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())

		req := &InvokeRequest{
			Message:      query,
			Dataset:      cfg.dataset,
			ThreadID:     threadID,
			NetworkLevel: cfg.networkLevel,
		}

		resp, err := client.InvokeAgent0(ctx, req)
		if err != nil {
			cancel()
			return sseErrorMsg{err: err}
		}

		stream := NewSSEStream(resp.Body)
		return sseStreamOpenedMsg{stream: stream, cancel: cancel}
	}
}

// readNextSSEEvent returns a tea.Cmd that reads the next event from the active stream.
func (m chatModel) readNextSSEEvent() tea.Cmd {
	stream := m.activeStream
	if stream == nil {
		return nil
	}
	return func() tea.Msg {
		return readNextEvent(stream)
	}
}

func readNextEvent(stream *SSEStream) tea.Msg {
	event, err := stream.Next()
	if err != nil {
		if err == io.EOF {
			stream.Close()
			return sseDoneMsg{}
		}
		stream.Close()
		return sseErrorMsg{err: err}
	}

	if event.IsDone() {
		stream.Close()
		return sseDoneMsg{}
	}

	if event.EventType == "status" {
		var status struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(event.Data), &status); err == nil {
			return sseStatusMsg{status: status.Status}
		}
		return sseStatusMsg{status: event.Data}
	}

	var snapshot InvokeResponse
	if err := json.Unmarshal([]byte(event.Data), &snapshot); err != nil {
		return sseErrorMsg{err: fmt.Errorf("failed to parse SSE event: %w", err)}
	}
	return sseSnapshotMsg{resp: &snapshot}
}

func (m *chatModel) cancelStream() {
	if m.streamCancel != nil {
		m.streamCancel()
	}
	if m.activeStream != nil {
		m.activeStream.Close()
		m.activeStream = nil
	}
}

func (m chatModel) loadExistingThread() tea.Cmd {
	client := m.client
	threadID := m.cfg.threadID
	return func() tea.Msg {
		resp, err := client.GetAgent0Thread(context.Background(), threadID)
		if err != nil {
			return threadLoadErrMsg{err: err}
		}
		return threadLoadedMsg{resp: resp}
	}
}

func formatStatus(status string) string {
	switch status {
	case "preparing":
		return "Preparing sandbox..."
	case "running":
		return "Running..."
	default:
		return status
	}
}
