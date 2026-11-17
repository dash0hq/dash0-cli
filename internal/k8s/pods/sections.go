package pods

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// " · max across pods" after a title under avg, min or max
func (p *page) aggNote() string {
	if p.target.isTotal() {
		return ""
	}
	return " · " + p.target.agg + " across pods"
}

// Pods drawn in a per-pod chart. Folding the rest into one line would dwarf them: a deployment that rolled churns
// through dozens of pods in an hour
const maxPods = 8

// key's per-pod series, busiest first, and how many pods had any. Busiest is the highest share of its limit
// (rankKey's per-pod series), as Dash0 flags a pod past 80 and 90 percent; pods without a limit follow, by mean
func (p *page) podSeries(key, rankKey string) ([]series, int) {
	type ranked struct {
		name       string
		values     []*float64
		peak, mean float64
		limited    bool
	}
	var all []ranked
	for k, v := range p.data {
		pod, ok := strings.CutPrefix(k, key+":")
		if !ok || !hasSamples(v) {
			continue
		}
		r := ranked{name: pod, values: v, mean: mean(v)}
		if u := p.data[rankKey+":"+pod]; rankKey != "" && hasSamples(u) {
			r.limited, r.peak = true, peakOf([]series{{values: u}})
		}
		all = append(all, r)
	}
	sort.Slice(all, func(i, j int) bool {
		a, b := all[i], all[j]
		switch {
		case a.limited != b.limited:
			return a.limited
		case a.peak != b.peak:
			return a.peak > b.peak
		case a.mean != b.mean:
			return a.mean > b.mean
		}
		return a.name < b.name
	})
	// Without --cluster a pod's name carries its cluster: unsaid when every pod is in one
	clusters := map[string]bool{}
	for _, r := range all {
		if c, _, ok := strings.Cut(r.name, "/"); ok {
			clusters[c] = true
		}
	}
	shown := all[:min(len(all), maxPods)]
	// Colours by name among the pods shown, not by rank: they stay apart, and a pod keeps its colour when the
	// ranking changes between refreshes, as long as the same pods are shown
	names := make([]string, len(shown))
	for i, r := range shown {
		names[i] = r.name
	}
	sort.Strings(names)
	var out []series
	for _, r := range shown {
		name := r.name
		if _, pod, ok := strings.Cut(name, "/"); ok && len(clusters) == 1 {
			name = pod
		}
		out = append(out, series{name, r.values, p.theme.series[sort.SearchStrings(names, r.name)%len(p.theme.series)]})
	}
	return out, len(all)
}

func mean(values []*float64) float64 {
	total, n := 0.0, 0
	for _, x := range values {
		if x != nil {
			total, n = total+*x, n+1
		}
	}
	if n == 0 {
		return 0
	}
	return total / float64(n)
}

// A per-pod card, its title saying when it shows only the busiest pods
func (p *page) podCard(title, key, rankKey, unit string, width, rows int) block {
	ss, n := p.podSeries(key, rankKey)
	if n > len(ss) {
		title += fmt.Sprintf(" · busiest %d of %d", len(ss), n)
	}
	return p.cardOf(title, unit, ss, width, rows, false)
}

// Whether the target served any request in the window. Under min the requests can be all zeros while other pods
// serve, but the error rate, all the pods' errors over their requests, has a sample wherever any served one
func (p *page) served() bool {
	return active(p.data["requests"]) || hasSamples(p.data["errorRate"])
}

// Whether a series has a sample anywhere in the window
func hasSamples(values []*float64) bool { return first(values) != nil }

// Whether a count or rate is above zero anywhere in the window. Dash0 answers sum() over no series with zeros, where
// Prometheus answers nothing, so zeros alone mean the pod has none
func active(values []*float64) bool {
	for _, v := range values {
		if v != nil && *v > 0 {
			return true
		}
	}
	return false
}

func sum(values []*float64) float64 {
	total := 0.0
	for _, v := range values {
		if v != nil {
			total += *v
		}
	}
	return total
}

// The section's heading, bold over a dim line saying what it counts (Heading, level 'heading')
func heading(title, note string) block {
	return block{line{{title, style{bold: true}}, {"  " + note, dimStyle}}}
}

