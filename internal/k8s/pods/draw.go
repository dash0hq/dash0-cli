package pods

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/dash0hq/ditt0-cli-design-system/go/ditt0"
)

// The page draws with Ditt0, Dash0's design system for terminal UIs
// (github.com/dash0hq/ditt0-cli-design-system/go/ditt0): its colours, Stat, Timeseries and layout. This file is what
// the page adds on top: which chart keeps one axis, and what a stat compares with.

type series = ditt0.Series

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

func peakOf(ss []series) float64 {
	peak := 0.0
	for _, s := range ss {
		for _, v := range s.Values {
			if v != nil {
				peak = math.Max(peak, math.Abs(*v))
			}
		}
	}
	return peak
}

// Whether a series that moves would draw as one flat line of dots on the others' axis, as 900 error logs do beside
// 30K unknown ones: it would hide its own changes, and under another series its colour. A braille row is four dot
// rows, so a series flattens when it spans less than one dot row
func flattens(ss []series, rows int) bool {
	allLo, allHi := math.Inf(1), math.Inf(-1)
	for _, s := range ss {
		for _, v := range s.Values {
			if v != nil {
				allLo, allHi = math.Min(allLo, *v), math.Max(allHi, *v)
			}
		}
	}
	for _, s := range ss {
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, v := range s.Values {
			if v != nil {
				lo, hi = math.Min(lo, *v), math.Max(hi, *v)
			}
		}
		if hi > lo && hi-lo < (allHi-allLo)/float64(4*rows-1) {
			return true
		}
	}
	return false
}

// The chart, width cells wide, its plot rows high, as Ditt0's Timeseries in its Dots line type. times: one per sample
// multiples: series that would flatten on one axis draw as small multiples instead, a short panel each with its own
// axis, headed by its name and unit. CPU and memory keep usage on its request's and limit's axis, where reading one
// against the other is the point
func chart(ss []series, times []time.Time, unit string, width, rows int, multiples bool) ditt0.Block {
	b := ditt0.Timeseries(ditt0.TimeseriesProps{
		Series: ss, Times: times, Unit: unit, Width: width, Rows: rows,
		OneAxis:  !(multiples && len(ss) > 1 && flattens(ss, rows)),
		LineType: ditt0.LineTypeBraille,
	})
	// A lone unnamed series needs no legend, its card's title names it: Ditt0 draws the legend as the last line
	if len(ss) == 1 && ss[0].Name == "" && len(b) > 0 {
		b = b[:len(b)-1]
	}
	return b
}

// Cards in a row, each chart grown until its card is as tall as the tallest, so the row's edges line up. One more row
// is one more line on one axis, and on small multiples too once every panel is past its fewest rows, where Ditt0 gives
// the row to a panel; below that a card stays as tall, so it takes a few more. A card that still can't match (one
// that says it is loading, say) gets blank lines inside its border
func fill(rows int, cards ...func(rows int) ditt0.Block) []ditt0.Block {
	out := make([]ditt0.Block, len(cards))
	h := 0
	for i, c := range cards {
		out[i] = c(rows)
		h = max(h, len(out[i]))
	}
	for i, c := range cards {
		// Each try adds a row, so h more is always enough for a card that grows
		for r := rows + 1; len(out[i]) < h && r <= rows+2*h; r++ {
			if b := c(r); len(b) <= h {
				out[i] = b
			}
		}
		out[i] = padCard(out[i], h)
	}
	return out
}

// A bordered card made h lines tall with blank lines above its bottom border
func padCard(b ditt0.Block, h int) ditt0.Block {
	if len(b) >= h || len(b) < 2 {
		return b
	}
	blank := ditt0.Line{{Text: "│", Style: ditt0.DimStyle}, {Text: strings.Repeat(" ", b.Width()-2)}, {Text: "│", Style: ditt0.DimStyle}}
	out := append(ditt0.Block{}, b[:len(b)-1]...)
	for len(out) < h-1 {
		out = append(out, blank)
	}
	return append(out, b[len(b)-1])
}

// Ditt0's Stat for a range: ▲ or ▼ since the range's first value. ink: a status colour the value reached, or "" for
// the theme's text; note: a dim line under the sparkline; goodUp: a rise is good (a request rate's), else bad
// (a latency's, an error rate's)
func stat(label string, values []*float64, unit, ink, compareLabel, note string, goodUp bool, t ditt0.Theme) ditt0.Block {
	goodWhen := "down"
	if goodUp {
		goodWhen = "up"
	}
	return ditt0.Stat(ditt0.StatProps{
		Label: label, Values: values, Unit: unit, Previous: first(values), CompareLabel: compareLabel,
		GoodWhen: goodWhen, Ink: ink, Note: note,
	}, t)
}

// The line's text alone, for --color none
func plain(l ditt0.Line) string {
	var sb strings.Builder
	for _, s := range l {
		sb.WriteString(s.Text)
	}
	return sb.String()
}

// Cells Ditt0's Timeseries leaves empty right of the plot, so the line doesn't run into the edge of its card
const rightGap = 2

// HH:MM on the local clock, as Dash0 shows times, and the zone as the charts write it at the end of their axis
func clock(t time.Time) string { return t.Local().Format("15:04") }

func zoneOf(t time.Time) string {
	_, off := t.Local().Zone()
	if off == 0 {
		return "UTC"
	}
	sign := "+"
	if off < 0 {
		sign, off = "-", -off
	}
	h, m := off/3600, off%3600/60
	if m == 0 {
		return fmt.Sprintf("GMT%s%d", sign, h)
	}
	return fmt.Sprintf("GMT%s%d:%02d", sign, h, m)
}
