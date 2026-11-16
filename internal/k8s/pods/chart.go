package pods

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Ditt0's Timeseries in its default 'box' line type, ported from ditt0-cli-design-system/components/molecules/charts/timeseries/timeseries.js:
// box-drawing strokes a row at a time, a dim ┄ rule along every labelled row, max, middle and min on the y axis
// between round numbers, the unit once above it, three times under it, and a legend of ━ swatches.

type series struct {
	name   string
	values []*float64
	color  string
}

// Cells left empty right of the plot, so the line doesn't run into the edge of its card
const rightGap = 2

// Which sides of a cell a stroke leaves by, so a line's strokes join into one glyph
const (
	up = 1 << iota
	down
	left
	right
	point
)

var glyphs = map[int]string{
	up: "╵", down: "╷", left: "╴", right: "╶",
	left | right: "─", up | down: "│",
	right | down: "╭", left | down: "╮", up | right: "╰", up | left: "╯",
	left | right | down: "┬", left | right | up: "┴", up | down | right: "├", up | down | left: "┤",
	up | down | left | right: "┼",
}

func glyph(sides int) string {
	if g, ok := glyphs[sides&^point]; ok {
		return g
	}
	return "•"
}

type span struct{ min, max float64 }

// The span the y axis covers; a flat series gets room either side so its line sits mid-panel
func spanOf(ss []series) span {
	lo, hi, any := math.Inf(1), math.Inf(-1), false
	for _, s := range ss {
		for _, v := range s.values {
			if v != nil {
				lo, hi, any = math.Min(lo, *v), math.Max(hi, *v), true
			}
		}
	}
	if !any {
		return span{0, 1}
	}
	if lo != hi {
		return span{lo, hi}
	}
	pad := math.Abs(lo) / 2
	if pad == 0 {
		pad = 1
	}
	return span{lo - pad, hi + pad}
}

// Widened to whole steps of 1, 2 or 5 times a power of ten, about four steps apart, as d3's nice() does; a middle
// that wouldn't read round takes one more step at the top
func nice(s span) span {
	raw := (s.max - s.min) / 4
	power := math.Pow(10, math.Floor(math.Log10(raw)))
	step := 10 * power
	for _, m := range []float64{1, 2, 5, 10} {
		if m*power >= raw {
			step = m * power
			break
		}
	}
	lo := math.Floor(s.min/step) * step
	hi := math.Ceil(s.max/step) * step
	mid := (lo + hi) / 2
	figures := 2
	if math.Abs(mid) >= 10 {
		figures = 3
	}
	if f := parse(precise(mid, figures)); math.Abs(f-mid) > math.Abs(mid)*1e-9 {
		hi += step
	}
	return span{lo, hi}
}

