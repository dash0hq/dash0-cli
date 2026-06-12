package otlp

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Signal enumerates the three OTLP signal types the proxy tracks. Profiles
// is intentionally absent — the OTel Profiles signal is not stable and
// adding it would lock the proxy to a moving target.
type Signal int

const (
	SignalLogs Signal = iota
	SignalSpans
	SignalMetrics

	signalCount = 3
)

// String returns the lowercase signal name used in stats lines, NDJSON event
// attributes, and `--tail` output headers.
func (s Signal) String() string {
	switch s {
	case SignalLogs:
		return "logs"
	case SignalSpans:
		return "spans"
	case SignalMetrics:
		return "metrics"
	default:
		return "unknown"
	}
}

// Stats holds atomic counters per signal. Methods are concurrency-safe; the
// hot path (consumer.ConsumeX + worker outcome) calls them with no locking
// overhead beyond an atomic add.
type Stats struct {
	forwarded [signalCount]atomic.Int64
	failed    [signalCount]atomic.Int64
}

// RecordForwarded adds n to the per-signal forwarded counter. The consumer
// (U13) calls this at enqueue time — it counts "accepted by the proxy", not
// "delivered to Dash0". Upstream failures are tracked separately via
// RecordFailed.
func (s *Stats) RecordForwarded(sig Signal, n int) {
	if sig < 0 || sig >= signalCount || n <= 0 {
		return
	}
	s.forwarded[sig].Add(int64(n))
}

// RecordFailed adds n to the per-signal failed counter. Workers (U4) call
// this after a terminal outbound failure (retries exhausted).
func (s *Stats) RecordFailed(sig Signal, n int) {
	if sig < 0 || sig >= signalCount || n <= 0 {
		return
	}
	s.failed[sig].Add(int64(n))
}

// Forwarded returns the current forwarded count for a signal (lock-free read).
func (s *Stats) Forwarded(sig Signal) int64 {
	if sig < 0 || sig >= signalCount {
		return 0
	}
	return s.forwarded[sig].Load()
}

// Failed returns the current failed count for a signal (lock-free read).
func (s *Stats) Failed(sig Signal) int64 {
	if sig < 0 || sig >= signalCount {
		return 0
	}
	return s.failed[sig].Load()
}

// Snapshot is a moment-in-time view of the counters. Captured atomically per
// signal but not transactionally across signals — a snapshot taken during a
// burst may see logs from after a span counter increment but spans from
// before. For visibility purposes this is fine.
type Snapshot struct {
	Forwarded [signalCount]int64
	Failed    [signalCount]int64
	Timestamp time.Time
}

// SnapshotWithRate annotates a Snapshot with per-signal forwarded rate
// (records per second) computed against the previous sample in the sampler's
// rolling window.
type SnapshotWithRate struct {
	Snapshot
	// Rate[i] is records-per-second for signal i, derived from the delta
	// against the previous snapshot. Rate is 0 on the first sample (no prior
	// reference) or when the time delta is zero.
	Rate [signalCount]float64
}

// snapshot captures the current counter values for every signal.
func (s *Stats) snapshot() Snapshot {
	var snap Snapshot
	for i := 0; i < signalCount; i++ {
		snap.Forwarded[i] = s.forwarded[i].Load()
		snap.Failed[i] = s.failed[i].Load()
	}
	snap.Timestamp = time.Now()
	return snap
}

// RateSampler tracks a rolling window of snapshots so consumers can render a
// sparkline timeline of forwarded rate per signal. The sampler owns the
// per-interval ticking and fans snapshots out to subscribed sink channels.
type RateSampler struct {
	stats    *Stats
	interval time.Duration
	capacity int

	mu     sync.Mutex
	window []SnapshotWithRate
}

// NewRateSampler constructs a sampler. capacity is the size of the rolling
// window the sparkline renderer (U7) reads from — at the proxy's hardcoded
// 1s tick interval, capacity=30 gives a 30-second history.
func NewRateSampler(stats *Stats, interval time.Duration, capacity int) *RateSampler {
	if capacity < 1 {
		capacity = 1
	}
	return &RateSampler{
		stats:    stats,
		interval: interval,
		capacity: capacity,
		window:   make([]SnapshotWithRate, 0, capacity),
	}
}

// Sample takes a fresh snapshot, computes per-signal rate against the
// previous sample in the window, appends to the window with ring-buffer
// semantics (oldest drops at capacity), and returns the result.
func (rs *RateSampler) Sample() SnapshotWithRate {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	cur := SnapshotWithRate{Snapshot: rs.stats.snapshot()}

	if len(rs.window) > 0 {
		prev := rs.window[len(rs.window)-1].Snapshot
		dt := cur.Timestamp.Sub(prev.Timestamp).Seconds()
		if dt > 0 {
			for i := 0; i < signalCount; i++ {
				delta := cur.Forwarded[i] - prev.Forwarded[i]
				cur.Rate[i] = float64(delta) / dt
			}
		}
	}

	if len(rs.window) >= rs.capacity {
		copy(rs.window, rs.window[1:])
		rs.window = rs.window[:rs.capacity-1]
	}
	rs.window = append(rs.window, cur)
	return cur
}

// History returns the current rolling window in chronological order (oldest
// first). The returned slice is a defensive copy so callers can iterate
// without holding the sampler's lock.
func (rs *RateSampler) History() []SnapshotWithRate {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	out := make([]SnapshotWithRate, len(rs.window))
	copy(out, rs.window)
	return out
}

// Run loops on a ticker at the sampler's interval, calls Sample on each tick,
// and fans the result out to every sink channel. Returns when ctx is done or
// when ctx is already done at entry. Non-blocking sends to sinks prevent a
// slow consumer from stalling the stats pipeline — if a sink's channel is
// full at tick time, that sink misses one sample rather than the whole
// pipeline backing up.
func (rs *RateSampler) Run(ctx context.Context, sinks ...chan<- SnapshotWithRate) {
	ticker := time.NewTicker(rs.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snap := rs.Sample()
			for _, sink := range sinks {
				select {
				case sink <- snap:
				default:
					// Sink is full; drop this sample for that sink. The next
					// tick will overwrite it for sparkline purposes anyway.
				}
			}
		}
	}
}
