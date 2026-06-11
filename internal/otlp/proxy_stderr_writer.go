package otlp

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

// stderrChannelDepth controls how many stats updates and lifecycle events
// can queue ahead of the writer goroutine. 8 is plenty at the 1-second
// stats cadence — an overflowing channel signals a stuck writer (most
// likely the terminal locked up), in which case dropping is preferable
// to stalling the proxy.
const stderrChannelDepth = 8

// sparklineHistoryCapacity is the number of recent rate samples retained
// per signal for the TTY sparkline. At the 1-second stats cadence this
// equals roughly thirty seconds of timeline.
const sparklineHistoryCapacity = 30

// sparklineMinWidth is the minimum terminal column count below which
// sparklines are suppressed and the stats line falls back to text-only
// counts. The threshold trades visual density against readability — under
// 100 columns the per-signal sparkline+rate+total triples overlap.
const sparklineMinWidth = 100

// defaultLineWidth is the fallback terminal width assumed when stderr is
// not a TTY or term.GetSize returns an error.
const defaultLineWidth = 80

// LifecycleEvent is the typed channel input for one-shot stderr messages
// that should print above the live stats line. The writer renders Message
// verbatim followed by a newline; Kind is reserved for future color or
// prefix customization and currently informational only.
type LifecycleEvent struct {
	Kind    LifecycleKind
	Message string
}

// LifecycleKind classifies lifecycle messages. v1 doesn't differentiate
// visually; the kind is recorded so a future polish pass can color or
// prefix the rendering without changing the channel signature.
type LifecycleKind int

const (
	LifecycleInfo LifecycleKind = iota
	LifecycleWarning
	LifecycleError
	LifecycleBanner
)

// StderrWriter owns os.Stderr in TTY mode and interleaves the live stats
// line with one-shot lifecycle messages.
//
// The writer maintains its own rolling per-signal rate history independent
// of the RateSampler — the stats channel only carries the current
// snapshot, not the full history, so the writer accumulates samples as
// they arrive. This decouples the writer from the sampler beyond the
// channel.
type StderrWriter struct {
	out io.Writer

	// fd is the OS file descriptor used for TTY detection and width
	// queries. In production it points at os.Stderr.Fd(); tests pass a
	// non-TTY fd to drive the text-only fallback path.
	fd int
}

// NewStderrWriter returns a writer plus the two channels callers push to.
// Stats updates feed the live line; lifecycle events render above it.
func NewStderrWriter(out io.Writer, fd int) (*StderrWriter, chan SnapshotWithRate, chan LifecycleEvent) {
	return &StderrWriter{
			out: out,
			fd:  fd,
		},
		make(chan SnapshotWithRate, stderrChannelDepth),
		make(chan LifecycleEvent, stderrChannelDepth)
}

// Run drains the stats and lifecycle channels until ctx is done.
//
// In TTY mode the writer:
//   - Tracks the rendered stats line so it can erase via \r-and-pad before
//     a lifecycle message or before redrawing.
//   - Maintains a per-signal rate ring buffer of capacity
//     sparklineHistoryCapacity for the sparkline.
//   - Re-snapshots terminal width on every render so a SIGWINCH-triggered
//     resize is picked up at the next stats tick (no separate resize
//     channel needed; see proxy_stderr_writer_unix.go for the optional
//     immediate-refresh path).
//
// In non-TTY mode the writer emits the start banner and other lifecycle
// events as plain lines but skips the redrawing stats display — piping
// stderr to a file or another process means in-place redraw would produce
// `\r`-littered output rather than a useful log.
func (w *StderrWriter) Run(ctx context.Context, statsCh <-chan SnapshotWithRate, lifecycleCh <-chan LifecycleEvent) {
	isTTY := term.IsTerminal(w.fd)

	var history [signalCount][]float64
	for i := range history {
		history[i] = make([]float64, 0, sparklineHistoryCapacity)
	}
	var lastSnapshot SnapshotWithRate
	var lastSnapshotSet bool
	var statsLineLen int

	erase := func() {
		if !isTTY || statsLineLen == 0 {
			return
		}
		// Carriage-return to the start, write spaces to overwrite the
		// previous line content, carriage-return again so the next write
		// starts at column zero.
		fmt.Fprintf(w.out, "\r%s\r", strings.Repeat(" ", statsLineLen))
		statsLineLen = 0
	}

	render := func() {
		if !isTTY || !lastSnapshotSet {
			return
		}
		width := terminalWidth(w.fd)
		line := formatStatsLine(history[:], lastSnapshot, width)
		// Erase any prior content on this line, then write the new line
		// without trailing newline so subsequent ticks overwrite in place.
		fmt.Fprintf(w.out, "\r%s", line)
		statsLineLen = utf8.RuneCountInString(line)
	}

	for {
		select {
		case <-ctx.Done():
			erase()
			return
		case snap := <-statsCh:
			recordRate(history[:], snap.Rate)
			lastSnapshot = snap
			lastSnapshotSet = true
			render()
		case ev := <-lifecycleCh:
			erase()
			fmt.Fprintf(w.out, "%s\n", ev.Message)
			render()
		}
	}
}

// recordRate appends each signal's current rate to its history ring with
// ring-buffer eviction at sparklineHistoryCapacity.
func recordRate(history [][]float64, rates [signalCount]float64) {
	for i := range history {
		if len(history[i]) >= sparklineHistoryCapacity {
			copy(history[i], history[i][1:])
			history[i] = history[i][:len(history[i])-1]
		}
		history[i] = append(history[i], rates[i])
	}
}

// formatStatsLine renders the per-signal stats. Includes sparklines when
// the terminal is wide enough; otherwise falls back to text-only counts.
func formatStatsLine(history [][]float64, snap SnapshotWithRate, width int) string {
	showSparkline := width >= sparklineMinWidth
	labels := [signalCount]string{"logs", "spans", "metrics"}

	parts := make([]string, 0, signalCount)
	for i, label := range labels {
		var prefix string
		if showSparkline && i < len(history) && len(history[i]) > 0 {
			prefix = Sparkline(history[i]) + " "
		}
		parts = append(parts, fmt.Sprintf("%s %s%.0f/s · %d total",
			label, prefix, snap.Rate[i], snap.Forwarded[i]))
	}
	return strings.Join(parts, "   ")
}

// terminalWidth returns the current terminal column count for fd, or
// defaultLineWidth when not a TTY or the size lookup fails.
func terminalWidth(fd int) int {
	if !term.IsTerminal(fd) {
		return defaultLineWidth
	}
	cols, _, err := term.GetSize(fd)
	if err != nil || cols <= 0 {
		return defaultLineWidth
	}
	return cols
}
