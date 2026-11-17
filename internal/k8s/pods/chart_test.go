package pods

import (
	"strings"
	"testing"
	"time"
)

func f(v float64) *float64 { return &v }

func TestFormatValue(t *testing.T) {
	cases := []struct {
		v    float64
		unit string
		want string
	}{
		{94711762944, "bytes", "94.7 GB"},
		{850, "bytes", "850 B"},
		{7.966, "cores", "7.97 cores"},
		{1128, "", "1128"},
		{12500, "", "12.5K"},
	}
	for _, c := range cases {
		if got := formatValue(f(c.v), c.unit); got != c.want {
			t.Errorf("formatValue(%v, %q) = %q, want %q", c.v, c.unit, got, c.want)
		}
	}
	if got := formatValue(nil, "bytes"); got != "no data" {
		t.Errorf("nil reads %q", got)
	}
}

func TestNice(t *testing.T) {
	cases := []struct{ in, want span }{
		{span{6.7, 36}, span{0, 40}},
		{span{77e9, 146e9}, span{60e9, 160e9}},
		{span{99, 102}, span{99, 103}},
	}
	for _, c := range cases {
		if got := nice(c.in); got != c.want {
			t.Errorf("nice(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestFitKeepsSamplesAndGaps(t *testing.T) {
	line, _ := fit([]*float64{f(1), nil, f(3)}, 5)
	if *line[0] != 1 || line[1] != nil || line[2] != nil || *line[4] != 3 {
		t.Errorf("a gap must stay a gap, never a zero: %v", line)
	}
	line, _ = fit([]*float64{f(1), f(3), f(5), f(7)}, 2)
	if len(line) != 2 || *line[0] != 2 || *line[1] != 6 {
		t.Errorf("folded columns take their mean: %v", line)
	}
}

func TestTimeseriesDrawsAxisAndLegend(t *testing.T) {
	start := time.Date(2026, 11, 11, 18, 0, 0, 0, time.UTC)
	times := []time.Time{start, start.Add(time.Minute), start.Add(2 * time.Minute)}
	out := timeseries([]series{{"usage", []*float64{f(1), f(2), f(3)}, "#828aff"}}, times, "cores", 40, 5)
	var all []string
	for _, l := range out {
		var sb strings.Builder
		for _, s := range l {
			sb.WriteString(s.text)
		}
		all = append(all, sb.String())
	}
	joined := strings.Join(all, "\n")
	for _, want := range []string{"cores", "┤", "└┬", "━ usage", "┄"} {
		if !strings.Contains(joined, want) {
			t.Errorf("chart lacks %q:\n%s", want, joined)
		}
	}
	if empty := timeseries([]series{{"usage", nil, "#828aff"}}, nil, "", 40, 5); !strings.Contains(empty[0][0].text, "No data") {
		t.Errorf("an empty series says so")
	}
}

func TestQueryEscapesLabelValues(t *testing.T) {
	q := target{namespace: `a"b`, kind: "pod", name: "p"}.query("k8s.pod.cpu.usage")
	if !strings.Contains(q, `k8s_namespace_name="a\"b"`) {
		t.Errorf("unescaped: %s", q)
	}
}

func TestBrailleDrawsOneJoinedLine(t *testing.T) {
	// 0 then 1 on one row: the bottom-left dot, the midpoint joined up its right-hand column, then the top dot
	cells := braille([][]*float64{{f(0), f(1)}}, []string{"#828aff"}, 1, span{0, 1})
	if got := cells[0][0].sides; got != 0x40|0x20|0x10 {
		t.Errorf("first cell %#x", got)
	}
	if got := cells[0][1].sides; got != 0x01 {
		t.Errorf("second cell %#x", got)
	}
	// The series listed first colours a shared cell
	cells = braille([][]*float64{{f(0)}, {f(0)}}, []string{"first", "second"}, 1, span{0, 1})
	if cells[0][0].color != "first" {
		t.Errorf("shared cell coloured %q", cells[0][0].color)
	}
}

func TestBrailleDrawsADescendingLine(t *testing.T) {
	// 1 then 0: the top-left dot, the midpoint one row down the right-hand column, then down to the bottom
	cells := braille([][]*float64{{f(1), f(0)}}, []string{"c"}, 1, span{0, 1})
	if got := cells[0][0].sides; got != 0x01|0x10 {
		t.Errorf("first cell %#x", got)
	}
	if got := cells[0][1].sides; got != 0x04|0x40 {
		t.Errorf("second cell %#x", got)
	}
}

func TestBrailleBreaksTheLineAtAGap(t *testing.T) {
	cells := braille([][]*float64{{f(0), nil, f(0)}}, []string{"c"}, 1, span{0, 1})
	if cells[0][1].sides != 0 || cells[0][1].color != "" {
		t.Errorf("gap drawn: %+v", cells[0][1])
	}
	// Each end is a lone sample, so a dash along the bottom row
	if cells[0][0].sides != 0x40|0x80 || cells[0][2].sides != 0x40|0x80 {
		t.Errorf("ends %#x %#x", cells[0][0].sides, cells[0][2].sides)
	}
}

func TestBrailleKeepsOnlyTheFirstSeriesDotsInASharedCell(t *testing.T) {
	// The second series' bottom dot would show in the first series' colour if the dots were merged
	cells := braille([][]*float64{{f(1)}, {f(0)}}, []string{"first", "second"}, 1, span{0, 1})
	if got := cells[0][0]; got.sides != 0x01|0x08 || got.color != "first" {
		t.Errorf("shared cell %+v", got)
	}
}

func TestBrailleJoinsAcrossCellRows(t *testing.T) {
	// 0 then 1 over two rows: up the right-hand column of the bottom cell, on into the top one
	cells := braille([][]*float64{{f(0), f(1)}}, []string{"c"}, 2, span{0, 1})
	want := [2][2]int{{0x80, 0x07}, {0x78, 0}}
	for r := range want {
		for c := range want[r] {
			if got := cells[r][c].sides; got != want[r][c] {
				t.Errorf("cell %d,%d %#x, want %#x", r, c, got, want[r][c])
			}
		}
	}
}

func TestBrailleDrawsALoneSampleAsADash(t *testing.T) {
	cells := braille([][]*float64{{nil, f(0.5), nil}}, []string{"c"}, 3, span{0, 1})
	// 0.5 on 12 dot rows lands on dot row 5: the second row of the middle cell, both columns
	if got := cells[1][1].sides; got != 0x02|0x10 {
		t.Errorf("lone sample %#x", got)
	}
}

func TestBrailleClampsValuesOutsideTheSpan(t *testing.T) {
	cells := braille([][]*float64{{f(-1), f(2)}}, []string{"c"}, 1, span{0, 1})
	if cells[0][0].sides&0x40 == 0 || cells[0][1].sides&0x01 == 0 {
		t.Errorf("clamped %#x %#x", cells[0][0].sides, cells[0][1].sides)
	}
}

func TestFlattensCountsDotRows(t *testing.T) {
	// Errors moving 0 to 15 beside requests up to 100 cross three of the 19 dot-row steps, so they stay on one axis
	ss := []series{{values: []*float64{f(0), f(100)}}, {values: []*float64{f(0), f(15)}}}
	if flattens(ss, 5) {
		t.Error("errors that move three dot rows sent to small multiples")
	}
	// Moving 4 of 100 is under one dot row
	ss[1].values = []*float64{f(0), f(4)}
	if !flattens(ss, 5) {
		t.Error("a line flat to the dot kept on the shared axis")
	}
}
