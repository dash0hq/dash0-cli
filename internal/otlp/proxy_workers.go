package otlp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Forwarder abstracts the dash0api.Client OTLP send surface so the worker
// pool can be exercised with a fake in unit tests without standing up the
// full API client. dash0api.Client satisfies this implicitly.
type Forwarder interface {
	SendLogs(ctx context.Context, ld plog.Logs, dataset *string) error
	SendTraces(ctx context.Context, td ptrace.Traces, dataset *string) error
	SendMetrics(ctx context.Context, md pmetric.Metrics, dataset *string) error
}

// authErrorThrottle bounds how often the worker pool surfaces an auth-error
// lifecycle line on stderr. 401s and 403s cluster (SDKs retry the same bad
// creds), and one "authentication to Dash0 failed" line per 30s
// communicates the root cause without filling the terminal.
const authErrorThrottle = 30 * time.Second

// WorkerPool drains the per-signal channels populated by ProxyConsumer (U13),
// calls the upstream Forwarder, classifies outcomes per KTD14, and surfaces
// failures via the Stats counters, the agent-mode event channel, and an
// at-most-once-per-throttle-window stderr line for credential issues.
//
// Concurrency is one goroutine per signal — the transport's
// max-concurrent-requests semaphore (KTD3b) provides the actual outbound
// concurrency cap, and per-signal goroutines keep batch ordering intact for
// downstream sanity (e.g., a metrics burst arriving in order goes upstream
// in order, modulo SDK retries).
type WorkerPool struct {
	forwarder Forwarder
	dataset   *string
	stats     *Stats
	emitter   *Emitter
	consumer  *ProxyConsumer
	decorator *Decorator

	// lifecycleCh receives the at-most-once auth-failure warning. nil in
	// non-agent / non-TTY runs; the stderr writer also suppresses lifecycle
	// rendering when piped, so the pool only needs to avoid blocking the
	// hot path.
	lifecycleCh chan<- LifecycleEvent

	// now is overridable so tests can drive the auth-error throttle without
	// real sleeps.
	now func() time.Time

	authMu       sync.Mutex
	lastAuthWarn time.Time
}

// NewWorkerPool constructs a pool. No goroutines are started until Run is
// called. The decorator may be nil — workers treat that the same as an
// empty decorator and skip the per-batch upsert step.
func NewWorkerPool(forwarder Forwarder, dataset *string, stats *Stats, emitter *Emitter, consumer *ProxyConsumer, lifecycleCh chan<- LifecycleEvent, decorator *Decorator) *WorkerPool {
	return &WorkerPool{
		forwarder:   forwarder,
		dataset:     dataset,
		stats:       stats,
		emitter:     emitter,
		consumer:    consumer,
		decorator:   decorator,
		lifecycleCh: lifecycleCh,
		now:         time.Now,
	}
}

// Run launches one goroutine per signal and blocks until ctx is cancelled.
// When ctx fires, each worker stops accepting new work from its channel; any
// in-flight forward finishes (the transport has its own per-request
// deadline) and the worker returns. Tests that want immediate shutdown
// should cancel and not wait on queued batches.
func (p *WorkerPool) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(signalCount)
	go func() {
		defer wg.Done()
		p.drain(ctx, SignalLogs)
	}()
	go func() {
		defer wg.Done()
		p.drain(ctx, SignalSpans)
	}()
	go func() {
		defer wg.Done()
		p.drain(ctx, SignalMetrics)
	}()
	wg.Wait()
}

func (p *WorkerPool) drain(ctx context.Context, sig Signal) {
	switch sig {
	case SignalLogs:
		ch := p.consumer.LogsChannel()
		for {
			select {
			case <-ctx.Done():
				return
			case ld, ok := <-ch:
				if !ok {
					return
				}
				p.sendLogs(ctx, ld)
			}
		}
	case SignalSpans:
		ch := p.consumer.TracesChannel()
		for {
			select {
			case <-ctx.Done():
				return
			case td, ok := <-ch:
				if !ok {
					return
				}
				p.sendTraces(ctx, td)
			}
		}
	case SignalMetrics:
		ch := p.consumer.MetricsChannel()
		for {
			select {
			case <-ctx.Done():
				return
			case md, ok := <-ch:
				if !ok {
					return
				}
				p.sendMetrics(ctx, md)
			}
		}
	}
}

