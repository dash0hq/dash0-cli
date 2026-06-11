package otlp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

func TestEmitter_NilChannelIsNoOp(t *testing.T) {
	// Constructing an Emitter without a channel must make every Emit method
	// a silent no-op so the caller can use one shape regardless of mode.
	e := NewEmitter("test-instance", nil)
	e.EmitStarted("h", "g", "d", "p")
	e.EmitForwarded(SignalLogs, 1, 100)
	e.EmitStats(SnapshotWithRate{})
	e.EmitError(ErrorKindUpstream5xx, "boom", 503)
	e.EmitShutdown("SIGTERM", [signalCount]int64{1, 2, 3})
	// No panic, no error — that's the contract.
}

func TestEmitter_StartedEvent(t *testing.T) {
	ch := make(chan plog.Logs, 1)
	e := NewEmitter("inst-1", ch)
	e.EmitStarted("http://127.0.0.1:4318", "127.0.0.1:4317", "default", "dev")

	ld := mustReceive(t, ch)
	checkResourceCommon(t, ld, "inst-1")
	lr := mustOneLogRecord(t, ld)
	if got := lr.EventName(); got != eventStarted {
		t.Errorf("EventName = %q; want %q", got, eventStarted)
	}
	mustHaveStr(t, lr.Attributes(), "endpoint.http", "http://127.0.0.1:4318")
	mustHaveStr(t, lr.Attributes(), "endpoint.grpc", "127.0.0.1:4317")
	mustHaveStr(t, lr.Attributes(), "dataset", "default")
	mustHaveStr(t, lr.Attributes(), "profile.name", "dev")
}

func TestEmitter_StartedEvent_OmitsEmptyEndpoints(t *testing.T) {
	// When one listener failed to bind, the corresponding endpoint attribute
	// is omitted rather than carrying an empty string.
	ch := make(chan plog.Logs, 1)
	e := NewEmitter("inst-1", ch)
	e.EmitStarted("http://127.0.0.1:4318", "", "default", "dev")

	ld := mustReceive(t, ch)
	lr := mustOneLogRecord(t, ld)
	if _, ok := lr.Attributes().Get("endpoint.grpc"); ok {
		t.Error("endpoint.grpc should be omitted when empty")
	}
}

func TestEmitter_ForwardedEvent(t *testing.T) {
	ch := make(chan plog.Logs, 1)
	e := NewEmitter("inst", ch)
	e.EmitForwarded(SignalSpans, 12, 4096)

	ld := mustReceive(t, ch)
	lr := mustOneLogRecord(t, ld)
	if got := lr.EventName(); got != eventForwarded {
		t.Errorf("EventName = %q; want %q", got, eventForwarded)
	}
	mustHaveStr(t, lr.Attributes(), "signal", "spans")
	mustHaveInt(t, lr.Attributes(), "count", 12)
	mustHaveInt(t, lr.Attributes(), "bytes", 4096)
}

func TestEmitter_StatsEvent(t *testing.T) {
	ch := make(chan plog.Logs, 1)
	e := NewEmitter("inst", ch)

	snap := SnapshotWithRate{
		Snapshot: Snapshot{
			Forwarded: [signalCount]int64{100, 50, 0},
			Failed:    [signalCount]int64{2, 0, 0},
		},
		Rate: [signalCount]float64{12.5, 5.0, 0},
	}
	e.EmitStats(snap)

	ld := mustReceive(t, ch)
	lr := mustOneLogRecord(t, ld)
	if got := lr.EventName(); got != eventStats {
		t.Errorf("EventName = %q; want %q", got, eventStats)
	}
	mustHaveDouble(t, lr.Attributes(), "logs.rate", 12.5)
	mustHaveInt(t, lr.Attributes(), "logs.total", 100)
	mustHaveInt(t, lr.Attributes(), "logs.failed", 2)
	mustHaveDouble(t, lr.Attributes(), "spans.rate", 5.0)
	mustHaveInt(t, lr.Attributes(), "spans.total", 50)
	mustHaveDouble(t, lr.Attributes(), "metrics.rate", 0)
	mustHaveInt(t, lr.Attributes(), "metrics.total", 0)
}

