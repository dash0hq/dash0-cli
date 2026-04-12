package agent0

import (
	"bufio"
	"io"
	"strings"
)

// SSEEvent represents a single Server-Sent Event.
type SSEEvent struct {
	EventType string // "status" or empty (default message event)
	Data      string // JSON payload or "[DONE]"
}

// IsDone returns true if this is the terminal [DONE] event.
func (e SSEEvent) IsDone() bool {
	return e.Data == "[DONE]"
}

// SSEStream reads Server-Sent Events from an io.ReadCloser.
type SSEStream struct {
	scanner *bufio.Scanner
	body    io.ReadCloser
	closed  bool
}

// NewSSEStream creates a new SSE stream reader from the given body.
func NewSSEStream(body io.ReadCloser) *SSEStream {
	return &SSEStream{
		scanner: bufio.NewScanner(body),
		body:    body,
	}
}

// Next reads and returns the next SSE event from the stream.
// Returns io.EOF when the stream ends normally.
func (s *SSEStream) Next() (*SSEEvent, error) {
	if s.closed {
		return nil, io.EOF
	}

	var event SSEEvent
	var dataLines []string

	for s.scanner.Scan() {
		line := s.scanner.Text()

		switch {
		case line == "":
			// Empty line marks the end of an event.
			if len(dataLines) > 0 {
				event.Data = strings.Join(dataLines, "\n")
				return &event, nil
			}
			// Empty line with no accumulated data: skip (e.g., leading blank lines).

		case strings.HasPrefix(line, "data: "):
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))

		case strings.HasPrefix(line, "data:"):
			// "data:" without space — value is the rest of the line.
			dataLines = append(dataLines, strings.TrimPrefix(line, "data:"))

		case strings.HasPrefix(line, "event: "):
			event.EventType = strings.TrimPrefix(line, "event: ")

		case strings.HasPrefix(line, "event:"):
			event.EventType = strings.TrimPrefix(line, "event:")

		default:
			// SSE comments (lines starting with ":") and unrecognized fields
			// ("id:", "retry:") are silently ignored.
		}
	}

	// Scanner finished: check for error or EOF.
	if err := s.scanner.Err(); err != nil {
		return nil, err
	}

	// If we have accumulated data without a trailing blank line, emit it.
	if len(dataLines) > 0 {
		event.Data = strings.Join(dataLines, "\n")
		return &event, nil
	}

	return nil, io.EOF
}

// Close closes the underlying stream.
func (s *SSEStream) Close() {
	if !s.closed {
		s.closed = true
		s.body.Close()
	}
}
