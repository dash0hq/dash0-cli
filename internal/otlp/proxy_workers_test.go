package otlp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// fakeForwarder is a test double for Forwarder. Each Send method records the
// call and returns the next error configured for that signal (cycling if
// exhausted).
type fakeForwarder struct {
	mu sync.Mutex

	logsErrs    []error
	tracesErrs  []error
	metricsErrs []error

	logsCalls    atomic.Int64
	tracesCalls  atomic.Int64
	metricsCalls atomic.Int64

	// lastDataset captures the *string pointer from the latest call so
	// dataset-passthrough behavior is verifiable.
	lastDataset *string

	// onSend is an optional hook called BEFORE each Send returns; tests use
	// it to inject deterministic panics or timing signals.
	onSend func(sig Signal)
}

func (f *fakeForwarder) SendLogs(_ context.Context, _ plog.Logs, dataset *string) error {
	// Count the attempt before any panic so panic recovery is observable to
	// the test (otherwise a panicking call would never increment).
	f.logsCalls.Add(1)
	f.mu.Lock()
	f.lastDataset = dataset
	hook := f.onSend
	var err error
	if len(f.logsErrs) > 0 {
		err = f.logsErrs[0]
		f.logsErrs = f.logsErrs[1:]
	}
	f.mu.Unlock()
	if hook != nil {
		hook(SignalLogs)
	}
	return err
}

func (f *fakeForwarder) SendTraces(_ context.Context, _ ptrace.Traces, dataset *string) error {
	f.tracesCalls.Add(1)
	f.mu.Lock()
	f.lastDataset = dataset
	hook := f.onSend
	var err error
	if len(f.tracesErrs) > 0 {
		err = f.tracesErrs[0]
		f.tracesErrs = f.tracesErrs[1:]
	}
	f.mu.Unlock()
	if hook != nil {
		hook(SignalSpans)
	}
	return err
}

func (f *fakeForwarder) SendMetrics(_ context.Context, _ pmetric.Metrics, dataset *string) error {
	f.metricsCalls.Add(1)
	f.mu.Lock()
	f.lastDataset = dataset
	hook := f.onSend
	var err error
	if len(f.metricsErrs) > 0 {
		err = f.metricsErrs[0]
		f.metricsErrs = f.metricsErrs[1:]
	}
	f.mu.Unlock()
	if hook != nil {
		hook(SignalMetrics)
	}
	return err
}

// errOnSend lets a single call panic. The Forwarder field captures the
// panic-trigger for the next Send call only.
func panicNext(sig Signal) func(Signal) {
	var fired atomic.Bool
	return func(s Signal) {
		if s == sig && fired.CompareAndSwap(false, true) {
			panic("synthetic worker panic for test")
		}
	}
}

// drainEvents collects all events the emitter has pushed onto ch into a
// snapshot slice for assertion.
func drainEvents(ch <-chan plog.Logs) []plog.Logs {
	out := make([]plog.Logs, 0)
	for {
		select {
		case ld := <-ch:
			out = append(out, ld)
		default:
			return out
		}
	}
}

// firstEventName returns the eventName attribute of the only LogRecord in
// ld, or "" if absent. Aborts the test if ld has 0 or >1 records.
func firstEventName(t *testing.T, ld plog.Logs) string {
	t.Helper()
	rl := ld.ResourceLogs().At(0)
	sl := rl.ScopeLogs().At(0)
	if sl.LogRecords().Len() != 1 {
		t.Fatalf("expected exactly 1 log record; got %d", sl.LogRecords().Len())
	}
	return sl.LogRecords().At(0).EventName()
}

// firstEventKindAttr returns the value of the `error.kind` attribute of the
// only LogRecord in ld. Aborts the test if not present.
func firstEventKindAttr(t *testing.T, ld plog.Logs) string {
	t.Helper()
	rl := ld.ResourceLogs().At(0)
	sl := rl.ScopeLogs().At(0)
	lr := sl.LogRecords().At(0)
	v, ok := lr.Attributes().Get("error.kind")
	if !ok {
		t.Fatalf("error.kind not present on event")
	}
	return v.AsString()
}

