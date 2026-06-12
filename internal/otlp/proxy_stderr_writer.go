package otlp

import (
	"context"
	"fmt"
	"io"
	"strconv"
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

// sparklineMaxWidth caps the rendered sparkline at this many glyphs. The
// rolling-window history retains more samples (sparklineHistoryCapacity)
// so the proxy can grow the window if a future redesign uses more
// horizontal space, but the per-render width stays bounded.
const sparklineMaxWidth = 5

// isTerminal is the substitutable hook the writer uses to decide whether
// to emit ANSI cursor-control sequences. Production code points it at
// term.IsTerminal; tests override it so the redraw path is exercisable
// without an actual TTY. The variable holds no state — only a function
// reference — so substitution is safe.
var isTerminal = term.IsTerminal

// LifecycleEvent is the typed channel input for one-shot stderr messages
// that should print above the live stats block. The writer renders Message
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
// block with one-shot lifecycle messages.
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
// Stats updates feed the live block; lifecycle events render above it.
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
// In TTY mode the writer renders a multi-line stats block — one line per
// signal — and redraws it in place on each tick using ANSI cursor
// controls. Lifecycle messages are inserted above the block by erasing
// the block first, printing the message, then redrawing.
//
// In non-TTY mode the writer emits lifecycle events as plain lines but
// skips the live stats display — piping stderr to a file or another
// process means in-place redraw would litter the output with control
// sequences rather than produce a useful log.
func (w *StderrWriter) Run(ctx context.Context, statsCh <-chan SnapshotWithRate, lifecycleCh <-chan LifecycleEvent) {
	isTTY := isTerminal(w.fd)

	var history [signalCount][]float64
	for i := range history {
		history[i] = make([]float64, 0, sparklineHistoryCapacity)
	}
	var lastSnapshot SnapshotWithRate
	var lastSnapshotSet bool

	// blockRendered tracks whether a stats block currently occupies the
	// signalCount lines above (and including) the cursor's current line.
	// erase() uses this to decide whether to emit cursor-movement +
	// screen-clear sequences.
	var blockRendered bool

	erase := func() {
		if !isTTY || !blockRendered {
			return
		}
		// Move to column 0, then up (signalCount-1) lines so the cursor
		// sits at the start of the stats block's top line, then clear
		// from cursor to end of screen.
		fmt.Fprintf(w.out, "\r\x1b[%dA\x1b[J", signalCount-1)
		blockRendered = false
	}

	render := func() {
		if !isTTY || !lastSnapshotSet {
			return
		}
		// Always erase any previously-rendered block before redrawing.
		// On the very first render `blockRendered` is false and erase()
		// is a no-op, so the initial draw lands at the natural cursor
		// position. Every subsequent tick clears the prior three rows
		// first, otherwise the new block would be appended at the end
		// of the old block's last line instead of overwriting it.
		erase()
		lines := formatStatsBlock(history[:], lastSnapshot)
		fmt.Fprint(w.out, strings.Join(lines, "\n"))
		blockRendered = true
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

// formatStatsBlock renders the per-signal stats as `signalCount` lines,
// one per signal, in the form:
//
//	<label>: <rate>/s <sparkline> <total> total
//
// Every column is right-aligned so the eye can scan vertically:
//   - Labels right-align to the longest signal name's width — the colon
//     becomes the column anchor (`   logs:`, `  spans:`, `metrics:`).
//   - Rate right-aligns in 5 columns.
//   - Sparklines are left-padded to sparklineMaxWidth so signals with
//     less history align with the cap.
//   - Totals right-align across rows to the widest total's digit count,
//     so leading whitespace grows for smaller numbers and `<digits>
//     total` always lines up.
func formatStatsBlock(history [][]float64, snap SnapshotWithRate) []string {
	labels := [signalCount]string{"logs", "spans", "metrics"}

	// Width of the longest signal label plus its colon, so all labels
	// right-align to that width: `   logs:`, `  spans:`, `metrics:` —
	// eight columns each, colon at column 7.
	const labelWidth = len("metrics:")

	// Pre-render totals as strings and find the max digit count so the
	// total column right-aligns across rows.
	totalStrs := [signalCount]string{}
	totalWidth := 0
	for i := 0; i < signalCount; i++ {
		totalStrs[i] = strconv.FormatInt(snap.Forwarded[i], 10)
		if len(totalStrs[i]) > totalWidth {
			totalWidth = len(totalStrs[i])
		}
	}

	lines := make([]string, signalCount)
	for i, label := range labels {
		var samples []float64
		if i < len(history) {
			samples = history[i]
		}
		spark := renderPaddedSparkline(samples, sparklineMaxWidth)
		lines[i] = fmt.Sprintf("%*s %5.0f/s %s %*s total",
			labelWidth, label+":", snap.Rate[i], spark, totalWidth, totalStrs[i])
	}
	return lines
}

// renderPaddedSparkline returns a sparkline string padded to `width`
// columns. Missing history (zero samples) renders as `width` spaces so
// the columns following the sparkline still line up across signals.
// Histories with fewer than `width` samples are left-padded with spaces
// so the most recent sample sits at the right edge.
func renderPaddedSparkline(samples []float64, width int) string {
	if len(samples) == 0 {
		return strings.Repeat(" ", width)
	}
	if len(samples) > width {
		samples = samples[len(samples)-width:]
	}
	s := Sparkline(samples)
	if pad := width - utf8.RuneCountInString(s); pad > 0 {
		return strings.Repeat(" ", pad) + s
	}
	return s
}

