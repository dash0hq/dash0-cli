package otlp

import (
	"fmt"
	"os"

	"github.com/muesli/termenv"
	"golang.org/x/term"
)

// tailColorEnabled is the runtime-resolved color toggle for `--tail`
// output. It is owned by main() via SetTailColorEnabled — the
// internal/color package already imports internal/otlp for the severity
// range type, so the only way to avoid a cycle is to have color flow in
// the opposite direction (main → otlp) instead of pulling color → otlp.
//
// Default is false. Callers that don't initialize it (most tests) get the
// monochrome path automatically.
var tailColorEnabled bool

// SetTailColorEnabled is called by main() once `dashcolor.NoColor` has
// been resolved. Pass true to enable semantic coloring of `--tail`
// output. Safe to call before or after the proxy starts; the renderer
// reads the variable each time it formats a record.
func SetTailColorEnabled(enabled bool) {
	tailColorEnabled = enabled
}

// stdoutOutput returns a termenv output for stdout, downgraded to ASCII
// when tail coloring is disabled or stdout is not a TTY. Mirrors the
// behavior of internal/color.StdoutOutput but lives here to avoid the
// import cycle.
func stdoutOutput() *termenv.Output {
	if !tailColorEnabled || !term.IsTerminal(int(os.Stdout.Fd())) {
		return termenv.NewOutput(os.Stdout, termenv.WithProfile(termenv.Ascii))
	}
	return termenv.NewOutput(os.Stdout)
}

// colorSeverity colorizes an OTLP log severity range string for `--tail`
// output. The color palette mirrors `internal/color.SprintSeverity` so
// `logs query` and `dash0 otlp proxy --tail` render the same severities
// the same way:
//
//	ERROR / FATAL  → red
//	WARN           → yellow
//	INFO           → bright cyan
//	UNKNOWN        → grey
//	DEBUG / TRACE  → unchanged (no color)
//
// width pads the returned string to at least width visible characters.
// When coloring is disabled (non-TTY stdout or --color=none), the
// function falls back to plain left-padded text.
func colorSeverity(sev string, width int) string {
	padded := sev
	if width > 0 {
		padded = fmt.Sprintf("%-*s", width, sev)
	}
	if !tailColorEnabled || !term.IsTerminal(int(os.Stdout.Fd())) {
		return padded
	}

	o := stdoutOutput()
	var fg termenv.Color
	switch OtlpLogSeverityRange(sev) {
	case Error, Fatal:
		fg = o.Color("1") // red
	case Warn:
		fg = o.Color("3") // yellow
	case Info:
		fg = o.Color("14") // bright cyan
	case Unknown:
		fg = o.Color("8") // bright black (grey)
	default:
		// Debug, Trace, custom text — no color
		return padded
	}
	return o.String(padded).Foreground(fg).String()
}

// colorSpanStatus colorizes a span status code string for `--tail`
// output. The palette mirrors `internal/color.SprintSpanStatus`:
//
//	ERROR  → red
//	OK     → green
//	UNSET  → unchanged
//
// width applies the same padding semantics as colorSeverity.
func colorSpanStatus(status string, width int) string {
	padded := status
	if width > 0 {
		padded = fmt.Sprintf("%-*s", width, status)
	}
	if !tailColorEnabled || !term.IsTerminal(int(os.Stdout.Fd())) {
		return padded
	}

	o := stdoutOutput()
	var fg termenv.Color
	switch status {
	case "Error", "ERROR", "STATUS_CODE_ERROR":
		fg = o.Color("1") // red
	case "Ok", "OK", "STATUS_CODE_OK":
		fg = o.Color("2") // green
	default:
		// Unset / unknown — no color
		return padded
	}
	return o.String(padded).Foreground(fg).String()
}