func TestClassifyError_4xxAuth(t *testing.T) {
	cases := []int{401, 403}
	for _, code := range cases {
		err := &dash0api.APIError{StatusCode: code}
		kind, gotCode := classifyError(err)
		if kind != ErrorKindUpstream4xxAuth {
			t.Errorf("status %d: kind = %s; want upstream_4xx_auth", code, kind)
		}
		if gotCode != code {
			t.Errorf("status %d: code = %d; want %d", code, gotCode, code)
		}
	}
}

func TestClassifyError_4xxOther(t *testing.T) {
	cases := []int{400, 404, 409, 422}
	for _, code := range cases {
		err := &dash0api.APIError{StatusCode: code}
		kind, gotCode := classifyError(err)
		if kind != ErrorKindUpstream4xxOther {
			t.Errorf("status %d: kind = %s; want upstream_4xx_other", code, kind)
		}
		if gotCode != code {
			t.Errorf("status %d: code = %d; want %d", code, gotCode, code)
		}
	}
}

func TestClassifyError_5xx(t *testing.T) {
	cases := []int{500, 502, 503, 504}
	for _, code := range cases {
		err := &dash0api.APIError{StatusCode: code}
		kind, gotCode := classifyError(err)
		if kind != ErrorKindUpstream5xx {
			t.Errorf("status %d: kind = %s; want upstream_5xx", code, kind)
		}
		if gotCode != code {
			t.Errorf("status %d: code = %d; want %d", code, gotCode, code)
		}
	}
}

func TestClassifyError_Unreachable(t *testing.T) {
	// Plain non-APIError errors are network/transport-level failures.
	kind, code := classifyError(errors.New("dial tcp 127.0.0.1:443: connect: connection refused"))
	if kind != ErrorKindUpstreamUnreachable {
		t.Errorf("plain error kind = %s; want upstream_unreachable", kind)
	}
	if code != 0 {
		t.Errorf("plain error code = %d; want 0", code)
	}
}

func TestClassifyError_WrappedAPIError(t *testing.T) {
	wrapped := fmt.Errorf("send failed: %w", &dash0api.APIError{StatusCode: 503})
	kind, code := classifyError(wrapped)
	if kind != ErrorKindUpstream5xx {
		t.Errorf("wrapped 503 kind = %s; want upstream_5xx", kind)
	}
	if code != 503 {
		t.Errorf("wrapped 503 code = %d; want 503", code)
	}
}

func TestWorkerPool_HappyPath_NoFailedNoEvent(t *testing.T) {
	forwarder := &fakeForwarder{}
	stats := &Stats{}
	eventCh := make(chan plog.Logs, 4)
	emitter := NewEmitter("inst", eventCh)
	consumer := NewProxyConsumer(stats, NewEmitter("inst", nil), nil)
	pool := NewWorkerPool(forwarder, nil, stats, emitter, consumer, nil)

	pool.sendLogs(context.Background(), newLogsBatch(3))

	if got := forwarder.logsCalls.Load(); got != 1 {
		t.Errorf("SendLogs calls = %d; want 1", got)
	}
	if got := stats.Failed(SignalLogs); got != 0 {
		t.Errorf("Failed(logs) = %d; want 0 on success", got)
	}
	if evts := drainEvents(eventCh); len(evts) != 0 {
		t.Errorf("success should emit no events; got %d", len(evts))
	}
}

func TestWorkerPool_Upstream5xx_BumpsFailedAndEmitsEvent(t *testing.T) {
	forwarder := &fakeForwarder{
		logsErrs: []error{&dash0api.APIError{StatusCode: 503, Status: "503 Service Unavailable"}},
	}
	stats := &Stats{}
	eventCh := make(chan plog.Logs, 4)
	emitter := NewEmitter("inst", eventCh)
	consumer := NewProxyConsumer(stats, NewEmitter("inst", nil), nil)
	pool := NewWorkerPool(forwarder, nil, stats, emitter, consumer, nil)

	pool.sendLogs(context.Background(), newLogsBatch(2))

	if got := stats.Failed(SignalLogs); got != 2 {
		t.Errorf("Failed(logs) = %d; want 2", got)
	}
	evts := drainEvents(eventCh)
	if len(evts) != 1 {
		t.Fatalf("expected 1 error event; got %d", len(evts))
	}
	if name := firstEventName(t, evts[0]); name != eventError {
		t.Errorf("event name = %q; want %q", name, eventError)
	}
	if kind := firstEventKindAttr(t, evts[0]); kind != string(ErrorKindUpstream5xx) {
		t.Errorf("error.kind = %q; want %q", kind, ErrorKindUpstream5xx)
	}

	// Subsequent forwards still work (the worker did not bail).
	pool.sendLogs(context.Background(), newLogsBatch(1))
	if got := forwarder.logsCalls.Load(); got != 2 {
		t.Errorf("SendLogs calls after second send = %d; want 2", got)
	}
}

