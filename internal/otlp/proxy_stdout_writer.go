package otlp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	"go.opentelemetry.io/collector/pdata/plog"
)

// stdoutEventBufferDepth controls how many events the emitter can queue
// ahead of the writer goroutine. At the 1-second stats cadence + occasional
// forwarded/error events, 128 is comfortably more than enough; an
// overflowing channel signals a stuck writer (or a stuck downstream
// consumer of stdout), in which case dropping is preferable to stalling
// the proxy.
const stdoutEventBufferDepth = 128

// StdoutWriter drains an event channel and writes one OTLP/JSON line per
// event to its underlying writer. The single-goroutine ownership of
// os.Stdout guarantees one valid JSON record per line: concurrent emitters
// produce no interleaving because everyone funnels through one channel and
// one writer.
//
// Each event triggers a marshal → write → newline → flush cycle, so a
// downstream consumer reading the stream line-by-line sees complete
// records without head-of-line buffering delays.
type StdoutWriter struct {
	out io.Writer

	// marshalLogs is overridable so tests can simulate marshal failures.
	// Production callers use plog.JSONMarshaler{}.MarshalLogs (set by
	// NewStdoutWriter); tests substitute a function that returns an error
	// to exercise the onMarshalErr path.
	marshalLogs func(plog.Logs) ([]byte, error)

	// onMarshalErr is invoked when marshalLogs fails. The default
	// implementation writes a brief notice to os.Stderr; tests override
	// for assertion.
	onMarshalErr func(error)
}

// NewStdoutWriter returns a writer that drains events to out. The caller
// also receives the event channel that emitters should push to.
func NewStdoutWriter(out io.Writer) (*StdoutWriter, chan plog.Logs) {
	var m plog.JSONMarshaler
	return &StdoutWriter{
		out:         out,
		marshalLogs: m.MarshalLogs,
		onMarshalErr: func(err error) {
			// Best-effort notice — the proxy keeps running even when one
			// event can't be marshaled.
			fmt.Fprintf(stdoutWriterErrSink, "dash0 otlp proxy: failed to marshal event: %v\n", err)
		},
	}, make(chan plog.Logs, stdoutEventBufferDepth)
}

// stdoutWriterErrSink is the destination for the default onMarshalErr.
// Indirected as a package-level variable so tests can substitute a buffer
// to capture marshal-error notices without touching os.Stderr.
var stdoutWriterErrSink io.Writer = os.Stderr

// Run drains ch into the writer's out until ctx is done or the channel is
// closed. Returns nil on graceful exit; returns an error only when writing
// to out fails irrecoverably (which should be rare on os.Stdout — typically
// only when the receiving pipe has closed).
func (w *StdoutWriter) Run(ctx context.Context, ch <-chan plog.Logs) error {
	bw := bufio.NewWriter(w.out)
	for {
		select {
		case <-ctx.Done():
			_ = bw.Flush()
			return nil
		case ld, ok := <-ch:
			if !ok {
				_ = bw.Flush()
				return nil
			}
			if err := w.writeOne(bw, ld); err != nil {
				_ = bw.Flush()
				return err
			}
		}
	}
}

func (w *StdoutWriter) writeOne(bw *bufio.Writer, ld plog.Logs) error {
	data, err := w.marshalLogs(ld)
	if err != nil {
		w.onMarshalErr(err)
		return nil
	}
	if _, err := bw.Write(data); err != nil {
		return fmt.Errorf("write event to stdout: %w", err)
	}
	if err := bw.WriteByte('\n'); err != nil {
		return fmt.Errorf("write newline to stdout: %w", err)
	}
	// Flush per event so a line-by-line consumer sees records immediately
	// rather than buffered behind a 4KB write batch.
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("flush stdout: %w", err)
	}
	return nil
}
