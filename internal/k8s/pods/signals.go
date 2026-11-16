package pods

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// What the page asks Dash0 for. A panel's query depends on the window's step, its rate window; by splits its answer
// into a series per value of that label, kept as <key>:<value>.
type panel struct {
	key   string
	query func(t target, step time.Duration) string
	by    string
}

func (t target) selector(metric string, extra ...string) string {
	sel := []string{"otel_metric_name=" + quote(metric), "k8s_namespace_name=" + quote(t.namespace), "k8s_" + t.kind + "_name=" + quote(t.name)}
	if t.cluster != "" {
		sel = append(sel, "k8s_cluster_name="+quote(t.cluster))
	}
	return "{" + strings.Join(append(sel, extra...), ", ") + "}"
}

// A gauge once per pod, or per pod and container for the containers' requests and limits (the k8s_cluster receiver
// reports them per container). A pod whose resource attributes change mid-life has two series for a while, the same
// value under two dash0_resource_hash; summing them would count it twice. The cluster stays in the grouping, because
// without --cluster the selector matches a pod of the same namespace and name in every cluster, and those are summed
func (t target) once(metric string) string {
	by := "k8s_cluster_name, k8s_namespace_name, k8s_pod_name"
	if strings.HasPrefix(metric, "k8s.container.") {
		by += ", k8s_container_name"
	}
	return "max by (" + by + ") (" + t.selector(metric) + ")"
}

// sum() of a gauge, each pod and container counted once
func (t target) query(metric string) string { return "sum(" + t.once(metric) + ")" }

func gauge(metric string) func(target, time.Duration) string {
	return func(t target, _ time.Duration) string { return t.query(metric) }
}

func rangeOf(step time.Duration) string { return fmt.Sprintf("[%ds]", int(step.Seconds())) }

// RED from the spans the pod serves, as Ditt0's Services Overview asks them (connectors/dash0/overview.js): SERVER
// spans only, a rate over the step, the duration from dash0.spans.duration's histogram
func spans(t target, extra ...string) string {
	return t.selector("dash0.spans", append([]string{`otel_span_kind="SERVER"`}, extra...)...)
}

func quantile(q, by string) func(target, time.Duration) string {
	return func(t target, step time.Duration) string {
		return "histogram_quantile(" + q + ", sum by (le" + by + ") (rate(" + t.selector("dash0.spans.duration", `otel_span_kind="SERVER"`) + rangeOf(step) + ")))"
	}
}

var panels = []panel{
	{"cpu", gauge("k8s.pod.cpu.usage"), ""}, {"cpuRequest", gauge("k8s.container.cpu_request"), ""}, {"cpuLimit", gauge("k8s.container.cpu_limit"), ""},
	{"mem", gauge("k8s.pod.memory.working_set"), ""}, {"memRequest", gauge("k8s.container.memory_request"), ""}, {"memLimit", gauge("k8s.container.memory_limit"), ""},
	{"requests", func(t target, step time.Duration) string { return "sum(rate(" + spans(t) + rangeOf(step) + "))" }, ""},
	{"errors", func(t target, step time.Duration) string {
		return "sum(rate(" + spans(t, `otel_span_status_code="ERROR"`) + rangeOf(step) + "))"
	}, ""},
	{"p50", quantile("0.5", ""), ""}, {"p95", quantile("0.95", ""), ""}, {"p99", quantile("0.99", ""), ""},
	// Log records per step, by severity
	{"logs", func(t target, step time.Duration) string {
		return "sum by (otel_log_severity_range) (increase(" + t.selector("dash0.logs") + rangeOf(step) + "))"
	}, "otel_log_severity_range"},
}

// A workload's panels split by pod as well, for the per-pod view and the avg, min and max views
var podPanels = []panel{
	{"cpuPods", byPod("k8s.pod.cpu.usage"), "k8s_pod_name"},
	{"memPods", byPod("k8s.pod.memory.working_set"), "k8s_pod_name"},
	{"requestsPods", func(t target, step time.Duration) string {
		return "sum by (k8s_pod_name) (rate(" + spans(t) + rangeOf(step) + "))"
	}, "k8s_pod_name"},
	{"p95Pods", quantile("0.95", ", k8s_pod_name"), "k8s_pod_name"},
	{"cpuRequestPods", byPod("k8s.container.cpu_request"), "k8s_pod_name"},
	{"cpuLimitPods", byPod("k8s.container.cpu_limit"), "k8s_pod_name"},
	{"memRequestPods", byPod("k8s.container.memory_request"), "k8s_pod_name"},
	{"memLimitPods", byPod("k8s.container.memory_limit"), "k8s_pod_name"},
	{"errorsPods", func(t target, step time.Duration) string {
		return "sum by (k8s_pod_name) (rate(" + spans(t, `otel_span_status_code="ERROR"`) + rangeOf(step) + "))"
	}, "k8s_pod_name"},
}