func TestWorkerPool_Upstream401_EmitsLifecycleWarning(t *testing.T) {
	forwarder := &fakeForwarder{
		logsErrs: []error{&dash0api.APIError{StatusCode: 401, Status: "401 Unauthorized"}},
	}
	stats := &Stats{}
	eventCh := make(chan plog.Logs, 4)
	emitter := NewEmitter("inst", eventCh)
	consumer := NewProxyConsumer(stats, NewEmitter("inst", nil), nil)
	lifecycleCh := make(chan LifecycleEvent, 2)
	pool := NewWorkerPool(forwarder, nil, stats, emitter, consumer, lifecycleCh)

	pool.sendLogs(context.Background(), newLogsBatch(1))

	if kind := firstEventKindAttr(t, <-eventCh); kind != string(ErrorKindUpstream4xxAuth) {
		t.Errorf("error.kind = %q; want %q", kind, ErrorKindUpstream4xxAuth)
	}
	select {
	case ev := <-lifecycleCh:
		if ev.Kind != LifecycleError {
			t.Errorf("lifecycle Kind = %d; want LifecycleError", ev.Kind)
		}
		if ev.Message == "" {
			t.Error("lifecycle Message is empty")
		}
	case <-time.After(50 * time.Millisecond):
		t.Error("expected lifecycle event within 50ms; got nothing")
	}
}

func TestWorkerPool_Upstream401_ThrottleSuppressesRepeat(t *testing.T) {
	forwarder := &fakeForwarder{
		logsErrs: []error{
			&dash0api.APIError{StatusCode: 401},
			&dash0api.APIError{StatusCode: 401},
			&dash0api.APIError{StatusCode: 401},
		},
	}
	stats := &Stats{}
	eventCh := make(chan plog.Logs, 8)
	emitter := NewEmitter("inst", eventCh)
	consumer := NewProxyConsumer(stats, NewEmitter("inst", nil), nil)
	lifecycleCh := make(chan LifecycleEvent, 4)
	pool := NewWorkerPool(forwarder, nil, stats, emitter, consumer, lifecycleCh)

	// Use a fixed clock so the throttle window is deterministic.
	fixed := time.Now()
	pool.now = func() time.Time { return fixed }

	// Three rapid 401s should produce exactly one lifecycle line.
	pool.sendLogs(context.Background(), newLogsBatch(1))
	pool.sendLogs(context.Background(), newLogsBatch(1))
	pool.sendLogs(context.Background(), newLogsBatch(1))

	// Drain lifecycle ch: exactly one event.
	count := 0
loop:
	for {
		select {
		case <-lifecycleCh:
			count++
		case <-time.After(20 * time.Millisecond):
			break loop
		}
	}
	if count != 1 {
		t.Errorf("expected 1 lifecycle warning within throttle window; got %d", count)
	}

	// Advance the clock past the throttle window — next 401 surfaces again.
	pool.now = func() time.Time { return fixed.Add(authErrorThrottle + time.Second) }
	forwarder.logsErrs = append(forwarder.logsErrs, &dash0api.APIError{StatusCode: 401})
	pool.sendLogs(context.Background(), newLogsBatch(1))
	select {
	case <-lifecycleCh:
		// expected
	case <-time.After(50 * time.Millisecond):
		t.Error("after throttle window expiry, next 401 should re-surface warning")
	}
}

