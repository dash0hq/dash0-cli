package agent0

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// debugLogger writes raw API events to a log file for debugging.
// nil-safe: all methods are no-ops when the logger is nil.
type debugLogger struct {
	file *os.File
}

func newDebugLogger(path string) (*debugLogger, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open debug log: %w", err)
	}
	fmt.Fprintf(f, "\n=== agent0 chat session started at %s ===\n", time.Now().Format(time.RFC3339))
	return &debugLogger{file: f}, nil
}

func (l *debugLogger) Close() {
	if l == nil || l.file == nil {
		return
	}
	l.file.Close()
}

// LogSSEEvent logs a raw SSE event (before any parsing or rendering).
func (l *debugLogger) LogSSEEvent(eventType, data string) {
	if l == nil {
		return
	}
	fmt.Fprintf(l.file, "[%s] event=%q data=%s\n", time.Now().Format("15:04:05.000"), eventType, data)
}

// LogSnapshot logs a parsed SSE snapshot with pretty-printed JSON.
func (l *debugLogger) LogSnapshot(resp *InvokeResponse) {
	if l == nil {
		return
	}
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		fmt.Fprintf(l.file, "[%s] snapshot (marshal error: %v)\n", time.Now().Format("15:04:05.000"), err)
		return
	}
	fmt.Fprintf(l.file, "[%s] snapshot:\n%s\n", time.Now().Format("15:04:05.000"), data)
}

// LogMessage logs a single message with its raw content.
func (l *debugLogger) LogMessage(role, id, content string) {
	if l == nil {
		return
	}
	fmt.Fprintf(l.file, "[%s] message role=%s id=%s content:\n%s\n---\n",
		time.Now().Format("15:04:05.000"), role, id, content)
}

// Log writes a freeform debug line.
func (l *debugLogger) Log(format string, args ...any) {
	if l == nil {
		return
	}
	fmt.Fprintf(l.file, "[%s] %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
}