// Requests, errors and duration of the spans the pod serves: nil when it served none in the window
func (p *page) redSection(cardW int, side bool, rows int) block {
	if !p.served() {
		return nil
	}
	requests, errors := p.data["requests"], p.data["errors"]
	since := "vs " + clock(p.win.start)
	errTitle := "Errors"
	if p.aggNote() != "" {
		errTitle += " · all pods"
	}
	stats := hjoin(6,
		stat("Requests"+p.aggNote(), requests, "/s", "", since, "", true, p.theme),
		stat(errTitle, p.data["errorRate"], "percent", "", since, "", false, p.theme),
		stat("Duration p95", p.data["p95"], "seconds", "", since, "", false, p.theme),
	)

	var req, dur block
	if p.perPod {
		// One axis for every pod: comparing them is the point
		req = p.podCard("Requests by pod", "requestsPods", "", "/s", cardW, rows)
		dur = p.podCard("Duration p95 by pod", "p95Pods", "", "seconds", cardW, rows)
	} else {
		var reqSeries []series
		reqSeries = append(reqSeries, series{"requests", requests, p.theme.series[0]})
		if active(errors) {
			// ERROR takes the error red wherever it sits, as Ditt0's color manager colours it by meaning
			reqSeries = append(reqSeries, series{"errors", errors, p.theme.errorInk})
		}
		var durSeries []series
		for i, q := range []string{"p99", "p95", "p50"} {
			if v := p.data[q]; hasSamples(v) {
				durSeries = append(durSeries, series{q, v, p.theme.series[i]})
			}
		}
		req = p.cardOf("Requests"+p.aggNote(), "/s", reqSeries, cardW, rows, true)
		dur = p.cardOf("Duration", "seconds", durSeries, cardW, rows, true)
	}
	charts := vjoin(1, req, dur)
	if side {
		charts = hjoin(1, req, dur)
	}
	return vjoin(1, heading("Requests", "SERVER spans"), stats, charts)
}

// The severities in the order a log chart lists them, worst first, each in its status colour
func (p *page) severities() []struct{ name, color string } {
	t := p.theme
	return []struct{ name, color string }{
		{"FATAL", t.errorInk}, {"ERROR", t.errorInk}, {"WARN", t.warningInk}, {"INFO", t.infoInk},
		{"DEBUG", t.debugInk}, {"TRACE", t.debugInk}, {"UNKNOWN", t.unknownInk},
	}
}

// Log record counts by severity: nil when the pod wrote none in the window
func (p *page) logsSection(width, rows int) block {
	var ss []series
	var total, errs, warns []*float64
	for _, sev := range p.severities() {
		v, ok := p.data["logs:"+sev.name]
		if !ok || !active(v) {
			continue
		}
		ss = append(ss, series{sev.name, v, sev.color})
		total = addSeries(total, v)
		switch sev.name {
		case "FATAL", "ERROR":
			errs = addSeries(errs, v)
		case "WARN":
			warns = addSeries(warns, v)
		}
	}
	if len(ss) == 0 {
		return nil
	}
	span := ranges[p.rangeIdx].label
	count := func(label string, v []*float64, ink string) block {
		n := sum(v)
		out := block{text(label, dimStyle)}
		if n > 0 || v != nil {
			txt := formatValue(&n, "")
			st := style{bold: true}
			if n > 0 && ink != "" {
				st.fg = ink
			}
			out = append(out, text(txt, st))
		} else {
			out = append(out, text("0", style{bold: true, dim: true}))
		}
		if active(v) {
			out = append(out, text(sparkline(v, 20), style{fg: ink, dim: ink == ""}))
		} else {
			// As wide as a sparkline, so the stats beside it keep their places
			out = append(out, text(strings.Repeat(" ", 20), style{}))
		}
		return out
	}
	stats := hjoin(6,
		count("Log records · "+span, total, ""),
		count("Errors", errs, p.theme.errorInk),
		count("Warnings", warns, p.theme.warningInk),
	)
	chart := p.cardOf("Log records per "+stepLabel(p.win.step), "", ss, width, rows, true)
	return vjoin(1, heading("Logs", "records by severity"), stats, chart)
}

// Series added step by step; a step none of them has stays none
func addSeries(a, b []*float64) []*float64 {
	if a == nil {
		a = make([]*float64, len(b))
	}
	for i := range b {
		if b[i] == nil {
			continue
		}
		v := *b[i]
		if a[i] != nil {
			v += *a[i]
		}
		a[i] = &v
	}
	return a
}

func stepLabel(step time.Duration) string {
	if step >= time.Hour {
		return fmt.Sprintf("%d h", int(step.Hours()))
	}
	if step == time.Minute {
		return "minute"
	}
	return fmt.Sprintf("%d min", int(step.Minutes()))
}
