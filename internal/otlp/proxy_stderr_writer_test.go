package otlp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
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

// signalLines filters out the blank separator rows from a formatStatsBlock
// result so individual signal rows can be checked by index 0..statsBlockLines-1.
func signalLines(lines []string) []string {
	out := make([]string, 0, signalCount)
	for _, l := range lines {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func TestFormatStatsBlock_OneLinePerSignal(t *testing.T) {
	snap := SnapshotWithRate{
		Snapshot: Snapshot{Forwarded: [signalCount]int64{100, 200, 300}},
		Rate:     [signalCount]float64{5, 10, 0},
	}
	history := [][]float64{{1, 2, 5}, {3, 7, 10}, {0, 0, 0}}
	lines := formatStatsBlock(history, snap, 5)
	if len(lines) != statsBlockLines {
		t.Fatalf("got %d lines; want %d (signal rows + blank separators)", len(lines), statsBlockLines)
	}
	signals := signalLines(lines)
	if len(signals) != signalCount {
		t.Fatalf("got %d non-blank lines; want %d", len(signals), signalCount)
	}
	// Each signal row must carry its own label, rate, and total.
	wantPrefixes := []string{"logs:", "spans:", "metrics:"}
	wantSuffixes := []string{"100 total", "200 total", "300 total"}
	wantRates := []string{"5/s", "10/s", "0/s"}
	for i, line := range signals {
		if !strings.HasPrefix(strings.TrimSpace(line), wantPrefixes[i]) {
			t.Errorf("signal row %d (%q) should start with %q", i, line, wantPrefixes[i])
		}
		if !strings.Contains(line, wantSuffixes[i]) {
			t.Errorf("signal row %d (%q) missing %q", i, line, wantSuffixes[i])
		}
		if !strings.Contains(line, wantRates[i]) {
			t.Errorf("signal row %d (%q) missing rate %q", i, line, wantRates[i])
		}
	}
}

func TestFormatStatsBlock_BlankSeparatorsBetweenSignals(t *testing.T) {
	// The output has signalCount rows interleaved with (statsBlockLines-1)
	// blank rows — visual breathing room between signals.
	snap := SnapshotWithRate{
		Snapshot: Snapshot{Forwarded: [signalCount]int64{1, 2, 3}},
		Rate:     [signalCount]float64{4, 5, 6},
	}
	lines := formatStatsBlock(nil, snap, 5)
	for i, line := range lines {
		// Odd indices (1, 3, ...) should be blank; even indices have content.
		if i%2 == 0 {
			if line == "" {
				t.Errorf("line %d expected to be a signal row; got blank", i)
			}
		} else {
			if line != "" {
				t.Errorf("line %d expected to be a blank separator; got %q", i, line)
			}
		}
	}
}

func TestFormatStatsBlock_LabelsAreColumnAligned(t *testing.T) {
	// Labels share the same prefix width — the colon after each label
	// sits at the same column. Walk only the signal rows so the blank
	// separators don't trip the colon search.
	snap := SnapshotWithRate{
		Snapshot: Snapshot{Forwarded: [signalCount]int64{1, 2, 3}},
		Rate:     [signalCount]float64{4, 5, 6},
	}
	signals := signalLines(formatStatsBlock(nil, snap, 5))

	colonAt := -1
	for i, line := range signals {
		idx := strings.Index(line, ":")
		if idx < 0 {
			t.Fatalf("signal row %d missing colon: %q", i, line)
		}
		if colonAt == -1 {
			colonAt = idx
			continue
		}
		if idx != colonAt {
			t.Errorf("signal row %d colon at column %d; want %d (labels not aligned)\nrow 0: %q\nthis : %q",
				i, idx, colonAt, signals[0], line)
		}
	}
}

func TestFormatStatsBlock_SparklineHonoursRequestedWidth(t *testing.T) {
	// Pass a sparkline width and assert each signal row carries at most
	// that many glyphs, regardless of how much history was supplied.
	long := make([]float64, sparklineHistoryCapacity)
	for i := range long {
		long[i] = float64(i + 1)
	}
	history := [][]float64{long, long, long}
	snap := SnapshotWithRate{
		Snapshot: Snapshot{Forwarded: [signalCount]int64{1, 2, 3}},
		Rate:     [signalCount]float64{4, 5, 6},
	}
	const requestedWidth = 7
	signals := signalLines(formatStatsBlock(history, snap, requestedWidth))
	for i, line := range signals {
		glyphs := 0
		for _, r := range line {
			for _, g := range "▁▂▃▄▅▆▇█" {
				if r == g {
					glyphs++
					break
				}
			}
		}
		if glyphs > requestedWidth {
			t.Errorf("signal row %d has %d sparkline glyphs; want at most %d",
				i, glyphs, requestedWidth)
		}
	}
}

func TestFormatStatsBlock_TotalsAreRightAligned(t *testing.T) {
	// Totals across signal rows must right-align: the rightmost digit
	// of each total sits at the same column, and `<digits> total` lines
	// up. Skip blank separator rows when looking for the suffix.
	snap := SnapshotWithRate{
		Snapshot: Snapshot{Forwarded: [signalCount]int64{1234, 540, 0}},
		Rate:     [signalCount]float64{42, 18, 0},
	}
	signals := signalLines(formatStatsBlock(nil, snap, 5))

	totalIdx := -1
	for i, line := range signals {
		idx := strings.Index(line, " total")
		if idx < 0 {
			t.Fatalf("signal row %d missing ' total': %q", i, line)
		}
		if totalIdx == -1 {
			totalIdx = idx
			continue
		}
		if idx != totalIdx {
			t.Errorf("signal row %d ' total' at column %d; want %d (totals not right-aligned)\nrow 0: %q\nthis : %q",
				i, idx, totalIdx, signals[0], line)
		}
	}

	// Spot-check the largest number sits flush with the ' total' anchor:
	// the four-digit "1234" should appear immediately before " total".
	if !strings.Contains(signals[0], "1234 total") {
		t.Errorf("logs row should contain '1234 total' with no padding before the number; got %q", signals[0])
	}
	// And smaller numbers carry leading spaces so they still end at the
	// same column.
	if !strings.Contains(signals[2], "   0 total") {
		t.Errorf("metrics row should pad '0' with three leading spaces (3 = 4 - 1); got %q", signals[2])
	}
}

func TestFormatStatsBlock_EmptyHistoryPreservesAlignment(t *testing.T) {
	// With no history, the sparkline must render as `width` spaces so
	// the `<total> total` column still lines up across signal rows.
	snap := SnapshotWithRate{
		Snapshot: Snapshot{Forwarded: [signalCount]int64{0, 0, 0}},
		Rate:     [signalCount]float64{0, 0, 0},
	}
	signals := signalLines(formatStatsBlock(nil, snap, 5))
	// All signal rows should be the same length (assuming totals format
	// the same width). With Forwarded=0 across all signals this holds.
	wantLen := utf8.RuneCountInString(signals[0])
	for i, line := range signals {
		if utf8.RuneCountInString(line) != wantLen {
			t.Errorf("signal row %d length = %d; want %d (alignment broken)\nrow 0: %q\nthis : %q",
				i, utf8.RuneCountInString(line), wantLen, signals[0], line)
		}
	}
}

func TestCurrentSparklineWidth_NonTTYReturnsDefault(t *testing.T) {
	// Non-TTY fd → fallback to sparklineDefaultWidth regardless of any
	// terminalSize override (the function returns before calling it).
	prev := isTerminal
	isTerminal = func(int) bool { return false }
	t.Cleanup(func() { isTerminal = prev })

	if got := currentSparklineWidth(0); got != sparklineDefaultWidth {
		t.Errorf("currentSparklineWidth on non-TTY = %d; want %d", got, sparklineDefaultWidth)
	}
}

func TestCurrentSparklineWidth_ScalesWithTerminalWidth(t *testing.T) {
	// At 200 cols there's plenty of room — the result should saturate at
	// sparklineHistoryCapacity. At 60 cols the result should be smaller.
	// At a tiny 30 cols the result should clamp to sparklineMinWidth.
	prev := isTerminal
	isTerminal = func(int) bool { return true }
	t.Cleanup(func() { isTerminal = prev })

	prevSize := terminalSize
	t.Cleanup(func() { terminalSize = prevSize })

	cases := []struct {
		cols int
		want int
	}{
		{cols: 200, want: sparklineHistoryCapacity},
		{cols: 60, want: 60 - statsBlockFixedOverhead - 10 - statsLineSafetyMargin},
		{cols: 30, want: sparklineMinWidth}, // clamp floor
	}
	for _, c := range cases {
		terminalSize = func(int) (int, int, error) { return c.cols, 24, nil }
		got := currentSparklineWidth(1)
		if got != c.want {
			t.Errorf("currentSparklineWidth at %d cols = %d; want %d", c.cols, got, c.want)
		}
	}
}

func TestCurrentSparklineWidth_SizeErrorFallsBackToDefault(t *testing.T) {
	prev := isTerminal
	isTerminal = func(int) bool { return true }
	t.Cleanup(func() { isTerminal = prev })

	prevSize := terminalSize
	t.Cleanup(func() { terminalSize = prevSize })
	terminalSize = func(int) (int, int, error) { return 0, 0, fmt.Errorf("size unknown") }

	if got := currentSparklineWidth(1); got != sparklineDefaultWidth {
		t.Errorf("currentSparklineWidth with size error = %d; want %d", got, sparklineDefaultWidth)
	}
}

func TestRenderPaddedSparkline_EmptyReturnsSpaces(t *testing.T) {
	got := renderPaddedSparkline(nil, 5)
	if got != "     " {
		t.Errorf("empty history → %q; want 5 spaces", got)
	}
}

func TestRenderPaddedSparkline_ShortLeftPads(t *testing.T) {
	// Two samples in a 5-wide slot → 3 leading spaces + 2 glyphs.
	got := renderPaddedSparkline([]float64{1, 2}, 5)
	if utf8.RuneCountInString(got) != 5 {
		t.Errorf("rune count = %d; want 5", utf8.RuneCountInString(got))
	}
	if !strings.HasPrefix(got, "   ") {
		t.Errorf("short history should be left-padded; got %q", got)
	}
}

func TestRenderPaddedSparkline_LongTailSlices(t *testing.T) {
	// Ten samples in a 5-wide slot → the most recent 5 only.
	got := renderPaddedSparkline([]float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 5)
	if utf8.RuneCountInString(got) != 5 {
		t.Errorf("rune count = %d; want 5", utf8.RuneCountInString(got))
	}
	// The rightmost glyph should be the highest of the last 5 samples
	// (value 10 → top of the 8-level ramp → full block).
	runes := []rune(got)
	if runes[len(runes)-1] != '█' {
		t.Errorf("last glyph = %q; want '█' (highest of last 5 samples)", string(runes[len(runes)-1]))
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

// withFakeTTY substitutes the isTerminal hook so the writer's TTY
// redraw path runs in tests. Restores the original on cleanup.
func withFakeTTY(t *testing.T) {
	t.Helper()
	prev := isTerminal
	isTerminal = func(int) bool { return true }
	t.Cleanup(func() { isTerminal = prev })
}

func TestStderrWriter_TTY_RedrawErasesBetweenTicks(t *testing.T) {
	// Regression for the multi-line redraw bug: two stats ticks should
	// produce exactly two stats blocks in the output, with an ANSI
	// cursor-up + clear-to-end-of-screen sequence between them. Before
	// the fix the second block was appended to the end of the first
	// block's last line and showed up as one long mangled line.
	withFakeTTY(t)

	buf := &safeBuffer{}
	w, statsCh, _ := NewStderrWriter(buf, 1) // any non-negative fd; isTerminal is faked
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx, statsCh, nil)
		close(done)
	}()

	first := SnapshotWithRate{
		Snapshot: Snapshot{Forwarded: [signalCount]int64{1, 2, 3}},
		Rate:     [signalCount]float64{4, 5, 6},
	}
	second := SnapshotWithRate{
		Snapshot: Snapshot{Forwarded: [signalCount]int64{10, 20, 30}},
		Rate:     [signalCount]float64{40, 50, 60},
	}
	statsCh <- first
	statsCh <- second

	// Wait for two blocks to land. Each block is signalCount lines, but
	// since render writes lines joined by '\n' the buffer should contain
	// 2 × (statsBlockLines-1) newlines plus the erase sequence.
	deadline := time.After(500 * time.Millisecond)
	for {
		if strings.Count(buf.String(), "metrics:") >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("did not see two stats blocks within 500ms; buf:\n%s", buf.String())
		case <-time.After(5 * time.Millisecond):
		}
	}

	cancel()
	<-done

	got := buf.String()

	// The erase sequence is `\r\x1b[<N>A\x1b[J` where N = statsBlockLines-1.
	wantErase := fmt.Sprintf("\r\x1b[%dA\x1b[J", statsBlockLines-1)
	if !strings.Contains(got, wantErase) {
		t.Errorf("expected the cursor-up + clear-screen sequence between ticks; not found in output:\n%q", got)
	}

	// And the second block should not be appended to the first block's
	// final line — the substring "metrics:" should appear at column 0
	// of its line at least twice (once per block). Verify by checking
	// that no line contains "metrics:" followed by " logs:" without a
	// newline between them.
	if strings.Contains(got, "metrics:     6/s ▁▁▁  3 total   logs:") {
		t.Errorf("second block appended to first block's last line:\n%s", got)
	}
}

func TestStderrWriter_TTY_RedrawErasesOnFinalShutdown(t *testing.T) {
	// On context cancel the writer should erase the block so the shell
	// prompt appears at the top of where the block was, not at the end
	// of metrics:'s line.
	withFakeTTY(t)
	buf := &safeBuffer{}
	w, statsCh, _ := NewStderrWriter(buf, 1)
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

	// Wait for the first block.
	deadline := time.After(500 * time.Millisecond)
	for {
		if strings.Contains(buf.String(), "metrics:") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("first block did not render")
		case <-time.After(5 * time.Millisecond):
		}
	}

	cancel()
	<-done

	// After cancel, the erase sequence should appear at least twice:
	// once before the (no-op) second render that doesn't fire, and at
	// minimum the ctx.Done erase. Easier assertion: the final erase is
	// present, count >= 1.
	got := buf.String()
	wantErase := fmt.Sprintf("\r\x1b[%dA\x1b[J", statsBlockLines-1)
	if strings.Count(got, wantErase) < 1 {
		t.Errorf("expected at least one erase sequence after shutdown; got:\n%q", got)
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
