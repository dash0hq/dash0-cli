package color

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
)

// SprintSpanStatus returns the span status string color-coded and padded to
// width visible characters for terminal output. When width is 0, no padding
// is applied.
func SprintSpanStatus(status string, width int) string {
	if color.NoColor || !isatty.IsTerminal(os.Stdout.Fd()) {
		if width > 0 {
			return fmt.Sprintf("%-*s", width, status)
		}
		return status
	}
	return sprintSpanStatusColored(status, width)
}

func sprintSpanStatusColored(status string, width int) string {
	padded := status
	if width > 0 {
		padded = fmt.Sprintf("%-*s", width, status)
	}
	switch status {
	case "ERROR":
		return colorError.Sprint(padded)
	default:
		return padded
	}
}
