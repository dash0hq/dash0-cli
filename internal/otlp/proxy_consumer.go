package otlp

import (
	"context"
	"errors"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// signalQueueDepth is the per-signal channel buffer between the consumer
// and the worker pool. 128 deep gives the 10-concurrent transport (KTD3b)
// headroom under bursty SDK output; saturation triggers backpressure
// signalling (HTTP 503 / gRPC UNAVAILABLE) per KTD3a.
const signalQueueDepth = 128

// errQueueFull signals to the otlpreceiver that the proxy's per-signal
// queue has filled. The receiver maps this (non-permanent) error to the
// SDK as HTTP 503 / gRPC UNAVAILABLE per the OTLP spec, triggering the
// SDK's mandated exponential backoff. Always wrapped via
// consumererror.NewRetryableError so intent is explicit.
var errQueueFull = errors.New("proxy queue full")

// ProxyConsumer implements consumer.Logs / consumer.Traces / consumer.Metrics
// and bridges the otlpreceiver's pdata delivery into the worker pool's
// per-signal channels.
//
// Per KTD3a (async-forward), the consumer returns nil (mapping to HTTP 200
// / gRPC OK) after enqueueing — the actual upstream forward happens
// asynchronously on the worker goroutines. The OTLP spec scopes acceptance
// to a single hop, so this is spec-compliant.
//
// Lifecycle side-effects on each Consume call:
//   1. stats forwarded counter incremented (U6) — drives both the TTY
//      sparkline (U7) and the agent-mode stats event (U8).
//   2. agent-mode `dash0.cli.otlp_proxy.forwarded` event emitted (U8).
//   3. --tail rendering pushed to TailCh when --tail is enabled (U7/U9).
//   4. Non-blocking enqueue to the per-signal channel.
//   5. Non-empty channel → return nil (200). Full channel → return retryable
//      error (503 + SDK exponential backoff).
type ProxyConsumer struct {
	stats   *Stats
	emitter *Emitter

	// tailCh, when non-nil, receives the rendered --tail string for each
	// inbound batch. Setting it to nil disables --tail rendering on the
	// hot path.
	tailCh chan<- string

	// Per-signal channels read by the worker pool (U4). Exposed via
	// accessor methods so the workers can pick them up without holding a
	// direct reference to the consumer.
	logsCh    chan plog.Logs
	tracesCh  chan ptrace.Traces
	metricsCh chan pmetric.Metrics
}

// NewProxyConsumer constructs the consumer plus its three per-signal
// channels. tailCh may be nil; when non-nil the consumer pushes the
// debug-exporter-style rendering of each inbound batch to it.
func NewProxyConsumer(stats *Stats, emitter *Emitter, tailCh chan<- string) *ProxyConsumer {
	return &ProxyConsumer{
		stats:     stats,
		emitter:   emitter,
		tailCh:    tailCh,
		logsCh:    make(chan plog.Logs, signalQueueDepth),
		tracesCh:  make(chan ptrace.Traces, signalQueueDepth),
		metricsCh: make(chan pmetric.Metrics, signalQueueDepth),
	}
}

// LogsChannel returns the channel the worker pool drains for log batches.
func (c *ProxyConsumer) LogsChannel() <-chan plog.Logs { return c.logsCh }

// TracesChannel returns the channel the worker pool drains for trace batches.
func (c *ProxyConsumer) TracesChannel() <-chan ptrace.Traces { return c.tracesCh }

// MetricsChannel returns the channel the worker pool drains for metric batches.
func (c *ProxyConsumer) MetricsChannel() <-chan pmetric.Metrics { return c.metricsCh }

// Capabilities reports MutatesData=false so the receiver knows it can hand
// us the pdata directly without making a defensive copy. The consumer
// never modifies inbound data — workers send it through dash0api.Client
// as-is.
func (c *ProxyConsumer) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

// ConsumeLogs implements consumer.Logs.
func (c *ProxyConsumer) ConsumeLogs(_ context.Context, ld plog.Logs) error {
	count := ld.LogRecordCount()
	c.observe(SignalLogs, count, func() string { return RenderLogs(ld) })

	select {
	case c.logsCh <- ld:
		return nil
	default:
		return consumererror.NewRetryableError(errQueueFull)
	}
}

// ConsumeTraces implements consumer.Traces.
func (c *ProxyConsumer) ConsumeTraces(_ context.Context, td ptrace.Traces) error {
	count := td.SpanCount()
	c.observe(SignalSpans, count, func() string { return RenderTraces(td) })

	select {
	case c.tracesCh <- td:
		return nil
	default:
		return consumererror.NewRetryableError(errQueueFull)
	}
}

// ConsumeMetrics implements consumer.Metrics.
func (c *ProxyConsumer) ConsumeMetrics(_ context.Context, md pmetric.Metrics) error {
	count := md.DataPointCount()
	c.observe(SignalMetrics, count, func() string { return RenderMetrics(md) })

	select {
	case c.metricsCh <- md:
		return nil
	default:
		return consumererror.NewRetryableError(errQueueFull)
	}
}

// observe fans the lifecycle side-effects of an accepted batch out to the
// stats counters, the agent-mode event stream, and the --tail rendering
// channel. The renderer closure is evaluated lazily so we don't pay the
// CPU cost when --tail is disabled (the common case).
func (c *ProxyConsumer) observe(sig Signal, count int, render func() string) {
	if count <= 0 {
		return
	}
	c.stats.RecordForwarded(sig, count)
	// bytes=0 for v1: pdata doesn't expose a cheap byte count, and the
	// "is it flowing?" UX is more usefully driven by record counts. We
	// can add a proto-marshal-based estimate later if it proves useful.
	c.emitter.EmitForwarded(sig, int64(count), 0)

	if c.tailCh != nil {
		rendered := render()
		if rendered == "" {
			return
		}
		select {
		case c.tailCh <- rendered:
		default:
			// --tail writer is slow; drop this rendering so the consumer
			// stays on the hot path. The forward itself is unaffected —
			// stats and the forward queue still progress.
		}
	}
}
