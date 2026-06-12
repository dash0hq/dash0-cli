//go:build integration

package otlp

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
)

// recordingForwarder captures Send-method invocations for assertion. Used
// to verify the receiver → consumer → worker → forwarder chain end-to-
// end without standing up a real dash0api.Client.
type recordingForwarder struct {
	mu sync.Mutex

	logs    []plog.Logs
	traces  []ptrace.Traces
	metrics []pmetric.Metrics

	// returnErr, when set, makes each Send return that error. Used to
	// drive the worker-pool failure-classification paths.
	returnErr error

	// receivedDatasets tracks which dataset values workers forward. The
	// proxy's outbound dataset is read from the active profile via
	// client.ResolveDataset; in this fixture we pass it explicitly.
	receivedDatasets []*string
}

func (f *recordingForwarder) SendLogs(_ context.Context, ld plog.Logs, dataset *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, ld)
	f.receivedDatasets = append(f.receivedDatasets, dataset)
	return f.returnErr
}

func (f *recordingForwarder) SendTraces(_ context.Context, td ptrace.Traces, dataset *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.traces = append(f.traces, td)
	f.receivedDatasets = append(f.receivedDatasets, dataset)
	return f.returnErr
}

func (f *recordingForwarder) SendMetrics(_ context.Context, md pmetric.Metrics, dataset *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.metrics = append(f.metrics, md)
	f.receivedDatasets = append(f.receivedDatasets, dataset)
	return f.returnErr
}

func (f *recordingForwarder) logsCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.logs)
}

func (f *recordingForwarder) tracesCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.traces)
}

func (f *recordingForwarder) metricsCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.metrics)
}

// integrationHarness builds the full proxy pipeline (consumer + workers +
// pipeline + emitter + stats) with a recordingForwarder downstream. The
// returned closer cleanly shuts the pipeline and waits for workers.
type integrationHarness struct {
	t         *testing.T
	forwarder *recordingForwarder
	stats     *Stats
	emitter   *Emitter
	consumer  *ProxyConsumer
	pipeline  *Pipeline
	workers   *WorkerPool
	eventCh   chan plog.Logs

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newIntegrationHarness(t *testing.T, dataset *string) *integrationHarness {
	t.Helper()

	forwarder := &recordingForwarder{}
	stats := &Stats{}
	eventCh := make(chan plog.Logs, 64)
	emitter := NewEmitter("test-instance", eventCh)
	consumer := NewProxyConsumer(stats, emitter, nil)

	httpPort := pickFreePort(t)
	grpcPort := pickFreePort(t)
	pipeline, err := BuildPipeline(context.Background(), httpPort, grpcPort, consumer)
	if err != nil {
		t.Fatalf("BuildPipeline: %v", err)
	}

	workers := NewWorkerPool(forwarder, dataset, stats, emitter, consumer, nil)

	ctx, cancel := context.WithCancel(context.Background())
	h := &integrationHarness{
		t:         t,
		forwarder: forwarder,
		stats:     stats,
		emitter:   emitter,
		consumer:  consumer,
		pipeline:  pipeline,
		workers:   workers,
		eventCh:   eventCh,
		ctx:       ctx,
		cancel:    cancel,
	}

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		workers.Run(ctx)
	}()

	if err := pipeline.Start(ctx); err != nil {
		cancel()
		t.Fatalf("pipeline.Start: %v", err)
	}

	return h
}

func (h *integrationHarness) shutdown() {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = h.pipeline.Shutdown(shutdownCtx)
	h.cancel()
	h.wg.Wait()
}

// HTTPEndpoint returns the bound HTTP listener address.
func (h *integrationHarness) HTTPEndpoint() string {
	return h.pipeline.Endpoints().HTTPEndpoint
}

