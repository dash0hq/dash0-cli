package otlp

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// nonTTYFd is a fake file descriptor that will fail TTY detection. The
// writer's non-TTY fallback path renders lifecycle events as plain lines
// and suppresses the in-place stats redraw.
const nonTTYFd = -1

func TestStderrWriter_NonTTY_LifecycleAsPlainLines(t *testing.T) {
	buf := &safeBuffer{}
	w, _, lifecycleCh := NewStderrWriter(buf, nonTTYFd)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx, nil, lifecycleCh)
		close(done)
	}()

	lifecycleCh <- LifecycleEvent{Kind: LifecycleBanner, Message: "OTLP/HTTP listening on http://127.0.0.1:4318"}
	lifecycleCh <- LifecycleEvent{Kind: LifecycleInfo, Message: "OTLP/gRPC listening on 127.0.0.1:4317"}

	// Give the writer time to drain both messages.
	deadline := time.After(500 * time.Millisecond)
	for {
		if strings.Count(buf.String(), "\n") >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("writer did not emit both lifecycle messages in time; got:\n%s", buf.String())
		case <-time.After(5 * time.Millisecond):
		}
	}

	cancel()
	<-done

	out := buf.String()
	if !strings.Contains(out, "OTLP/HTTP listening on http://127.0.0.1:4318\n") {
		t.Errorf("missing HTTP banner; got:\n%s", out)
	}
	if !strings.Contains(out, "OTLP/gRPC listening on 127.0.0.1:4317\n") {
		t.Errorf("missing gRPC banner; got:\n%s", out)
	}
	// No carriage-returns in non-TTY mode — those are only for in-place
	// stats redrawing.
	if strings.Contains(out, "\r") {
		t.Errorf("non-TTY output should not contain carriage returns; got:\n%q", out)
	}
}

func TestStderrWriter_NonTTY_StatsSuppressed(t *testing.T) {
	// In non-TTY mode the stats channel should drain without producing any
	// output — the in-place redraw is meaningless when stderr is piped
	// somewhere that won't interpret \r as cursor movement.
	buf := &safeBuffer{}
	w, statsCh, _ := NewStderrWriter(buf, nonTTYFd)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx, statsCh, nil)
		close(done)
	}()

	statsCh <- SnapshotWithRate{
		Snapshot: Snapshot{Forwarded: [signalCount]int64{1, 2, 3}},
		Rate:     [signalCount]float64{4, 5, 6},
	}

	// Give the writer time to "process" the snapshot. It should produce
	// nothing — we sleep a brief interval then assert no output.
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if got := buf.String(); got != "" {
		t.Errorf("non-TTY stats should produce no output; got %q", got)
	}
}

func TestStderrWriter_ExitsOnContextCancel(t *testing.T) {
	buf := &safeBuffer{}
	w, statsCh, lifecycleCh := NewStderrWriter(buf, nonTTYFd)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx, statsCh, lifecycleCh)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Run did not exit within 200ms of context cancel")
	}
}

func TestFormatStatsLine_AllSignalsLabeled(t *testing.T) {
	snap := SnapshotWithRate{
		Snapshot: Snapshot{Forwarded: [signalCount]int64{100, 200, 300}},
		Rate:     [signalCount]float64{5, 10, 0},
	}
	history := [][]float64{{1, 2, 5}, {3, 7, 10}, {0, 0, 0}}
	// Wide terminal — sparklines should appear.
	got := formatStatsLine(history, snap, 200)
	for _, want := range []string{"logs", "spans", "metrics", "5/s", "10/s", "0/s", "100 total", "200 total", "300 total"} {
		if !strings.Contains(got, want) {
			t.Errorf("stats line missing %q; got:\n%s", want, got)
		}
	}
}

