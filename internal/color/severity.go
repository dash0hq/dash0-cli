package color

import (
	"fmt"
	"os"

	"github.com/dash0hq/dash0-cli/internal/otlp"
	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
)

// severityWidth is the column width reserved for the severity field in table output.
const severityWidth = 10

var (
	colorError   = color.New(color.FgRed)
	colorWarn    = color.New(color.FgYellow)
	colorInfo    = color.New(color.FgHiCyan)
	colorUnknown = color.New(color.FgHiBlack)
)

// SprintSeverity returns the severity string color-coded for terminal output.
// The returned string is padded to severityWidth visible characters so that
// table columns stay aligned even when ANSI escape codes are present.
// When color is disabled (via color.NoColor) or stdout is not a TTY, the
// severity is returned as plain left-padded text.
func SprintSeverity(sev string) string {
	if color.NoColor || !isatty.IsTerminal(os.Stdout.Fd()) {
		return fmt.Sprintf("%-*s", severityWidth, sev)
	}
	return sprintSeverityColored(sev)
}

func sprintSeverityColored(sev string) string {
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
		return fmt.Sprintf("%-*s", severityWidth, sev)
	}
	// Pad the visible text first, then wrap with ANSI codes so the
	// terminal sees the correct column width.
	padded := fmt.Sprintf("%-*s", severityWidth, sev)
	return c.Sprint(padded)
}