// waitForCount polls a counter until it reaches want or the deadline expires.
func waitForCount(t *testing.T, fetch func() int, want int, label string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		if fetch() >= want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("%s: count did not reach %d in 2s; current=%d", label, want, fetch())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestIntegration_HTTPJSONLogsForwarded(t *testing.T) {
	h := newIntegrationHarness(t, nil)
	defer h.shutdown()

	// Build an OTLP/JSON logs payload with a recognizable body string.
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "integration-test")
	sl := rl.ScopeLogs().AppendEmpty()
	sl.LogRecords().AppendEmpty().Body().SetStr("hello from integration test")
	body, err := plogotlp.NewExportRequestFromLogs(ld).MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	url := "http://" + h.HTTPEndpoint() + "/v1/logs"
	resp := doPost(t, url, body, "application/json")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HTTP status = %d; want 200", resp.StatusCode)
	}

	waitForCount(t, h.forwarder.logsCount, 1, "logs forwarded")

	// Assert the forwarded pdata round-tripped intact.
	forwarded := h.forwarder.logs[0]
	if forwarded.LogRecordCount() != 1 {
		t.Errorf("forwarded LogRecordCount = %d; want 1", forwarded.LogRecordCount())
	}
	got := forwarded.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Body().Str()
	if got != "hello from integration test" {
		t.Errorf("forwarded body = %q; want roundtripped 'hello from integration test'", got)
	}
}

func TestIntegration_HTTPJSONTracesForwarded(t *testing.T) {
	h := newIntegrationHarness(t, nil)
	defer h.shutdown()

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "integration-test")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Spans().AppendEmpty().SetName("integration-span")
	body, err := ptraceotlp.NewExportRequestFromTraces(td).MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	url := "http://" + h.HTTPEndpoint() + "/v1/traces"
	resp := doPost(t, url, body, "application/json")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HTTP status = %d; want 200", resp.StatusCode)
	}

	waitForCount(t, h.forwarder.tracesCount, 1, "traces forwarded")

	forwarded := h.forwarder.traces[0]
	if forwarded.SpanCount() != 1 {
		t.Errorf("forwarded SpanCount = %d; want 1", forwarded.SpanCount())
	}
	got := forwarded.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Name()
	if got != "integration-span" {
		t.Errorf("forwarded span name = %q; want 'integration-span'", got)
	}
}

func TestIntegration_HTTPJSONMetricsForwarded(t *testing.T) {
	h := newIntegrationHarness(t, nil)
	defer h.shutdown()

	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "integration-test")
	sm := rm.ScopeMetrics().AppendEmpty()
	m := sm.Metrics().AppendEmpty()
	m.SetName("integration_counter")
	m.SetEmptyGauge().DataPoints().AppendEmpty().SetIntValue(42)
	body, err := pmetricotlp.NewExportRequestFromMetrics(md).MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	url := "http://" + h.HTTPEndpoint() + "/v1/metrics"
	resp := doPost(t, url, body, "application/json")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HTTP status = %d; want 200", resp.StatusCode)
	}

	waitForCount(t, h.forwarder.metricsCount, 1, "metrics forwarded")

	forwarded := h.forwarder.metrics[0]
	got := forwarded.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Name()
	if got != "integration_counter" {
		t.Errorf("forwarded metric name = %q; want 'integration_counter'", got)
	}
}

func TestIntegration_DatasetPassthrough(t *testing.T) {
	staging := "staging"
	h := newIntegrationHarness(t, &staging)
	defer h.shutdown()

	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	sl.LogRecords().AppendEmpty().Body().SetStr("with dataset")
	body, _ := plogotlp.NewExportRequestFromLogs(ld).MarshalJSON()

	url := "http://" + h.HTTPEndpoint() + "/v1/logs"
	resp := doPost(t, url, body, "application/json")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HTTP status = %d; want 200", resp.StatusCode)
	}

	waitForCount(t, h.forwarder.logsCount, 1, "logs forwarded with dataset")

	if got := h.forwarder.receivedDatasets[0]; got == nil || *got != "staging" {
		t.Errorf("forwarded dataset = %v; want pointer to 'staging'", got)
	}
}

func TestIntegration_StatsCountersTrack(t *testing.T) {
	h := newIntegrationHarness(t, nil)
	defer h.shutdown()

	// Post three log batches; each carries two records.
	for i := 0; i < 3; i++ {
		ld := plog.NewLogs()
		rl := ld.ResourceLogs().AppendEmpty()
		sl := rl.ScopeLogs().AppendEmpty()
		sl.LogRecords().AppendEmpty().Body().SetStr("rec-A")
		sl.LogRecords().AppendEmpty().Body().SetStr("rec-B")
		body, _ := plogotlp.NewExportRequestFromLogs(ld).MarshalJSON()
		resp := doPost(t, "http://"+h.HTTPEndpoint()+"/v1/logs", body, "application/json")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("batch %d HTTP status = %d; want 200", i, resp.StatusCode)
		}
	}

	waitForCount(t, h.forwarder.logsCount, 3, "all three log batches forwarded")

	if got := h.stats.Forwarded(SignalLogs); got != 6 {
		t.Errorf("Stats.Forwarded(logs) = %d; want 6 (3 batches × 2 records)", got)
	}
}

