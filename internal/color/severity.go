package color

import (
	"fmt"

	"github.com/dash0hq/dash0-cli/internal/otlp"
	"github.com/muesli/termenv"
)

// SprintSeverity returns the severity string color-coded and padded to width
// visible characters for terminal output. When width is 0, no padding is applied.
// When color is disabled (via NoColor) or stdout is not a TTY, the
// severity is returned as plain left-padded text.
func SprintSeverity(sev string, width int) string {
	if NoColor {
		if width > 0 {
			return fmt.Sprintf("%-*s", width, sev)
		}
		return sev
	}
	return sprintSeverityColored(sev, width)
}

func sprintSeverityColored(sev string, width int) string {
	o := StdoutOutput()

	padded := sev
	if width > 0 {
		padded = fmt.Sprintf("%-*s", width, sev)
	}

	var fg termenv.Color
	switch otlp.OtlpLogSeverityRange(sev) {
	case otlp.Error, otlp.Fatal:
		fg = o.Color("1") // red
	case otlp.Warn:
		fg = o.Color("3") // yellow
	case otlp.Info:
		fg = o.Color("14") // bright cyan
	case otlp.Unknown:
		fg = o.Color("8") // bright black (grey)
	default:
		// Debug, Trace, custom text — no color
		return padded
	}

	return o.String(padded).Foreground(fg).String()
}
