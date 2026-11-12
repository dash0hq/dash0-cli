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
	q := target{namespace: `a"b`, pod: "p"}.query("k8s.pod.cpu.usage")
	if !strings.Contains(q, `k8s_namespace_name="a\"b"`) {
		t.Errorf("unescaped: %s", q)
	}
}