func TestEmitter_ErrorEvent(t *testing.T) {
	ch := make(chan plog.Logs, 1)
	e := NewEmitter("inst", ch)
	e.EmitError(ErrorKindUpstream4xxAuth, "Dash0 returned 401", 401)

	ld := mustReceive(t, ch)
	lr := mustOneLogRecord(t, ld)
	if got := lr.EventName(); got != eventError {
		t.Errorf("EventName = %q; want %q", got, eventError)
	}
	mustHaveStr(t, lr.Attributes(), "error.kind", string(ErrorKindUpstream4xxAuth))
	mustHaveStr(t, lr.Attributes(), "reason", "Dash0 returned 401")
	mustHaveInt(t, lr.Attributes(), "code", 401)
}

func TestEmitter_ErrorEvent_OmitsBlankFields(t *testing.T) {
	ch := make(chan plog.Logs, 1)
	e := NewEmitter("inst", ch)
	e.EmitError(ErrorKindUpstreamUnreachable, "", 0)

	ld := mustReceive(t, ch)
	lr := mustOneLogRecord(t, ld)
	if _, ok := lr.Attributes().Get("reason"); ok {
		t.Error("reason should be omitted when empty")
	}
	if _, ok := lr.Attributes().Get("code"); ok {
		t.Error("code should be omitted when zero")
	}
}

func TestEmitter_ShutdownEvent(t *testing.T) {
	ch := make(chan plog.Logs, 1)
	e := NewEmitter("inst", ch)
	totals := [signalCount]int64{1000, 500, 250}
	e.EmitShutdown("SIGTERM", totals)

	ld := mustReceive(t, ch)
	lr := mustOneLogRecord(t, ld)
	if got := lr.EventName(); got != eventShutdown {
		t.Errorf("EventName = %q; want %q", got, eventShutdown)
	}
	mustHaveStr(t, lr.Attributes(), "reason", "SIGTERM")
	mustHaveInt(t, lr.Attributes(), "final_total.logs", 1000)
	mustHaveInt(t, lr.Attributes(), "final_total.spans", 500)
	mustHaveInt(t, lr.Attributes(), "final_total.metrics", 250)
}

func TestEmitter_TimestampsSet(t *testing.T) {
	ch := make(chan plog.Logs, 1)
	e := NewEmitter("inst", ch)
	fixed := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	e.now = func() time.Time { return fixed }
	e.EmitForwarded(SignalLogs, 1, 1)

	ld := mustReceive(t, ch)
	lr := mustOneLogRecord(t, ld)
	if !lr.Timestamp().AsTime().Equal(fixed) {
		t.Errorf("Timestamp = %s; want %s", lr.Timestamp().AsTime(), fixed)
	}
	if !lr.ObservedTimestamp().AsTime().Equal(fixed) {
		t.Errorf("ObservedTimestamp = %s; want %s", lr.ObservedTimestamp().AsTime(), fixed)
	}
}

func TestEmitter_ScopeName(t *testing.T) {
	ch := make(chan plog.Logs, 1)
	e := NewEmitter("inst", ch)
	e.EmitForwarded(SignalLogs, 1, 1)

	ld := mustReceive(t, ch)
	sl := ld.ResourceLogs().At(0).ScopeLogs().At(0)
	if got := sl.Scope().Name(); got != scopeName {
		t.Errorf("Scope().Name() = %q; want %q", got, scopeName)
	}
}

func TestEmitter_NonBlockingDrops(t *testing.T) {
	// Channel buffer of 1: fill it once, then a second emit must not block.
	ch := make(chan plog.Logs, 1)
	e := NewEmitter("inst", ch)
	e.EmitForwarded(SignalLogs, 1, 1)

	done := make(chan struct{})
	go func() {
		e.EmitForwarded(SignalLogs, 2, 2)
		close(done)
	}()
	select {
	case <-done:
		// Returned promptly even though the channel was full — that's the
		// non-blocking-send contract.
	case <-time.After(200 * time.Millisecond):
		t.Fatal("EmitForwarded blocked on a full channel; should drop instead")
	}
}

