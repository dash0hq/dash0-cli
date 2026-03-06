package color

import (
	"fmt"
)

// SprintSpanStatus returns the span status string color-coded and padded to
// width visible characters for terminal output. When width is 0, no padding
// is applied.
func SprintSpanStatus(status string, width int) string {
	if NoColor {
		if width > 0 {
			return fmt.Sprintf("%-*s", width, status)
		}
		return status
	}
	return sprintSpanStatusColored(status, width)
}

func sprintSpanStatusColored(status string, width int) string {
	o := StdoutOutput()

	padded := status
	if width > 0 {
		padded = fmt.Sprintf("%-*s", width, status)
	}

	switch status {
	case "ERROR":
		return o.String(padded).Foreground(o.Color("1")).String() // red
	default:
		return padded
	}
}