func (p *WorkerPool) sendLogs(ctx context.Context, ld plog.Logs) {
	count := ld.LogRecordCount()
	defer p.recoverPanic(SignalLogs)
	p.decorator.DecorateLogs(ld)
	err := p.forwarder.SendLogs(ctx, ld, p.dataset)
	p.classifyOutcome(SignalLogs, count, err)
}

func (p *WorkerPool) sendTraces(ctx context.Context, td ptrace.Traces) {
	count := td.SpanCount()
	defer p.recoverPanic(SignalSpans)
	p.decorator.DecorateTraces(td)
	err := p.forwarder.SendTraces(ctx, td, p.dataset)
	p.classifyOutcome(SignalSpans, count, err)
}

func (p *WorkerPool) sendMetrics(ctx context.Context, md pmetric.Metrics) {
	count := md.DataPointCount()
	defer p.recoverPanic(SignalMetrics)
	p.decorator.DecorateMetrics(md)
	err := p.forwarder.SendMetrics(ctx, md, p.dataset)
	p.classifyOutcome(SignalMetrics, count, err)
}

// classifyOutcome turns a Send result into the KTD14 taxonomy and updates
// the failure counter, agent-mode error event, and the auth-warning
// lifecycle line. Success is a no-op — Forwarded is incremented by the
// consumer at enqueue time, and per-signal "success rate" is
// `Forwarded - Failed`.
func (p *WorkerPool) classifyOutcome(sig Signal, count int, err error) {
	if err == nil {
		return
	}
	kind, code := classifyError(err)
	p.stats.RecordFailed(sig, count)
	p.emitter.EmitError(kind, err.Error(), code)
	if kind == ErrorKindUpstream4xxAuth {
		p.maybeSurfaceAuthError()
	}
}

func (p *WorkerPool) recoverPanic(sig Signal) {
	if r := recover(); r != nil {
		// Don't bump the Failed counter — by the time the panic surfaces
		// the batch's count attribution is unreliable. The error event
		// itself ensures the panic isn't silently swallowed; downstream
		// retries are SDK-controlled.
		reason := fmt.Sprintf("worker panic in %s forwarder: %v", sig, r)
		p.emitter.EmitError(ErrorKindInternalPanic, reason, 0)
	}
}

func (p *WorkerPool) maybeSurfaceAuthError() {
	if p.lifecycleCh == nil {
		return
	}
	p.authMu.Lock()
	defer p.authMu.Unlock()
	now := p.now()
	if !p.lastAuthWarn.IsZero() && now.Sub(p.lastAuthWarn) < authErrorThrottle {
		return
	}
	select {
	case p.lifecycleCh <- LifecycleEvent{
		Kind:    LifecycleError,
		Message: "authentication to Dash0 failed; check your profile (re-run `dash0 config show`)",
	}:
		p.lastAuthWarn = now
	default:
		// Lifecycle channel full; skip this warning. The next 401 outside
		// the throttle window will retry.
	}
}

// classifyError maps a Send error into the proxy's ErrorKind taxonomy and
// the originating HTTP status code (0 when the error has no APIError
// payload — i.e., a network or transport-level failure). Uses errors.As so
// wrapped APIErrors are still classified correctly.
func classifyError(err error) (ErrorKind, int) {
	var apiErr *dash0api.APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode == 401 || apiErr.StatusCode == 403:
			return ErrorKindUpstream4xxAuth, apiErr.StatusCode
		case apiErr.StatusCode >= 400 && apiErr.StatusCode < 500:
			return ErrorKindUpstream4xxOther, apiErr.StatusCode
		case apiErr.StatusCode >= 500 && apiErr.StatusCode < 600:
			return ErrorKindUpstream5xx, apiErr.StatusCode
		}
	}
	return ErrorKindUpstreamUnreachable, 0
}
