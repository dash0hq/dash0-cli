package pods

import (
	"fmt"
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
	sel := []string{"otel_metric_name=" + quote(metric), "k8s_namespace_name=" + quote(t.namespace), "k8s_pod_name=" + quote(t.pod)}
	if t.cluster != "" {
		sel = append(sel, "k8s_cluster_name="+quote(t.cluster))
	}
	return "{" + strings.Join(append(sel, extra...), ", ") + "}"
}

// sum() of a gauge: the pod's usage, and its containers' requests and limits (the k8s_cluster receiver reports them
// per container)
func (t target) query(metric string) string { return "sum(" + t.selector(metric) + ")" }

func gauge(metric string) func(target, time.Duration) string {
	return func(t target, _ time.Duration) string { return t.query(metric) }
}

func rangeOf(step time.Duration) string { return fmt.Sprintf("[%ds]", int(step.Seconds())) }

// RED from the spans the pod serves, as Ditt0's Services Overview asks them (connectors/dash0/overview.js): SERVER
// spans only, a rate over the step, the duration from dash0.spans.duration's histogram
func spans(t target, extra ...string) string {
	return t.selector("dash0.spans", append([]string{`otel_span_kind="SERVER"`}, extra...)...)
}

func quantile(q string) func(target, time.Duration) string {
	return func(t target, step time.Duration) string {
		return "histogram_quantile(" + q + ", sum by (le) (rate(" + t.selector("dash0.spans.duration", `otel_span_kind="SERVER"`) + rangeOf(step) + ")))"
	}
}

var panels = []panel{
	{"cpu", gauge("k8s.pod.cpu.usage"), ""}, {"cpuRequest", gauge("k8s.container.cpu_request"), ""}, {"cpuLimit", gauge("k8s.container.cpu_limit"), ""},
	{"mem", gauge("k8s.pod.memory.working_set"), ""}, {"memRequest", gauge("k8s.container.memory_request"), ""}, {"memLimit", gauge("k8s.container.memory_limit"), ""},
	{"requests", func(t target, step time.Duration) string { return "sum(rate(" + spans(t) + rangeOf(step) + "))" }, ""},
	{"errors", func(t target, step time.Duration) string {
		return "sum(rate(" + spans(t, `otel_span_status_code="ERROR"`) + rangeOf(step) + "))"
	}, ""},
	{"p50", quantile("0.5"), ""}, {"p95", quantile("0.95"), ""}, {"p99", quantile("0.99"), ""},
	// Log records per step, by severity
	{"logs", func(t target, step time.Duration) string {
		return "sum by (otel_log_severity_range) (increase(" + t.selector("dash0.logs") + rangeOf(step) + "))"
	}, "otel_log_severity_range"},
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
	requests := p.data["requests"]
	if !active(requests) {
		return nil
	}
	errors := p.data["errors"]
	since := "vs " + clock(p.win.start)
	rate := errorRate(requests, errors)
	stats := hjoin(6,
		stat("Requests", requests, "/s", "", since, "", true, p.theme),
		stat("Errors", rate, "percent", "", since, "", false, p.theme),
		stat("Duration p95", p.data["p95"], "seconds", "", since, "", false, p.theme),
	)

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
	req := p.cardOf("Requests", "/s", reqSeries, cardW, rows, true)
	dur := p.cardOf("Duration", "seconds", durSeries, cardW, rows, true)
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