func TestWorkerPool_Upstream400_ClassifiedAs4xxOther(t *testing.T) {
	forwarder := &fakeForwarder{
		logsErrs: []error{&dash0api.APIError{StatusCode: 400, Status: "400 Bad Request"}},
	}
	stats := &Stats{}
	eventCh := make(chan plog.Logs, 4)
	emitter := NewEmitter("inst", eventCh)
	consumer := NewProxyConsumer(stats, NewEmitter("inst", nil), nil)
	lifecycleCh := make(chan LifecycleEvent, 2)
	pool := NewWorkerPool(forwarder, nil, stats, emitter, consumer, lifecycleCh)

	pool.sendLogs(context.Background(), newLogsBatch(1))

	evts := drainEvents(eventCh)
	if len(evts) != 1 {
		t.Fatalf("expected 1 error event; got %d", len(evts))
	}
	if kind := firstEventKindAttr(t, evts[0]); kind != string(ErrorKindUpstream4xxOther) {
		t.Errorf("error.kind = %q; want %q", kind, ErrorKindUpstream4xxOther)
	}
	// Plain 4xx_other should NOT trigger the auth lifecycle line — those
	// are reserved for actual credential issues.
	select {
	case ev := <-lifecycleCh:
		t.Errorf("4xx_other should not push a lifecycle event; got %+v", ev)
	case <-time.After(20 * time.Millisecond):
		// expected: nothing
	}
}

func TestWorkerPool_NetworkError_ClassifiedAsUnreachable(t *testing.T) {
	forwarder := &fakeForwarder{
		logsErrs: []error{errors.New("dial tcp 127.0.0.1:443: connection refused")},
	}
	stats := &Stats{}
	eventCh := make(chan plog.Logs, 4)
	emitter := NewEmitter("inst", eventCh)
	consumer := NewProxyConsumer(stats, NewEmitter("inst", nil), nil)
	pool := NewWorkerPool(forwarder, nil, stats, emitter, consumer, nil)

	pool.sendLogs(context.Background(), newLogsBatch(1))

	evts := drainEvents(eventCh)
	if len(evts) != 1 {
		t.Fatalf("expected 1 error event; got %d", len(evts))
	}
	if kind := firstEventKindAttr(t, evts[0]); kind != string(ErrorKindUpstreamUnreachable) {
		t.Errorf("error.kind = %q; want %q", kind, ErrorKindUpstreamUnreachable)
	}
}

func TestWorkerPool_PanicRecoveredAndReported(t *testing.T) {
	forwarder := &fakeForwarder{
		onSend: panicNext(SignalLogs),
	}
	stats := &Stats{}
	eventCh := make(chan plog.Logs, 4)
	emitter := NewEmitter("inst", eventCh)
	consumer := NewProxyConsumer(stats, NewEmitter("inst", nil), nil)
	pool := NewWorkerPool(forwarder, nil, stats, emitter, consumer, nil)

	// Should NOT propagate the panic out of sendLogs.
	pool.sendLogs(context.Background(), newLogsBatch(1))

	evts := drainEvents(eventCh)
	if len(evts) != 1 {
		t.Fatalf("expected 1 panic event; got %d", len(evts))
	}
	if kind := firstEventKindAttr(t, evts[0]); kind != string(ErrorKindInternalPanic) {
		t.Errorf("error.kind = %q; want %q", kind, ErrorKindInternalPanic)
	}
}

func TestWorkerPool_DatasetPassthrough(t *testing.T) {
	forwarder := &fakeForwarder{}
	stats := &Stats{}
	consumer := NewProxyConsumer(stats, NewEmitter("inst", nil), nil)

	staging := "staging"
	pool := NewWorkerPool(forwarder, &staging, stats, NewEmitter("inst", nil), consumer, nil)
	pool.sendLogs(context.Background(), newLogsBatch(1))
	if forwarder.lastDataset == nil || *forwarder.lastDataset != "staging" {
		t.Errorf("dataset on call = %v; want pointer to %q", forwarder.lastDataset, "staging")
	}

	// Passing nil dataset should reach the forwarder as nil — meaning "use
	// the profile's default" per the dash0api convention.
	forwarder.lastDataset = &staging // sentinel: should be overwritten with nil
	poolNil := NewWorkerPool(forwarder, nil, stats, NewEmitter("inst", nil), consumer, nil)
	poolNil.sendLogs(context.Background(), newLogsBatch(1))
	if forwarder.lastDataset != nil {
		t.Errorf("nil dataset should pass through as nil; got %v", forwarder.lastDataset)
	}
}

