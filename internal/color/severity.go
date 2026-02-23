package color

import (
	"fmt"
	"os"

	"github.com/dash0hq/dash0-cli/internal/otlp"
	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
)

var (
	colorError   = color.New(color.FgRed)
	colorWarn    = color.New(color.FgYellow)
	colorInfo    = color.New(color.FgHiCyan)
	colorUnknown = color.New(color.FgHiBlack)
)

// SprintSeverity returns the severity string color-coded and padded to width
// visible characters for terminal output. When width is 0, no padding is applied.
// When color is disabled (via color.NoColor) or stdout is not a TTY, the
// severity is returned as plain left-padded text.
func SprintSeverity(sev string, width int) string {
	if color.NoColor || !isatty.IsTerminal(os.Stdout.Fd()) {
		if width > 0 {
			return fmt.Sprintf("%-*s", width, sev)
		}
		return sev
	}
	return sprintSeverityColored(sev, width)
}

func sprintSeverityColored(sev string, width int) string {
	var c *color.Color
	switch otlp.OtlpLogSeverityRange(sev) {
	case otlp.Error, otlp.Fatal:
		c = colorError
	case otlp.Warn:
		c = colorWarn
	case otlp.Info:
		c = colorInfo
	case otlp.Unknown:
		c = colorUnknown
	default:
		// Debug, Trace, custom text — no color
		if width > 0 {
			return fmt.Sprintf("%-*s", width, sev)
		}
		return sev
	}
	// Pad the visible text first, then wrap with ANSI codes so the
	// terminal sees the correct column width.
	padded := sev
	if width > 0 {
		padded = fmt.Sprintf("%-*s", width, sev)
	}
	return c.Sprint(padded)
}