func TestStdoutWriter_WritesOneJSONLinePerEvent(t *testing.T) {
	buf := &safeBuffer{}
	w, ch := NewStdoutWriter(buf)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = w.Run(ctx, ch)
		close(done)
	}()

	e := NewEmitter("inst", ch)
	e.EmitForwarded(SignalLogs, 1, 100)
	e.EmitForwarded(SignalSpans, 2, 200)
	e.EmitShutdown("SIGTERM", [signalCount]int64{1, 2, 0})

	// Give the writer time to drain three events.
	deadline := time.After(500 * time.Millisecond)
	for {
		if strings.Count(buf.String(), "\n") >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("writer did not drain 3 events in time; got: %q", buf.String())
		case <-time.After(5 * time.Millisecond):
		}
	}

	cancel()
	<-done

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 NDJSON lines; got %d:\n%s", len(lines), buf.String())
	}
	for i, line := range lines {
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Errorf("line %d is not valid JSON: %v\n%s", i, err, line)
		}
	}
}

func TestStdoutWriter_ExitsOnContextCancel(t *testing.T) {
	var buf bytes.Buffer
	w, ch := NewStdoutWriter(&buf)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = w.Run(ctx, ch)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Run did not exit within 200ms of context cancel")
	}
}

func TestStdoutWriter_ExitsOnChannelClose(t *testing.T) {
	var buf bytes.Buffer
	w, ch := NewStdoutWriter(&buf)
	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		_ = w.Run(ctx, ch)
		close(done)
	}()
	close(ch)
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Run did not exit within 200ms of channel close")
	}
}

func TestStdoutWriter_MarshalErrorSurfacesToHandler(t *testing.T) {
	// Substitute the marshal-error sink with a thread-safe buffer.
	origSink := stdoutWriterErrSink
	sink := &safeBuffer{}
	stdoutWriterErrSink = sink
	defer func() { stdoutWriterErrSink = origSink }()

	var out bytes.Buffer
	w, ch := NewStdoutWriter(&out)

	// Force a marshal failure by substituting a marshalLogs that always errors.
	w.marshalLogs = func(plog.Logs) ([]byte, error) {
		return nil, errors.New("synthetic marshal failure")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = w.Run(ctx, ch) }()

	ch <- plog.NewLogs()
	deadline := time.After(500 * time.Millisecond)
	for sink.Len() == 0 {
		select {
		case <-deadline:
			t.Fatal("marshal-error handler was never invoked")
		case <-time.After(5 * time.Millisecond):
		}
	}
	if !strings.Contains(sink.String(), "failed to marshal event") {
		t.Errorf("expected marshal-error notice; got %q", sink.String())
	}
}

