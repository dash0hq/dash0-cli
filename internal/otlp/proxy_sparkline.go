package otlp

import (
	"math"
	"strings"
)

// sparklineGlyphs holds the 7-level Unicode block ramp used by Sparkline.
// Each rune lines up at the baseline so successive runes form a
// flat-bottomed timeline silhouette. The full-block character (█) is
// intentionally omitted: at the cap, a full block fuses visually with
// the line of text above and makes the silhouette look like it bleeds
// upward. Stopping at ▇ (7/8 cell height) leaves a one-pixel gap at the
// top, so the tallest bar never reaches the row above.
const sparklineGlyphs = "▁▂▃▄▅▆▇"

// Sparkline returns a string of len(series) characters from the
// sparklineGlyphs ramp, normalized against the maximum non-zero value in the
// series. Zero and negative values map to the lowest glyph; NaN and Inf are
// treated as zero so a single ill-formed sample doesn't blank the whole
// rendering.
//
// Empty input returns the empty string so callers can suppress rendering
// without an additional length check.
func Sparkline(series []float64) string {
	if len(series) == 0 {
		return ""
	}
	runes := []rune(sparklineGlyphs)
	levels := len(runes)

	maxVal := 0.0
	for _, v := range series {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		if v > maxVal {
			maxVal = v
		}
	}

	var b strings.Builder
	b.Grow(len(series) * 4) // upper bound: each rune is up to 4 bytes
	for _, v := range series {
		if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || maxVal == 0 {
			b.WriteRune(runes[0])
			continue
		}
		// Map [0, maxVal] linearly onto [0, levels-1]. Tied to maxVal
		// rather than an absolute scale so a steady-state series still
		// shows variation against its own range.
		idx := int(math.Round(v / maxVal * float64(levels-1)))
		switch {
		case idx < 0:
			idx = 0
		case idx >= levels:
			idx = levels - 1
		}
		b.WriteRune(runes[idx])
	}
	return b.String()
}
