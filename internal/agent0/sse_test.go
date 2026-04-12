package agent0

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStream(data string) *SSEStream {
	return NewSSEStream(io.NopCloser(strings.NewReader(data)))
}

func TestSSEStreamParseStatusEvent(t *testing.T) {
	stream := newTestStream("event: status\ndata: {\"status\":\"preparing\"}\n\n")

	event, err := stream.Next()
	require.NoError(t, err)
	assert.Equal(t, "status", event.EventType)
	assert.Equal(t, `{"status":"preparing"}`, event.Data)
	assert.False(t, event.IsDone())
}

func TestSSEStreamParseDataEvent(t *testing.T) {
	payload := `{"thread":{"id":"t1"},"messages":[]}`
	stream := newTestStream("data: " + payload + "\n\n")

	event, err := stream.Next()
	require.NoError(t, err)
	assert.Equal(t, "", event.EventType)
	assert.Equal(t, payload, event.Data)
}

func TestSSEStreamParseDoneEvent(t *testing.T) {
	stream := newTestStream("data: [DONE]\n\n")

	event, err := stream.Next()
	require.NoError(t, err)
	assert.True(t, event.IsDone())
	assert.Equal(t, "[DONE]", event.Data)
}

func TestSSEStreamMultipleEvents(t *testing.T) {
	input := "event: status\ndata: {\"status\":\"preparing\"}\n\ndata: {\"thread\":{\"id\":\"t1\"},\"messages\":[]}\n\ndata: [DONE]\n\n"
	stream := newTestStream(input)

	// Event 1: status
	event, err := stream.Next()
	require.NoError(t, err)
	assert.Equal(t, "status", event.EventType)

	// Event 2: data
	event, err = stream.Next()
	require.NoError(t, err)
	assert.Equal(t, "", event.EventType)
	assert.Contains(t, event.Data, `"id":"t1"`)

	// Event 3: done
	event, err = stream.Next()
	require.NoError(t, err)
	assert.True(t, event.IsDone())

	// EOF
	_, err = stream.Next()
	assert.ErrorIs(t, err, io.EOF)
}

func TestSSEStreamMultiLineData(t *testing.T) {
	input := "data: line1\ndata: line2\ndata: line3\n\n"
	stream := newTestStream(input)

	event, err := stream.Next()
	require.NoError(t, err)
	assert.Equal(t, "line1\nline2\nline3", event.Data)
}

func TestSSEStreamIgnoresComments(t *testing.T) {
	input := ": this is a comment\ndata: actual-data\n\n"
	stream := newTestStream(input)

	event, err := stream.Next()
	require.NoError(t, err)
	assert.Equal(t, "actual-data", event.Data)
}

func TestSSEStreamEmptyInput(t *testing.T) {
	stream := newTestStream("")

	_, err := stream.Next()
	assert.ErrorIs(t, err, io.EOF)
}

func TestSSEStreamLeadingBlankLines(t *testing.T) {
	input := "\n\n\ndata: payload\n\n"
	stream := newTestStream(input)

	event, err := stream.Next()
	require.NoError(t, err)
	assert.Equal(t, "payload", event.Data)
}

func TestSSEStreamDataWithoutTrailingBlankLine(t *testing.T) {
	// Stream that ends abruptly without a final blank line
	input := "data: incomplete"
	stream := newTestStream(input)

	event, err := stream.Next()
	require.NoError(t, err)
	assert.Equal(t, "incomplete", event.Data)
}

func TestSSEStreamDataWithoutSpace(t *testing.T) {
	// "data:" without the space after colon
	input := "data:no-space\n\n"
	stream := newTestStream(input)

	event, err := stream.Next()
	require.NoError(t, err)
	assert.Equal(t, "no-space", event.Data)
}

func TestSSEStreamClosePreventsReads(t *testing.T) {
	stream := newTestStream("data: test\n\n")
	stream.Close()

	_, err := stream.Next()
	assert.ErrorIs(t, err, io.EOF)
}

func TestSSEStreamCloseIsIdempotent(t *testing.T) {
	stream := newTestStream("data: test\n\n")
	stream.Close()
	stream.Close() // Should not panic
}
