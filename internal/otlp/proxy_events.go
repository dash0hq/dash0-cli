package otlp

import (
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

// Agent-mode event-name namespace. Future long-running commands (e.g.,
// `dash0 logs send --tail`) define their own subnamespace; the schema below
// is proxy-specific.
const (
	scopeName = "dash0.cli.otlp_proxy"

	eventStarted    = "dash0.cli.otlp_proxy.started"
	eventForwarded  = "dash0.cli.otlp_proxy.forwarded"
	eventStats      = "dash0.cli.otlp_proxy.stats"
	eventError      = "dash0.cli.otlp_proxy.error"
	eventShutdown   = "dash0.cli.otlp_proxy.shutdown"
)

// ErrorKind classifies a worker outbound failure (KTD14). Agents reading the
// event stream can use this to differentiate transient upstream errors (the
// SDK should keep retrying) from terminal ones (the proxy's own credentials
// are bad and retry won't help).
type ErrorKind string

const (
	ErrorKindUpstreamUnreachable ErrorKind = "upstream_unreachable"
	ErrorKindUpstream5xx         ErrorKind = "upstream_5xx"
	ErrorKindUpstream4xxAuth     ErrorKind = "upstream_4xx_auth"
	ErrorKindUpstream4xxOther    ErrorKind = "upstream_4xx_other"
	ErrorKindInternalPanic       ErrorKind = "internal_panic"
)

// Emitter builds OTLP/JSON event records about the proxy's own lifecycle and
// sends them on a channel. The actual stdout write is performed by a
// separate writer goroutine (StdoutWriter) so the hot path (consumer +
// workers calling EmitForwarded / EmitError) never blocks on I/O.
//
// When the channel is nil (the no-agent-mode case), every Emit method is a
// silent no-op — the caller constructs one Emitter shape regardless of mode
// and the events naturally drop when no one is listening.
type Emitter struct {
	// instanceID is the per-process UUID used as service.instance.id on
	// every emitted record (KTD15). Set once at proxy start; never changes
	// across the process lifetime.
	instanceID string
	ch         chan<- plog.Logs

	// now is overridable so tests can produce deterministic timestamps.
	now func() time.Time
}

// NewEmitter returns an Emitter that pushes events on ch. Pass a nil channel
// in non-agent mode to turn every Emit call into a no-op.
func NewEmitter(instanceID string, ch chan<- plog.Logs) *Emitter {
	return &Emitter{
		instanceID: instanceID,
		ch:         ch,
		now:        time.Now,
	}
}

// EmitStarted emits the proxy-startup event once both listeners are bound
// and the receiver is ready to accept traffic.
func (e *Emitter) EmitStarted(httpEndpoint, grpcEndpoint, dataset, profileName string) {
	if e == nil || e.ch == nil {
		return
	}
	ld := e.build(eventStarted, func(attrs pcommon.Map) {
		if httpEndpoint != "" {
			attrs.PutStr("endpoint.http", httpEndpoint)
		}
		if grpcEndpoint != "" {
			attrs.PutStr("endpoint.grpc", grpcEndpoint)
		}
		attrs.PutStr("dataset", dataset)
		attrs.PutStr("profile.name", profileName)
	})
	e.send(ld)
}

// EmitForwarded emits one event per inbound batch the consumer enqueued for
// forwarding.
func (e *Emitter) EmitForwarded(sig Signal, count, bytes int64) {
	if e == nil || e.ch == nil {
		return
	}
	ld := e.build(eventForwarded, func(attrs pcommon.Map) {
		attrs.PutStr("signal", sig.String())
		attrs.PutInt("count", count)
		attrs.PutInt("bytes", bytes)
	})
	e.send(ld)
}

// EmitStats emits the per-interval rate + total snapshot for agents to
// consume. Carries the same data as the TTY stats line, in structured form.
func (e *Emitter) EmitStats(snap SnapshotWithRate) {
	if e == nil || e.ch == nil {
		return
	}
	ld := e.build(eventStats, func(attrs pcommon.Map) {
		attrs.PutDouble("logs.rate", snap.Rate[SignalLogs])
		attrs.PutInt("logs.total", snap.Forwarded[SignalLogs])
		attrs.PutInt("logs.failed", snap.Failed[SignalLogs])
		attrs.PutDouble("spans.rate", snap.Rate[SignalSpans])
		attrs.PutInt("spans.total", snap.Forwarded[SignalSpans])
		attrs.PutInt("spans.failed", snap.Failed[SignalSpans])
		attrs.PutDouble("metrics.rate", snap.Rate[SignalMetrics])
		attrs.PutInt("metrics.total", snap.Forwarded[SignalMetrics])
		attrs.PutInt("metrics.failed", snap.Failed[SignalMetrics])
	})
	e.send(ld)
}

// EmitError emits an error event with KTD14 classification. code is the
// HTTP/gRPC status code from upstream when applicable; pass 0 when not.
func (e *Emitter) EmitError(kind ErrorKind, reason string, code int) {
	if e == nil || e.ch == nil {
		return
	}
	ld := e.build(eventError, func(attrs pcommon.Map) {
		attrs.PutStr("error.kind", string(kind))
		if reason != "" {
			attrs.PutStr("reason", reason)
		}
		if code != 0 {
			attrs.PutInt("code", int64(code))
		}
	})
	e.send(ld)
}

// EmitShutdown emits the terminating event with final cumulative totals.
// Callers should invoke this synchronously before cancelling the writer's
// context so the event lands before the process exits.
func (e *Emitter) EmitShutdown(reason string, finalTotals [signalCount]int64) {
	if e == nil || e.ch == nil {
		return
	}
	ld := e.build(eventShutdown, func(attrs pcommon.Map) {
		attrs.PutStr("reason", reason)
		attrs.PutInt("final_total.logs", finalTotals[SignalLogs])
		attrs.PutInt("final_total.spans", finalTotals[SignalSpans])
		attrs.PutInt("final_total.metrics", finalTotals[SignalMetrics])
	})
	e.send(ld)
}

// build constructs a plog.Logs with one LogRecord carrying eventName and the
// caller-supplied record attributes. Resource attributes (service.name,
// service.instance.id) and timestamps are populated uniformly.
func (e *Emitter) build(eventName string, setAttrs func(pcommon.Map)) plog.Logs {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "dash0-cli")
	rl.Resource().Attributes().PutStr("service.instance.id", e.instanceID)
	sl := rl.ScopeLogs().AppendEmpty()
	sl.Scope().SetName(scopeName)
	lr := sl.LogRecords().AppendEmpty()
	lr.SetEventName(eventName)
	now := e.now()
	ts := pcommon.NewTimestampFromTime(now)
	lr.SetTimestamp(ts)
	lr.SetObservedTimestamp(ts)
	setAttrs(lr.Attributes())
	return ld
}

// send pushes ld to the event channel without blocking. If the writer's
// channel is full (the writer goroutine is behind), the event is dropped
// rather than back-pressuring the caller — the hot path must never stall
// behind a slow stdout consumer.
func (e *Emitter) send(ld plog.Logs) {
	select {
	case e.ch <- ld:
	default:
		// dropped — writer is overwhelmed; better to lose this event than
		// stall a worker or consumer goroutine.
	}
}
