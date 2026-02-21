package color

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
)

// spanStatusWidth is the column width reserved for the span status field in table output.
const spanStatusWidth = 8

// SprintSpanStatus returns the span status string color-coded for terminal output.
// The returned string is padded to spanStatusWidth visible characters so that
// table columns stay aligned even when ANSI escape codes are present.
func SprintSpanStatus(status string) string {
	if color.NoColor || !isatty.IsTerminal(os.Stdout.Fd()) {
		return fmt.Sprintf("%-*s", spanStatusWidth, status)
	}
	return sprintSpanStatusColored(status)
}

func sprintSpanStatusColored(status string) string {
	padded := fmt.Sprintf("%-*s", spanStatusWidth, status)
	switch status {
	case "ERROR":
		return colorError.Sprint(padded)
	default:
		return padded
	}
}
