package output

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

const (
	progressThreshold = 10
	labelWidth        = 30
	// pctWidth covers " 100%" at the end of the line.
	pctWidth      = 5
	minBarWidth   = 10
	fallbackWidth = 80
)

// Progress displays a progress bar on stderr when the total exceeds the
// threshold and stderr is a terminal. The bar adapts to the terminal width.
//
// Layout:
//
//	Fetching 47 dashboards        ################                         45%
//	|--- label (30 chars) ------->|--- bar (adapts to terminal) -------->|pct|
type Progress struct {
	assetType string
	total     int
	active    bool
	lineWidth int
}

// NewProgress creates a progress indicator. It only activates when total
// exceeds the threshold and stderr is a TTY.
func NewProgress(assetType string, total int) *Progress {
	fd := int(os.Stderr.Fd())
	active := total > progressThreshold && term.IsTerminal(fd)
	width := fallbackWidth
	if active {
		if w, _, err := term.GetSize(fd); err == nil && w > 0 {
			width = w
		}
	}
	return &Progress{
		assetType: assetType,
		total:     total,
		active:    active,
		lineWidth: width,
	}
}

// Update prints the current progress. Call this after each item is fetched.
func (p *Progress) Update(current int) {
	if !p.active {
		return
	}
	barWidth := max(p.lineWidth-labelWidth-pctWidth, minBarWidth)
	pct := current * 100 / p.total
	filled := pct * barWidth / 100
	bar := strings.Repeat("#", filled) + strings.Repeat(" ", barWidth-filled)
	label := fmt.Sprintf("Fetching %d %s", p.total, p.assetType)
	fmt.Fprintf(os.Stderr, "\r%-*s%s %3d%%", labelWidth, label, bar, pct)
}

// Done clears the progress line.
func (p *Progress) Done() {
	if !p.active {
		return
	}
	fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", p.lineWidth))
}