func TestStdoutWriter_FlushPerEventEnablesLineByLineReader(t *testing.T) {
	// A real reader on the other end of the writer should see each event
	// without buffer delay. Use io.Pipe to simulate a downstream consumer.
	r, p := io.Pipe()
	t.Cleanup(func() { _ = p.Close(); _ = r.Close() })

	w, ch := NewStdoutWriter(p)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writerDone := make(chan struct{})
	go func() {
		_ = w.Run(ctx, ch)
		close(writerDone)
	}()
	_ = writerDone

	e := NewEmitter("inst", ch)
	e.EmitForwarded(SignalLogs, 1, 1)

	bufrd := bufio.NewReader(r)
	type result struct {
		line string
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		line, err := bufrd.ReadString('\n')
		resultCh <- result{line, err}
	}()

	select {
	case res := <-resultCh:
		if res.err != nil {
			t.Fatalf("read line: %v", res.err)
		}
		if res.line == "" || !strings.HasSuffix(res.line, "\n") {
			t.Errorf("expected newline-terminated line; got %q", res.line)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("writer did not flush within 500ms; line-by-line consumer would stall")
	}
}

func TestStdoutWriter_SerializesConcurrentEmitters(t *testing.T) {
	// Multiple goroutines emitting at once must not interleave on stdout —
	// the single-writer goroutine is the contract.
	var buf bytes.Buffer
	w, ch := NewStdoutWriter(&buf)
	ctx, cancel := context.WithCancel(context.Background())
	writerDone := make(chan struct{})
	go func() {
		_ = w.Run(ctx, ch)
		close(writerDone)
	}()

	e := NewEmitter("inst", ch)
	const goroutines = 8
	const eventsPerGoroutine = 16
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < eventsPerGoroutine; i++ {
				e.EmitForwarded(SignalLogs, 1, 1)
			}
		}()
	}
	wg.Wait()

	// Allow the writer time to drain whatever wasn't dropped by the
	// non-blocking emit. We can't assert exact counts because the channel
	// buffer (and the writer's pace) makes drops possible; we can assert
	// every emitted line is a complete JSON record.
	time.Sleep(50 * time.Millisecond)
	cancel()
	// Wait for the writer goroutine to fully exit before reading the
	// buffer — bytes.Buffer is not safe for concurrent reader+writer.
	<-writerDone

	scanner := bufio.NewScanner(&buf)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var v map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &v); err != nil {
			t.Errorf("interleaved line not valid JSON: %v\n%s", err, scanner.Bytes())
		}
	}
}

// safeBuffer is a thread-safe wrapper around bytes.Buffer for tests where
// one goroutine writes and another reads. bytes.Buffer itself races under
// that pattern.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *safeBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

// mustReceive pulls one event off ch with a short timeout to avoid blocking
// forever on a test bug.
func mustReceive(t *testing.T, ch <-chan plog.Logs) plog.Logs {
	t.Helper()
	select {
	case ld := <-ch:
		return ld
	case <-time.After(200 * time.Millisecond):
		t.Fatal("no event received within timeout")
		return plog.NewLogs()
	}
}

// mustOneLogRecord asserts ld has exactly one ResourceLogs → one ScopeLogs
// → one LogRecord and returns the record.
func mustOneLogRecord(t *testing.T, ld plog.Logs) plog.LogRecord {
	t.Helper()
	if ld.ResourceLogs().Len() != 1 {
		t.Fatalf("ResourceLogs count = %d; want 1", ld.ResourceLogs().Len())
	}
	rl := ld.ResourceLogs().At(0)
	if rl.ScopeLogs().Len() != 1 {
		t.Fatalf("ScopeLogs count = %d; want 1", rl.ScopeLogs().Len())
	}
	sl := rl.ScopeLogs().At(0)
	if sl.LogRecords().Len() != 1 {
		t.Fatalf("LogRecords count = %d; want 1", sl.LogRecords().Len())
	}
	return sl.LogRecords().At(0)
}

func checkResourceCommon(t *testing.T, ld plog.Logs, wantInstanceID string) {
	t.Helper()
	attrs := ld.ResourceLogs().At(0).Resource().Attributes()
	mustHaveStr(t, attrs, "service.name", "dash0-cli")
	mustHaveStr(t, attrs, "service.instance.id", wantInstanceID)
}

func mustHaveStr(t *testing.T, m pcommon.Map, key, want string) {
	t.Helper()
	v, ok := m.Get(key)
	if !ok {
		t.Errorf("missing attribute %q", key)
		return
	}
	if got := v.Str(); got != want {
		t.Errorf("%s = %q; want %q", key, got, want)
	}
}

func mustHaveInt(t *testing.T, m pcommon.Map, key string, want int64) {
	t.Helper()
	v, ok := m.Get(key)
	if !ok {
		t.Errorf("missing attribute %q", key)
		return
	}
	if got := v.Int(); got != want {
		t.Errorf("%s = %d; want %d", key, got, want)
	}
}

func mustHaveDouble(t *testing.T, m pcommon.Map, key string, want float64) {
	t.Helper()
	v, ok := m.Get(key)
	if !ok {
		t.Errorf("missing attribute %q", key)
		return
	}
	if got := v.Double(); got != want {
		t.Errorf("%s = %f; want %f", key, got, want)
	}
}
