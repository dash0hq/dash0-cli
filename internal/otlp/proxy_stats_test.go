package otlp

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSignal_String(t *testing.T) {
	cases := []struct {
		sig  Signal
		want string
	}{
		{SignalLogs, "logs"},
		{SignalSpans, "spans"},
		{SignalMetrics, "metrics"},
		{Signal(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.sig.String(); got != tc.want {
			t.Errorf("Signal(%d).String() = %q; want %q", tc.sig, got, tc.want)
		}
	}
}

func TestStats_RecordForwarded(t *testing.T) {
	s := &Stats{}
	s.RecordForwarded(SignalLogs, 3)
	s.RecordForwarded(SignalLogs, 2)
	s.RecordForwarded(SignalSpans, 7)
	if got := s.Forwarded(SignalLogs); got != 5 {
		t.Errorf("Forwarded(logs) = %d; want 5", got)
	}
	if got := s.Forwarded(SignalSpans); got != 7 {
		t.Errorf("Forwarded(spans) = %d; want 7", got)
	}
	if got := s.Forwarded(SignalMetrics); got != 0 {
		t.Errorf("Forwarded(metrics) = %d; want 0 (no records)", got)
	}
}

func TestStats_RecordFailed(t *testing.T) {
	s := &Stats{}
	s.RecordFailed(SignalLogs, 4)
	s.RecordFailed(SignalMetrics, 1)
	if got := s.Failed(SignalLogs); got != 4 {
		t.Errorf("Failed(logs) = %d; want 4", got)
	}
	if got := s.Failed(SignalSpans); got != 0 {
		t.Errorf("Failed(spans) = %d; want 0", got)
	}
	if got := s.Failed(SignalMetrics); got != 1 {
		t.Errorf("Failed(metrics) = %d; want 1", got)
	}
}

func TestStats_NegativeAndZeroIgnored(t *testing.T) {
	s := &Stats{}
	s.RecordForwarded(SignalLogs, 0)
	s.RecordForwarded(SignalLogs, -5)
	if got := s.Forwarded(SignalLogs); got != 0 {
		t.Errorf("non-positive count should be ignored; got %d", got)
	}
}

func TestStats_OutOfRangeSignalIgnored(t *testing.T) {
	s := &Stats{}
	s.RecordForwarded(Signal(-1), 5)
	s.RecordForwarded(Signal(99), 5)
	if got := s.Forwarded(SignalLogs); got != 0 {
		t.Errorf("out-of-range signal should not affect logs; got %d", got)
	}
	if got := s.Forwarded(Signal(99)); got != 0 {
		t.Errorf("Forwarded(out-of-range) should return 0; got %d", got)
	}
}

func TestStats_ConcurrentRecord(t *testing.T) {
	// Verifies atomic-add semantics under contention. Run under -race to
	// catch a regression to a non-atomic field type.
	s := &Stats{}
	const n = 1000
	const goroutines = 8
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < n; i++ {
				s.RecordForwarded(SignalLogs, 1)
				s.RecordFailed(SignalSpans, 1)
			}
		}()
	}
	wg.Wait()
	if got := s.Forwarded(SignalLogs); got != int64(n*goroutines) {
		t.Errorf("Forwarded(logs) under contention = %d; want %d", got, n*goroutines)
	}
	if got := s.Failed(SignalSpans); got != int64(n*goroutines) {
		t.Errorf("Failed(spans) under contention = %d; want %d", got, n*goroutines)
	}
}

func TestRateSampler_FirstSampleRateIsZero(t *testing.T) {
	s := &Stats{}
	s.RecordForwarded(SignalLogs, 100)

	rs := NewRateSampler(s, time.Second, 30)
	got := rs.Sample()

	if got.Forwarded[SignalLogs] != 100 {
		t.Errorf("Forwarded[logs] = %d; want 100", got.Forwarded[SignalLogs])
	}
	if got.Rate[SignalLogs] != 0 {
		t.Errorf("first-sample rate should be 0 (no prior reference); got %f", got.Rate[SignalLogs])
	}
}

func TestRateSampler_RateComputedAgainstPrior(t *testing.T) {
	s := &Stats{}
	rs := NewRateSampler(s, time.Second, 30)

	s.RecordForwarded(SignalLogs, 10)
	first := rs.Sample()

	// Sleep long enough that the time delta is non-zero. Use a small interval
	// so the test stays fast.
	time.Sleep(50 * time.Millisecond)
	s.RecordForwarded(SignalLogs, 20)
	second := rs.Sample()

	if second.Forwarded[SignalLogs] != 30 {
		t.Errorf("Forwarded[logs] cumulative = %d; want 30", second.Forwarded[SignalLogs])
	}
	if second.Rate[SignalLogs] <= 0 {
		t.Errorf("rate should be positive after 20 records arrived; got %f", second.Rate[SignalLogs])
	}
	// Sanity check the first sample is still rate-zero.
	if first.Rate[SignalLogs] != 0 {
		t.Errorf("first sample rate should remain 0; got %f", first.Rate[SignalLogs])
	}
}