func TestFormatStatsLine_NarrowTerminalDropsSparkline(t *testing.T) {
	snap := SnapshotWithRate{
		Snapshot: Snapshot{Forwarded: [signalCount]int64{1, 2, 3}},
		Rate:     [signalCount]float64{4, 5, 6},
	}
	history := [][]float64{{1, 2, 3}, {3, 4, 5}, {0, 1, 2}}
	got := formatStatsLine(history, snap, 60) // < sparklineMinWidth
	// None of the 8-level glyphs should appear when sparklines are
	// suppressed. The text rate/total fields should still be present.
	for _, g := range "▁▂▃▄▅▆▇█" {
		if strings.ContainsRune(got, g) {
			t.Errorf("narrow terminal should omit sparkline glyph %c; got:\n%s", g, got)
		}
	}
	for _, want := range []string{"logs", "spans", "metrics", "4/s", "5/s", "6/s"} {
		if !strings.Contains(got, want) {
			t.Errorf("narrow stats line missing %q; got:\n%s", want, got)
		}
	}
}

func TestFormatStatsLine_WideTerminalHasSparkline(t *testing.T) {
	snap := SnapshotWithRate{
		Snapshot: Snapshot{Forwarded: [signalCount]int64{1, 2, 3}},
		Rate:     [signalCount]float64{4, 5, 6},
	}
	history := [][]float64{{1, 2, 3, 4, 5}, {3, 4, 5}, {0, 1, 2}}
	got := formatStatsLine(history, snap, 200)
	// At least one block glyph should appear since some series have non-
	// zero variation.
	var foundGlyph bool
	for _, g := range "▁▂▃▄▅▆▇█" {
		if strings.ContainsRune(got, g) {
			foundGlyph = true
			break
		}
	}
	if !foundGlyph {
		t.Errorf("wide terminal should include sparkline glyphs; got:\n%s", got)
	}
}

func TestRecordRate_RingBufferEvicts(t *testing.T) {
	var history [signalCount][]float64
	for i := range history {
		history[i] = make([]float64, 0, sparklineHistoryCapacity)
	}

	// Push more than capacity values; the oldest should evict.
	for i := 0; i < sparklineHistoryCapacity+5; i++ {
		recordRate(history[:], [signalCount]float64{float64(i), float64(i * 2), float64(i * 3)})
	}

	for sig := 0; sig < signalCount; sig++ {
		if len(history[sig]) != sparklineHistoryCapacity {
			t.Errorf("signal %d history len = %d; want %d (ring buffer at capacity)",
				sig, len(history[sig]), sparklineHistoryCapacity)
		}
		// Newest sample should be the last push.
		want := float64((sparklineHistoryCapacity + 4) * (sig + 1))
		if got := history[sig][len(history[sig])-1]; got != want {
			t.Errorf("signal %d newest sample = %f; want %f", sig, got, want)
		}
	}
}

func TestRecordRate_FillingWindow(t *testing.T) {
	// Below capacity, recordRate should simply append.
	var history [signalCount][]float64
	for i := range history {
		history[i] = make([]float64, 0, sparklineHistoryCapacity)
	}

	for i := 0; i < 5; i++ {
		recordRate(history[:], [signalCount]float64{float64(i + 1), 0, 0})
	}
	if len(history[SignalLogs]) != 5 {
		t.Errorf("history len = %d; want 5", len(history[SignalLogs]))
	}
	for i, want := 0, 1.0; i < 5; i, want = i+1, want+1 {
		if history[SignalLogs][i] != want {
			t.Errorf("history[%d] = %f; want %f", i, history[SignalLogs][i], want)
		}
	}
}

func TestStderrWriter_ConcurrentLifecycleAndStats(t *testing.T) {
	// Stress: emit lots of lifecycle events concurrently with stats
	// updates. The single-writer goroutine must serialize them; no race
	// on the underlying buffer. Run under -race to catch regressions to
	// shared-state usage.
	buf := &safeBuffer{}
	w, statsCh, lifecycleCh := NewStderrWriter(buf, nonTTYFd)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx, statsCh, lifecycleCh)
		close(done)
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			lifecycleCh <- LifecycleEvent{Kind: LifecycleInfo, Message: "hello"}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			statsCh <- SnapshotWithRate{Rate: [signalCount]float64{1, 2, 3}}
		}
	}()
	wg.Wait()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	// At least one lifecycle line should have made it through; stats are
	// suppressed in non-TTY.
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("expected at least one lifecycle line in output; got %q", buf.String())
	}
}
