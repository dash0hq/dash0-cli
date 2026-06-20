package color

import (
	"fmt"
	"strings"
)

// SprintIssueStatus returns the failed-check issue status color-coded and
// padded to width visible characters for terminal output. When width is 0, no
// padding is applied.
func SprintIssueStatus(status string, width int) string {
	if NoColor {
		if width > 0 {
			return fmt.Sprintf("%-*s", width, status)
		}
		return status
	}
	return sprintIssueStatusColored(status, width)
}

func sprintIssueStatusColored(status string, width int) string {
	o := StdoutOutput()

	padded := status
	if width > 0 {
		padded = fmt.Sprintf("%-*s", width, status)
	}

	switch strings.ToLower(status) {
	case "critical":
		return o.String(padded).Foreground(o.Color("1")).String() // red
	case "degraded":
		return o.String(padded).Foreground(o.Color("3")).String() // yellow
	case "healthy":
		return o.String(padded).Foreground(o.Color("2")).String() // green
	default:
		return padded
	}
}
