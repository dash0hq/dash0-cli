package pods

import (
	"github.com/dash0hq/ditt0-cli-design-system/go/ditt0"
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
		if got := ditt0.FormatValue(f(c.v), c.unit); got != c.want {
			t.Errorf("ditt0.FormatValue(%v, %q) = %q, want %q", c.v, c.unit, got, c.want)
		}
	}
	if got := ditt0.FormatValue(nil, "bytes"); got != "no data" {
		t.Errorf("nil reads %q", got)
	}
}

func TestTimeseriesDrawsAxisAndLegend(t *testing.T) {
	start := time.Date(2026, 11, 11, 18, 0, 0, 0, time.UTC)
	times := []time.Time{start, start.Add(time.Minute), start.Add(2 * time.Minute)}
	out := chart([]series{{Name: "usage", Values: []*float64{f(1), f(2), f(3)}, Color: "#828aff"}}, times, "cores", 40, 5, false)
	joined := out.String()
	for _, want := range []string{"cores", "┤", "└┬", "━ usage", "┄"} {
		if !strings.Contains(joined, want) {
			t.Errorf("chart lacks %q:\n%s", want, joined)
		}
	}
	if empty := chart([]series{{Name: "usage", Color: "#828aff"}}, nil, "", 40, 5, false); !strings.Contains(empty.String(), "No data") {
		t.Errorf("an empty series says so")
	}
}

func TestQueryEscapesLabelValues(t *testing.T) {
	q := target{namespace: `a"b`, kind: "pod", name: "p"}.query("k8s.pod.cpu.usage")
	if !strings.Contains(q, `k8s_namespace_name="a\"b"`) {
		t.Errorf("unescaped: %s", q)
	}
}

// The charts are Ditt0's Dots line type: braille, a gap in the samples a gap in the line, never a zero
func TestChartDrawsBrailleAndKeepsGaps(t *testing.T) {
	start := time.Date(2026, 11, 11, 18, 0, 0, 0, time.UTC)
	var times []time.Time
	values := make([]*float64, 30)
	for i := range values {
		times = append(times, start.Add(time.Duration(i)*time.Minute))
		if i < 10 || i >= 20 {
			values[i] = f(float64(i % 7))
		}
	}
	out := chart([]series{{Name: "usage", Values: values, Color: "#828aff"}}, times, "cores", 60, 5, false)
	dots, gap := 0, 0
	for _, l := range out {
		col := 0
		for _, s := range l {
			for _, r := range s.Text {
				if r > 0x2800 && r <= 0x28ff {
					dots++
					// The samples 10 to 19 of 30 are missing: the middle third of the plot, past the 4 cells of y axis
					if plot := col - 4; plot > 20 && plot < 32 {
						gap++
					}
				}
				col++
			}
		}
	}
	if dots == 0 {
		t.Fatalf("no braille in the chart:\n%s", out.String())
	}
	if gap != 0 {
		t.Errorf("%d cells drawn inside the gap:\n%s", gap, out.String())
	}
}

// Small multiples only where the page asks for them and a series would flatten; CPU and memory keep one axis
func TestChartSplitsOnlyWhenASeriesFlattens(t *testing.T) {
	start := time.Date(2026, 11, 11, 18, 0, 0, 0, time.UTC)
	times := []time.Time{start, start.Add(time.Minute), start.Add(2 * time.Minute)}
	ss := []series{
		{Name: "UNKNOWN", Values: []*float64{f(30000), f(31000), f(29000)}, Color: "#939396"},
		{Name: "ERROR", Values: []*float64{f(900), f(905), f(910)}, Color: "#e33238"},
	}
	if out := chart(ss, times, "/s", 60, 8, true).String(); !strings.Contains(out, "━ ERROR · /s") {
		t.Errorf("a flat series gets a panel of its own:\n%s", out)
	}
	if out := chart(ss, times, "/s", 60, 8, false).String(); strings.Contains(out, "━ ERROR · /s") || !strings.Contains(out, "━ UNKNOWN") {
		t.Errorf("one axis when the page asks for it:\n%s", out)
	}
	five := []series{}
	for i, name := range []string{"pod-a", "pod-b", "pod-c", "pod-d", "pod-e"} {
		five = append(five, series{Name: name, Values: []*float64{f(float64(i)), f(float64(5 - i)), f(float64(i))}, Color: ditt0.Dark.ManagedSeriesColor(i)})
	}
	if out := chart(five, times, "cores", 60, 8, true).String(); strings.Contains(out, " · cores") || !strings.Contains(out, "━ pod-e") {
		t.Errorf("five pods that move stay on one axis:\n%s", out)
	}
}

func TestFlattensCountsDotRows(t *testing.T) {
	// Errors moving 0 to 15 beside requests up to 100 cross three of the 19 dot-row steps, so they stay on one axis
	ss := []series{{Values: []*float64{f(0), f(100)}}, {Values: []*float64{f(0), f(15)}}}
	if flattens(ss, 5) {
		t.Error("errors that move three dot rows sent to small multiples")
	}
	// Moving 4 of 100 is under one dot row
	ss[1].Values = []*float64{f(0), f(4)}
	if !flattens(ss, 5) {
		t.Error("a line flat to the dot kept on the shared axis")
	}
}

// Cards in a row end up equally tall: one axis, small multiples that split their rows, and a card that can't grow
func TestFillMatchesHeights(t *testing.T) {
	start := time.Date(2026, 11, 11, 18, 0, 0, 0, time.UTC)
	times := []time.Time{start, start.Add(time.Minute), start.Add(2 * time.Minute)}
	big, small, tiny := []*float64{f(1200), f(1300), f(1250)}, []*float64{f(1), f(3), f(2)}, []*float64{f(0.004), f(0.006), f(0.005)}
	card := func(ss []series, multiples bool) func(int) ditt0.Block {
		return func(rows int) ditt0.Block {
			return ditt0.Border(chart(ss, times, "seconds", 40, rows, multiples), 44, 1, ditt0.DimStyle)
		}
	}
	fixed := func(int) ditt0.Block {
		return ditt0.Border(ditt0.Block{ditt0.Text("loading", ditt0.DimStyle)}, 44, 1, ditt0.DimStyle)
	}
	for _, rows := range []int{5, 8, 12} {
		got := fill(rows,
			card([]series{{Name: "requests", Values: big}, {Name: "errors", Values: small}}, true),
			card([]series{{Values: small}}, false),
			card([]series{{Name: "p99", Values: big}, {Name: "p95", Values: small}, {Name: "p50", Values: tiny}}, true),
			fixed)
		for i, b := range got {
			if len(b) != len(got[0]) {
				t.Errorf("rows %d: card %d is %d lines, card 0 %d", rows, i, len(b), len(got[0]))
			}
		}
	}
}