func byPod(metric string) func(target, time.Duration) string {
	return func(t target, _ time.Duration) string { return "sum by (k8s_pod_name) (" + t.selector(metric) + ")" }
}

// The panels a target asks for: a pod's, and a workload's per-pod ones too
func panelsFor(t target) []panel {
	if t.kind == "pod" {
		return panels
	}
	return append(append([]panel{}, panels...), podPanels...)
}

// How a workload's pods add up in its stats and charts: their total, or the mean, least or most of the pods running
// at each step
var aggs = []string{"total", "avg", "min", "max"}

// key's series under the page's agg: the total as Dash0 sums it, else worked out step by step from key's per-pod
// series (<key>Pods), so switching asks Dash0 nothing
func (p *page) of(key string) []*float64 {
	if p.agg == 0 {
		return p.data[key]
	}
	var pods [][]*float64
	for k, v := range p.data {
		if strings.HasPrefix(k, key+"Pods:") {
			pods = append(pods, v)
		}
	}
	return aggregate(pods, aggs[p.agg])
}

// Series combined step by step over those with a sample there: avg, min or max; a step none has stays none
func aggregate(pods [][]*float64, how string) []*float64 {
	if len(pods) == 0 {
		return nil
	}
	out := make([]*float64, len(pods[0]))
	for i := range out {
		var got []float64
		for _, v := range pods {
			if i < len(v) && v[i] != nil {
				got = append(got, *v[i])
			}
		}
		if len(got) == 0 {
			continue
		}
		acc := got[0]
		for _, x := range got[1:] {
			switch how {
			case "avg":
				acc += x
			case "min":
				acc = math.Min(acc, x)
			case "max":
				acc = math.Max(acc, x)
			}
		}
		if how == "avg" {
			acc /= float64(len(got))
		}
		out[i] = &acc
	}
	return out
}

// " · max pod" after a title under avg, min or max
func (p *page) aggNote() string {
	if p.agg == 0 {
		return ""
	}
	return " · " + aggs[p.agg] + " pod"
}

// Pods drawn in a per-pod chart. Folding the rest into one line would dwarf them: a deployment that rolled churns
// through dozens of pods in an hour
const maxPods = 8

// key's per-pod series, busiest first by mean, and how many pods had any
func (p *page) podSeries(key string) ([]series, int) {
	type ranked struct {
		name   string
		values []*float64
		mean   float64
	}
	var all []ranked
	for k, v := range p.data {
		pod, ok := strings.CutPrefix(k, key+":")
		if !ok || !hasSamples(v) {
			continue
		}
		total, n := 0.0, 0
		for _, x := range v {
			if x != nil {
				total, n = total+*x, n+1
			}
		}
		all = append(all, ranked{pod, v, total / float64(n)})
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].mean > all[j].mean || all[i].mean == all[j].mean && all[i].name < all[j].name
	})
	var out []series
	for i, r := range all[:min(len(all), maxPods)] {
		out = append(out, series{r.name, r.values, p.theme.series[i%len(p.theme.series)]})
	}
	return out, len(all)
}

// A per-pod card, its title saying when it shows only the busiest pods
func (p *page) podCard(title, key, unit string, width, rows int) block {
	ss, n := p.podSeries(key)
	if n > len(ss) {
		title += fmt.Sprintf(" · busiest %d of %d", len(ss), n)
	}
	return p.cardOf(title, unit, ss, width, rows, false)
}

// Pods with a CPU sample at the window's end: those running now
func (p *page) runningPods() int {
	n := 0
	for k, v := range p.data {
		if strings.HasPrefix(k, "cpuPods:") && len(v) > 0 && v[len(v)-1] != nil {
			n++
		}
	}
	return n
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

// The error rate per step, in percent of the requests; none where there were no requests
func errorRate(requests, errors []*float64) []*float64 {
	out := make([]*float64, len(requests))
	for i, r := range requests {
		if r == nil || *r == 0 {
			continue
		}
		e := 0.0
		if i < len(errors) && errors[i] != nil {
			e = *errors[i]
		}
		v := 100 * e / *r
		out[i] = &v
	}
	return out
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
	if !active(p.data["requests"]) {
		return nil
	}
	requests, errors := p.of("requests"), p.of("errors")
	since := "vs " + clock(p.win.start)
	// The share of all requests: an avg, min or max pod's errors over another pod's requests would mean nothing
	rate := errorRate(p.data["requests"], p.data["errors"])
	stats := hjoin(6,
		stat("Requests"+p.aggNote(), requests, "/s", "", since, "", true, p.theme),
		stat("Errors", rate, "percent", "", since, "", false, p.theme),
		stat("Duration p95", p.data["p95"], "seconds", "", since, "", false, p.theme),
	)

	var req, dur block
	if p.perPod {
		// One axis for every pod: comparing them is the point
		req = p.podCard("Requests by pod", "requestsPods", "/s", cardW, rows)
		dur = p.podCard("Duration p95 by pod", "p95Pods", "seconds", cardW, rows)
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
