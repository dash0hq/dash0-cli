package pods

import (
	"fmt"
	"testing"
	"time"
)

func TestNextKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"q", "q"},
		{"\x1b", "\x1b"},
		{"\x1b[A", "\x1b[A"},
		{"\x1b[5~", "\x1b[5~"},
		{"\x1bOH", "\x1bOH"},
		{"\x1bOP", "\x1bOP"},       // F1
		{"\x1b[Z", "\x1b[Z"},       // Shift-Tab
		{"\x1b[15~", "\x1b[15~"},   // F5
		{"\x1b[1;5A", "\x1b[1;5A"}, // Ctrl-Up
		{"\x1bx", "\x1bx"},         // Alt-x
		{"\x1b[Ar", "\x1b[A"},      // a sequence, then a key
		{"\x1b[1;5", "\x1b[1;5"},   // cut short: read whole, never as Esc
	}
	for _, c := range cases {
		if got := nextKey(c.in); got != c.want {
			t.Errorf("nextKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// k9s can hand the page a pane of any size: no width or height may panic
func TestRenderTinyTerminals(t *testing.T) {
	values := []*float64{f(0.1), f(0.2), nil, f(0.4)}
	end := time.Unix(1_800_000_000, 0)
	for _, withData := range []bool{false, true} {
		p := &page{target: target{namespace: "ns", pod: "pod"}, theme: dark, apiURL: "https://api.example.com"}
		if withData {
			p.win = window{end.Add(-3 * time.Minute), end, time.Minute}
			p.at = end
			p.data = map[string][]*float64{"cpu": values, "mem": values, "requests": values, "errors": values,
				"p50": values, "p95": values, "p99": values, "logs:ERROR": values}
		}
		for w := 0; w <= 40; w++ {
			for _, h := range []int{0, 1, 5, 50} {
				t.Run(fmt.Sprintf("data=%v/%dx%d", withData, w, h), func(t *testing.T) {
					p.render(w, h)
				})
			}
		}
	}
}

func TestPlainHasNoEscapes(t *testing.T) {
	l := line{{"a", style{fg: "#ffffff", bold: true, link: "https://example.com"}}, {"b", dimStyle}}
	if got := l.plain(); got != "ab" {
		t.Errorf("plain() = %q", got)
	}
}