func TestWorkerPool_TracesAndMetricsHaveSameOutcomeContract(t *testing.T) {
	// Verify the worker pool wiring is the same for all three signals: a
	// 503 on traces increments Failed(spans) and emits an error event; same
	// for metrics. Logs is already covered above.
	forwarder := &fakeForwarder{
		tracesErrs:  []error{&dash0api.APIError{StatusCode: 503}},
		metricsErrs: []error{&dash0api.APIError{StatusCode: 503}},
	}
	stats := &Stats{}
	eventCh := make(chan plog.Logs, 4)
	emitter := NewEmitter("inst", eventCh)
	consumer := NewProxyConsumer(stats, NewEmitter("inst", nil), nil)
	pool := NewWorkerPool(forwarder, nil, stats, emitter, consumer, nil)

	pool.sendTraces(context.Background(), newTracesBatch(2))
	pool.sendMetrics(context.Background(), newMetricsBatch(3))

	if got := stats.Failed(SignalSpans); got != 2 {
		t.Errorf("Failed(spans) = %d; want 2", got)
	}
	if got := stats.Failed(SignalMetrics); got != 3 {
		t.Errorf("Failed(metrics) = %d; want 3", got)
	}
	if evts := drainEvents(eventCh); len(evts) != 2 {
		t.Errorf("expected 2 error events (one per signal); got %d", len(evts))
	}
}

func TestWorkerPool_Run_DrainsAllSignals(t *testing.T) {
	// End-to-end through the channels: enqueue via ConsumeX, run the pool,
	// verify all three signal goroutines drain.
	forwarder := &fakeForwarder{}
	stats := &Stats{}
	consumer := NewProxyConsumer(stats, NewEmitter("inst", nil), nil)
	pool := NewWorkerPool(forwarder, nil, stats, NewEmitter("inst", nil), consumer, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		pool.Run(ctx)
		close(done)
	}()

	if err := consumer.ConsumeLogs(ctx, newLogsBatch(1)); err != nil {
		t.Fatalf("ConsumeLogs: %v", err)
	}
	if err := consumer.ConsumeTraces(ctx, newTracesBatch(1)); err != nil {
		t.Fatalf("ConsumeTraces: %v", err)
	}
	if err := consumer.ConsumeMetrics(ctx, newMetricsBatch(1)); err != nil {
		t.Fatalf("ConsumeMetrics: %v", err)
	}

	// Wait until all three goroutines have processed their batch.
	deadline := time.After(500 * time.Millisecond)
	for {
		l := forwarder.logsCalls.Load()
		tr := forwarder.tracesCalls.Load()
		m := forwarder.metricsCalls.Load()
		if l >= 1 && tr >= 1 && m >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("not all signals drained; calls = logs=%d traces=%d metrics=%d", l, tr, m)
		case <-time.After(5 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("pool.Run did not exit within 200ms of context cancel")
	}
}

func TestWorkerPool_Run_PanicRecoversInGoroutine(t *testing.T) {
	// The drain goroutine should keep accepting after a single batch
	// panicked. Otherwise a single bad batch would stop the whole signal.
	forwarder := &fakeForwarder{
		onSend: panicNext(SignalLogs),
	}
	stats := &Stats{}
	eventCh := make(chan plog.Logs, 4)
	consumer := NewProxyConsumer(stats, NewEmitter("inst", nil), nil)
	pool := NewWorkerPool(forwarder, nil, stats, NewEmitter("inst", eventCh), consumer, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		pool.Run(ctx)
		close(done)
	}()

	// First batch panics; second batch should still be processed.
	if err := consumer.ConsumeLogs(ctx, newLogsBatch(1)); err != nil {
		t.Fatalf("ConsumeLogs panic batch: %v", err)
	}
	if err := consumer.ConsumeLogs(ctx, newLogsBatch(1)); err != nil {
		t.Fatalf("ConsumeLogs subsequent: %v", err)
	}

	deadline := time.After(500 * time.Millisecond)
	for {
		if forwarder.logsCalls.Load() >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("worker did not recover after panic; SendLogs called %d times", forwarder.logsCalls.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
}