func parse(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func itoa(n int) string { return strconv.Itoa(n) }
func pad2(n int) string { return fmt.Sprintf("%02d", n) }

// The y labels by row: max, middle and min, with as many figures as it takes to write each exactly
func yLabels(rows int, s span, sc scale) map[int]string {
	values := []float64{s.max, (s.max + s.min) / 2, s.min}
	labels := make([]string, 3)
	for i, v := range values {
		labels[i] = formatNumber(v, sc)
	}
	exact := func() bool {
		if labels[0] == labels[1] || labels[1] == labels[2] || labels[0] == labels[2] {
			return false
		}
		for i, v := range values {
			if math.Abs(parse(labels[i])*sc.divide-v) > math.Abs(v)*1e-9 {
				return false
			}
		}
		return true
	}
	for figures := 3; figures <= 6 && !exact(); figures++ {
		for i, v := range values {
			labels[i] = precise(v/sc.divide, figures)
		}
	}
	return map[int]string{0: labels[0], (rows - 1) / 2: labels[1], rows - 1: labels[2]}
}

// One value per column. With fewer samples than columns each sample sits at its exact value in a column of its own,
// and the line runs straight to the next; with more, samples fold into columns at their mean. index: the sample a
// column's time label names
func fit(values []*float64, columns int) (line []*float64, index []int) {
	n := len(values)
	if n == 0 {
		return nil, nil
	}
	if n > columns {
		size := (n + columns - 1) / columns
		for i := 0; i < n; i += size {
			sum, count := 0.0, 0
			for _, v := range values[i:min(i+size, n)] {
				if v != nil {
					sum, count = sum+*v, count+1
				}
			}
			index = append(index, i)
			if count == 0 {
				line = append(line, nil)
			} else {
				m := sum / float64(count)
				line = append(line, &m)
			}
		}
		return line, index
	}
	if n == 1 {
		return values, []int{0}
	}
	col := func(i int) int { return int(math.Round(float64(i*(columns-1)) / float64(n-1))) }
	line = make([]*float64, columns)
	index = make([]int, columns)
	for i := 0; i < n-1; i++ {
		a, b := col(i), col(i+1)
		for c := a; c <= b; c++ {
			index[c] = i
			if c == b && b != a {
				index[c] = i + 1
			}
			switch {
			case c == a:
				line[c] = values[i]
			case c == b:
				line[c] = values[i+1]
			case values[i] != nil && values[i+1] != nil:
				v := *values[i] + (*values[i+1]-*values[i])*float64(c-a)/float64(b-a)
				line[c] = &v
			}
		}
	}
	return line, index
}

type cell struct {
	sides int
	color string
}

// Each column enters from the left at the previous column's row, runs to its own, and leaves right there; a gap cuts
// the line. Where two series pass through one cell, the one listed first keeps it, as if drawn over the other
func strokes(lines [][]*float64, colors []string, rows int, s span) [][]cell {
	rowOf := func(v float64) int { return rows - 1 - int(math.Round((v-s.min)/(s.max-s.min)*float64(rows-1))) }
	columns := len(lines[0])
	cells := make([][]cell, rows)
	for r := range cells {
		cells[r] = make([]cell, columns)
	}
	for k, values := range lines {
		for c, v := range values {
			if v == nil {
				continue
			}
			own := rowOf(*v)
			entry, exit := -1, -1
			if c > 0 && values[c-1] != nil {
				entry = rowOf(*values[c-1])
			}
			if c < columns-1 && values[c+1] != nil {
				exit = own
			}
			top, bottom := own, own
			if entry >= 0 {
				top, bottom = min(own, entry), max(own, entry)
			}
			for r := top; r <= bottom; r++ {
				sides := 0
				if r > top {
					sides |= up
				}
				if r < bottom {
					sides |= down
				}
				if r == entry {
					sides |= left
				}
				if r == exit {
					sides |= right
				}
				if sides == 0 {
					sides = point
				}
				at := &cells[r][c]
				if at.color != "" && at.color != colors[k] {
					continue
				}
				at.sides |= sides
				at.color = colors[k]
			}
		}
	}
	return cells
}

// HH:MM on the local clock, as Dash0 shows times, and the zone once at the end of the axis
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
		return "GMT" + sign + itoa(h)
	}
	return "GMT" + sign + itoa(h) + ":" + pad2(m)
}

// How many rows each panel of small multiples gets: odd, so the middle label and its rule sit exactly halfway
const multipleRows = 5

// Whether a series that moves would draw as one flat row on the others' axis, as 900 error logs do beside 30K
// unknown ones: it would hide its own changes, and under another series its colour (timeseries.js flattens)
func flattens(ss []series, rows int) bool {
	all := spanOf(ss)
	for _, s := range ss {
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, v := range s.values {
			if v != nil {
				lo, hi = math.Min(lo, *v), math.Max(hi, *v)
			}
		}
		if hi > lo && hi-lo < (all.max-all.min)/float64(rows-1) {
			return true
		}
	}
	return false
}

func peakOf(ss []series) float64 {
	peak := 0.0
	for _, s := range ss {
		for _, v := range s.values {
			if v != nil {
				peak = math.Max(peak, math.Abs(*v))
			}
		}
	}
	return peak
}

// One panel's y axis and strokes, rows high, each row the axis then a cell per column
func plot(ss []series, lines [][]*float64, rows int, rng span, labels map[int]string, labelWidth int) block {
	colors := make([]string, len(ss))
	for k, s := range ss {
		colors[k] = s.color
	}
	cells := strokes(lines, colors, rows, rng)
	var out block
	for r := 0; r < rows; r++ {
		axis := strings.Repeat(" ", labelWidth) + " │"
		if l, ok := labels[r]; ok {
			axis = padLeft(l, labelWidth) + " ┤"
		}
		row := line{{axis, dimStyle}}
		for c := range cells[r] {
			if at := cells[r][c]; at.color != "" {
				row = append(row, seg{glyph(at.sides), style{fg: at.color}})
			} else if _, ok := labels[r]; ok {
				row = append(row, seg{"┄", dimStyle})
			} else {
				row = append(row, seg{" ", style{}})
			}
		}
		out = append(out, row)
	}
	return out
}

// The time axis under the plot: a tick at the start, middle and end, their times, and the zone
func timeAxis(index []int, times []time.Time, labelWidth int) block {
	columns := len(index)
	ticks := []int{0, (columns - 1) / 2, columns - 1}
	var axis strings.Builder
	axis.WriteString(strings.Repeat(" ", labelWidth) + " └")
	for c := 0; c < columns; c++ {
		if c == ticks[0] || c == ticks[1] || c == ticks[2] {
			axis.WriteString("┬")
		} else {
			axis.WriteString("─")
		}
	}
	out := block{text(axis.String(), dimStyle)}

	// The labels at the start, middle and end; the end one is the last sample's time, ending under its tick
	at := []rune{}
	for k, c := range ticks {
		i := index[c]
		if k == 2 {
			i = len(times) - 1
		}
		label := clock(times[i])
		pos := c
		if k == 2 {
			pos = c + 1 - len(label)
		}
		if len(at) <= pos {
			at = append(at, []rune(strings.Repeat(" ", pos-len(at))+label)...)
		}
	}
	timeRow := strings.Repeat(" ", labelWidth+2) + string(at)
	// The zone after the last label, or on a row of its own under it when the row can't take it
	zone := zoneOf(times[len(times)-1])
	if n := len([]rune(timeRow)); n+1+len(zone) <= labelWidth+2+columns+rightGap {
		return append(out, text(timeRow+" "+zone, dimStyle))
	}
	return append(out, text(timeRow, dimStyle), text(padLeft(zone, labelWidth+2+columns), dimStyle))
}