func TestRateSampler_RingBufferEvictsOldest(t *testing.T) {
	s := &Stats{}
	const capacity = 3
	rs := NewRateSampler(s, time.Millisecond, capacity)

	for i := 0; i < 5; i++ {
		s.RecordForwarded(SignalLogs, 1)
		time.Sleep(2 * time.Millisecond)
		rs.Sample()
	}

	history := rs.History()
	if len(history) != capacity {
		t.Errorf("history length = %d; want %d (ring buffer at capacity)", len(history), capacity)
	}
	// The forwarded totals should be the last three sample values: 3, 4, 5.
	for i, want := range []int64{3, 4, 5} {
		if history[i].Forwarded[SignalLogs] != want {
			t.Errorf("history[%d].Forwarded[logs] = %d; want %d", i, history[i].Forwarded[SignalLogs], want)
		}
	}
}

func TestRateSampler_HistoryIsDefensiveCopy(t *testing.T) {
	s := &Stats{}
	rs := NewRateSampler(s, time.Second, 30)
	s.RecordForwarded(SignalLogs, 1)
	rs.Sample()

	h1 := rs.History()
	if len(h1) != 1 {
		t.Fatalf("history length = %d; want 1", len(h1))
	}
	// Mutate the returned slice; subsequent History() must not reflect the
	// mutation.
	h1[0].Forwarded[SignalLogs] = 999

	h2 := rs.History()
	if h2[0].Forwarded[SignalLogs] != 1 {
		t.Errorf("History should return defensive copies; mutation leaked (got %d, want 1)", h2[0].Forwarded[SignalLogs])
	}
}

func TestRateSampler_RunEmitsOnTick(t *testing.T) {
	s := &Stats{}
	rs := NewRateSampler(s, 10*time.Millisecond, 5)
	sink := make(chan SnapshotWithRate, 4)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go rs.Run(ctx, sink)

	// Wait for at least two ticks.
	deadline := time.After(200 * time.Millisecond)
	var received []SnapshotWithRate
collect:
	for {
		select {
		case s := <-sink:
			received = append(received, s)
			if len(received) >= 2 {
				break collect
			}
		case <-deadline:
			break collect
		}
	}
	if len(received) < 2 {
		t.Errorf("expected at least 2 snapshots; got %d", len(received))
	}
}

func TestRateSampler_RunExitsOnContextCancel(t *testing.T) {
	s := &Stats{}
	rs := NewRateSampler(s, 10*time.Millisecond, 5)
	sink := make(chan SnapshotWithRate, 4)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		rs.Run(ctx, sink)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// Run returned promptly after cancel; expected.
	case <-time.After(200 * time.Millisecond):
		t.Error("Run did not exit within 200ms of context cancel")
	}
}

func TestRateSampler_RunFansOutToMultipleSinks(t *testing.T) {
	s := &Stats{}
	rs := NewRateSampler(s, 10*time.Millisecond, 5)
	sinkA := make(chan SnapshotWithRate, 4)
	sinkB := make(chan SnapshotWithRate, 4)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go rs.Run(ctx, sinkA, sinkB)

	deadline := time.After(200 * time.Millisecond)
	var gotA, gotB bool
	for !(gotA && gotB) {
		select {
		case <-sinkA:
			gotA = true
		case <-sinkB:
			gotB = true
		case <-deadline:
			t.Fatalf("did not receive from both sinks within deadline; gotA=%t gotB=%t", gotA, gotB)
		}
	}
}

func TestRateSampler_SlowSinkDoesNotBlockTicker(t *testing.T) {
	// Buffered cap 1, never drained: subsequent ticks must drop the sample
	// for that sink, not stall the ticker (which would in turn stall the
	// counters being snapshotted into history).
	s := &Stats{}
	rs := NewRateSampler(s, 10*time.Millisecond, 5)
	stuckSink := make(chan SnapshotWithRate, 1)
	healthySink := make(chan SnapshotWithRate, 8)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go rs.Run(ctx, stuckSink, healthySink)

	deadline := time.After(200 * time.Millisecond)
	var healthyCount int
collect:
	for {
		select {
		case <-healthySink:
			healthyCount++
			if healthyCount >= 3 {
				break collect
			}
		case <-deadline:
			break collect
		}
	}
	if healthyCount < 3 {
		t.Errorf("stuck sink should not stall the ticker; healthy sink received %d snapshots, want >= 3", healthyCount)
	}
}

func TestRateSampler_CapacityFloorAtOne(t *testing.T) {
	// Defensive: bogus capacity (<1) should not crash on Sample.
	s := &Stats{}
	rs := NewRateSampler(s, time.Second, 0)
	rs.Sample()
	rs.Sample()
	history := rs.History()
	if len(history) != 1 {
		t.Errorf("capacity floored to 1; history length = %d; want 1", len(history))
	}
}
