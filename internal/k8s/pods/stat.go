package pods

import (
	"math"
	"regexp"
	"strings"
)

// Ditt0's Stat, ported from ditt0-cli-design-system/components/molecules/charts/stat/stat.jsx: the number in digits three rows tall, its
// unit on the bottom row beside it, ▲ or ▼ since the start of the range, and the values as a sparkline under it.

var bigDigits = map[rune][3]string{
	'0': {"┏━┓", "┃ ┃", "┗━┛"}, '1': {"╺┓ ", " ┃ ", "╺┻╸"}, '2': {"╺━┓", "┏━┛", "┗━╸"}, '3': {"╺━┓", " ━┫", "╺━┛"},
	'4': {"╻ ╻", "┗━┫", "  ╹"}, '5': {"┏━╸", "┗━┓", "╺━┛"}, '6': {"┏━╸", "┣━┓", "┗━┛"}, '7': {"╺━┓", "  ┃", "  ╹"},
	'8': {"┏━┓", "┣━┫", "┗━┛"}, '9': {"┏━┓", "┗━┫", "╺━┛"}, '.': {" ", " ", "╻"}, '-': {"   ", "╺━╸", "   "},
}

const eighths = "▁▂▃▄▅▆▇█"
const sparkWidth = 30

var numberRe = regexp.MustCompile(`^(-?[\d.]+)(.*)$`)

// The values as one row of eighths, each cell the mean of the values it covers, low to high between their min and
// max; a flat line sits at the middle, a gap stays blank
func sparkline(values []*float64, width int) string {
	cells := min(width, len(values))
	means := make([]*float64, cells)
	lo, hi := math.Inf(1), math.Inf(-1)
	for i := range means {
		sum, count := 0.0, 0
		for _, v := range values[i*len(values)/cells : (i+1)*len(values)/cells] {
			if v != nil {
				sum, count = sum+*v, count+1
			}
		}
		if count > 0 {
			m := sum / float64(count)
			means[i], lo, hi = &m, math.Min(lo, m), math.Max(hi, m)
		}
	}
	runes := []rune(eighths)
	var sb strings.Builder
	for _, m := range means {
		switch {
		case m == nil:
			sb.WriteRune(' ')
		case hi == lo:
			sb.WriteRune(runes[3])
		default:
			sb.WriteRune(runes[int(math.Round((*m-lo)/(hi-lo)*7))])
		}
	}
	return sb.String()
}

func last(values []*float64) *float64 {
	for i := len(values) - 1; i >= 0; i-- {
		if values[i] != nil {
			return values[i]
		}
	}
	return nil
}

func first(values []*float64) *float64 {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

// label over the number; ink: a status colour the value reached, or "" for the theme's text; note: a dim line under
// the sparkline; compareLabel names what the change is against; goodUp: a rise is good (a request rate's), else bad
// (a latency's, an error rate's: goodWhen 'down')
func stat(label string, values []*float64, unit, ink, compareLabel, note string, goodUp bool, t theme) block {
	current := last(values)
	txt := formatValue(current, unit)
	out := block{text(label, dimStyle)}
	if current == nil {
		out = append(out, text(txt, style{bold: true, dim: true}))
	} else {
		m := numberRe.FindStringSubmatch(txt)
		number, suffix := m[1], strings.TrimSpace(m[2])
		for row := 0; row < 3; row++ {
			parts := []string{}
			for _, ch := range number {
				parts = append(parts, bigDigits[ch][row])
			}
			l := line{{strings.Join(parts, " "), style{fg: ink, bold: true}}}
			if row == 2 && suffix != "" {
				l = append(l, seg{" " + suffix, style{fg: ink, bold: true}})
			}
			out = append(out, l)
		}
	}
	if prev := first(values); prev != nil && current != nil && *prev != 0 {
		change := (*current - *prev) / math.Abs(*prev) * 100
		arrow, color := "=", ""
		good, bad := t.healthyInk, t.errorInk
		if goodUp {
			good, bad = bad, good
		}
		if change > 0 {
			arrow, color = "▲", bad
		} else if change < 0 {
			arrow, color = "▼", good
		}
		abs := math.Abs(change)
		out = append(out, line{{arrow + " " + formatValue(&abs, "") + "%", style{fg: color}}, {" " + compareLabel, dimStyle}})
	}
	if len(values) > 1 {
		st := style{fg: ink, dim: ink == ""}
		out = append(out, text(sparkline(values, sparkWidth), st))
	}
	if note != "" {
		out = append(out, text(note, dimStyle))
	}
	return out
}