// The chart, width cells wide, its plot rows high. times: one per sample
func timeseries(ss []series, times []time.Time, unit string, width, rows int) block {
	return chart(ss, times, unit, width, rows, false)
}

// multiples: series that would flatten on one axis draw as small multiples instead, a short panel each with its own
// axis, headed by its name and unit, as Ditt0's Timeseries does. CPU and memory keep usage on its request's and
// limit's axis, where reading one against the other is the point
func chart(ss []series, times []time.Time, unit string, width, rows int, multiples bool) block {
	if len(ss) > 1 && multiples && flattens(ss, rows) {
		return smallMultiples(ss, times, unit, width)
	}
	rng := nice(spanOf(ss))
	// The scale from the largest value, not the rounded axis, so 698 MB stays MB on an axis to 1 GB
	sc := scaleFor(unit, peakOf(ss))
	labels := yLabels(rows, rng, sc)
	unitLabel := strings.TrimSpace(sc.suffix)
	labelWidth := len(unitLabel)
	for _, l := range labels {
		labelWidth = max(labelWidth, len(l))
	}
	columns := max(8, width-labelWidth-2-rightGap)

	lines := make([][]*float64, len(ss))
	var index []int
	for k, s := range ss {
		lines[k], index = fit(s.values, columns)
	}
	if len(index) == 0 {
		return block{text("No data in this time range.", dimStyle)}
	}

	var out block
	if unitLabel != "" {
		out = append(out, text(padLeft(unitLabel, labelWidth), dimStyle))
	}
	out = append(out, plot(ss, lines, rows, rng, labels, labelWidth)...)
	out = append(out, timeAxis(index, times, labelWidth)...)

	// The legend, past the y axis, going on to the next row when one is full
	legendRow := line{{strings.Repeat(" ", labelWidth+2), style{}}}
	for k, s := range ss {
		item := line{{"━", style{fg: s.color}}, {" " + s.name, dimStyle}}
		if k > 0 && legendRow.width()+2+item.width() > width {
			out = append(out, legendRow)
			legendRow = line{{strings.Repeat(" ", labelWidth+2), style{}}}
		} else if k > 0 {
			legendRow = append(legendRow, seg{"  ", style{}})
		}
		legendRow = append(legendRow, item...)
	}
	return append(out, line{}, legendRow)
}

// A panel per series, each on its own axis and scale, so each is headed by its name and unit (ERROR · /s) in its
// colour's place, with a blank row between panels, so one panel's labels never read as the next one's axis. One time
// axis under them all
func smallMultiples(ss []series, times []time.Time, unit string, width int) block {
	type panelOf struct {
		rng    span
		labels map[int]string
		suffix string
	}
	panels := make([]panelOf, len(ss))
	labelWidth := 0
	for k, s := range ss {
		one := []series{s}
		rng := nice(spanOf(one))
		sc := scaleFor(unit, peakOf(one))
		panels[k] = panelOf{rng, yLabels(multipleRows, rng, sc), strings.TrimSpace(sc.suffix)}
		for _, l := range panels[k].labels {
			labelWidth = max(labelWidth, len(l))
		}
	}
	columns := max(8, width-labelWidth-2-rightGap)

	var out block
	var index []int
	for k, s := range ss {
		var line1 []*float64
		line1, index = fit(s.values, columns)
		if len(index) == 0 {
			return block{text("No data in this time range.", dimStyle)}
		}
		if k > 0 {
			out = append(out, line{})
		}
		head := s.name
		if panels[k].suffix != "" {
			head += " · " + panels[k].suffix
		}
		out = append(out, line{{strings.Repeat(" ", labelWidth+2), style{}}, {"━ ", style{fg: s.color}}, {head, dimStyle}})
		out = append(out, plot([]series{s}, [][]*float64{line1}, multipleRows, panels[k].rng, panels[k].labels, labelWidth)...)
	}
	return append(out, timeAxis(index, times, labelWidth)...)
}

func padLeft(s string, w int) string {
	if n := len([]rune(s)); n < w {
		return strings.Repeat(" ", w-n) + s
	}
	return s
}