// doPost is a tiny helper that consolidates request construction and
// content-type headers.
func doPost(t *testing.T, url string, body []byte, contentType string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", contentType)

	// Retry briefly to allow the listener to come up.
	var resp *http.Response
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err = http.DefaultClient.Do(req)
		if err == nil {
			return resp
		}
		time.Sleep(10 * time.Millisecond)
		// Re-create the request because http reads the body once.
		req, _ = http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", contentType)
	}
	t.Fatalf("POST %s failed after retries: %v", url, err)
	return nil
}

// Stress: send batches faster than the forwarder can drain to drive the
// queue toward saturation. Verifies that the receiver returns HTTP 503
// (mapped from the consumer's retryable error) when the queue is full.
//
// We use a forwarder that blocks until released, so the worker can't
// drain. After enqueuing signalQueueDepth+1 batches, the next inbound
// request must return 503.
func TestIntegration_QueueSaturationReturns503(t *testing.T) {
	blocked := make(chan struct{})
	forwarder := &blockingForwarder{release: blocked}
	stats := &Stats{}
	consumer := NewProxyConsumer(stats, NewEmitter("inst", nil), nil)

	httpPort := pickFreePort(t)
	grpcPort := pickFreePort(t)
	pipeline, err := BuildPipeline(context.Background(), httpPort, grpcPort, consumer)
	if err != nil {
		t.Fatalf("BuildPipeline: %v", err)
	}
	workers := NewWorkerPool(forwarder, nil, stats, NewEmitter("inst", nil), consumer, nil)
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); workers.Run(ctx) }()

	if err := pipeline.Start(ctx); err != nil {
		cancel()
		close(blocked)
		wg.Wait()
		t.Fatalf("pipeline.Start: %v", err)
	}

	// Cleanup in dependency order: release blocked forwarder first so
	// in-flight Sends finish, then shut down the pipeline (which can drain
	// cleanly), then cancel the worker context, then wait for the worker
	// goroutine. Doing this via t.Cleanup with explicit ordering — defers
	// would deadlock because pipeline.Shutdown blocks on the worker while
	// the worker is still blocked on `release`.
	t.Cleanup(func() {
		close(blocked)
		shutdownCtx, c := context.WithTimeout(context.Background(), 2*time.Second)
		defer c()
		_ = pipeline.Shutdown(shutdownCtx)
		cancel()
		wg.Wait()
	})

	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	sl.LogRecords().AppendEmpty().Body().SetStr("saturate")
	body, _ := plogotlp.NewExportRequestFromLogs(ld).MarshalJSON()

	url := "http://127.0.0.1:" + strconv.Itoa(httpPort) + "/v1/logs"
	// First batch is held inside the worker (blockingForwarder); the next
	// signalQueueDepth batches fill the per-signal channel. After that,
	// new POSTs hit a full channel and the consumer returns a retryable
	// error → receiver responds 503.
	var saw503 bool
	for i := 0; i < signalQueueDepth+10; i++ {
		resp := doPost(t, url, body, "application/json")
		if resp.StatusCode == http.StatusServiceUnavailable {
			saw503 = true
			_ = resp.Body.Close()
			break
		}
		_ = resp.Body.Close()
	}
	if !saw503 {
		t.Errorf("expected at least one 503 once the queue saturated; never saw one in %d requests", signalQueueDepth+10)
	}
}

// blockingForwarder is a Forwarder that holds its Send calls until
// `release` is closed. Used to drive queue saturation tests.
type blockingForwarder struct {
	release <-chan struct{}
}

func (b *blockingForwarder) SendLogs(_ context.Context, _ plog.Logs, _ *string) error {
	<-b.release
	return nil
}
func (b *blockingForwarder) SendTraces(_ context.Context, _ ptrace.Traces, _ *string) error {
	<-b.release
	return nil
}
func (b *blockingForwarder) SendMetrics(_ context.Context, _ pmetric.Metrics, _ *string) error {
	<-b.release
	return nil
}
