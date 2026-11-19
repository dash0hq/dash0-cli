package pods

import (
	"context"
	"fmt"
	"github.com/dash0hq/ditt0-cli-design-system/go/ditt0"
	"strconv"
	"strings"
	"sync"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

const refreshEvery = 30 * time.Second

// The ranges Ditt0's live pages offer, by key
var ranges = []struct {
	label string
	d     time.Duration
}{{"15m", 15 * time.Minute}, {"1h", time.Hour}, {"6h", 6 * time.Hour}, {"24h", 24 * time.Hour}}

// Steps a window can take: the smallest that fits the plot's columns
var steps = []time.Duration{time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour}

// kind: what name names, as Dash0's resource attribute k8s.<kind>.name has it: pod, deployment, statefulset, ...
// agg: how a workload's pods add up, one of aggs
type target struct{ cluster, namespace, kind, name, context, profile, agg string }

// A single pod, not a workload's pods
func (t target) isPod() bool { return t.kind == "pod" }

// The pods added up, not their mean, least or most
func (t target) isTotal() bool { return t.agg == "" || t.agg == "total" }

func quote(v string) string { return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v) + `"` }

type page struct {
	mu        sync.Mutex
	target    target
	rangeIdx  int
	win       window
	data      map[string][]*float64
	failed    []string
	lastError string
	at        time.Time
	loading   bool
	gen       int
	theme     ditt0.Theme
	scroll    int
	perPod    bool
	apiURL    string
	dataset   *string
}

// The window for a plot columns wide: ending on the last whole step, a step back, as Ditt0's dashboards do
// (live-page windowOf), so the newest sample isn't still filling in
func windowFor(d time.Duration, columns int, now time.Time) window {
	step := steps[len(steps)-1]
	for _, s := range steps {
		if int(d/s) <= max(columns, 1) {
			step = s
			break
		}
	}
	end := now.Truncate(step).Add(-step)
	return window{end.Add(-d), end, step}
}

func (p *page) fetch(ctx context.Context, c promClient, columns int) {
	p.mu.Lock()
	p.gen++
	gen := p.gen
	win := windowFor(ranges[p.rangeIdx].d, columns, time.Now())
	t := p.target
	panels := panelsFor(t, p.perPod)
	p.loading = true
	p.mu.Unlock()

	type answer struct {
		pn     panel
		values map[string][]*float64
		err    error
	}
	answers := make(chan answer, len(panels))
	queryCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for _, pn := range panels {
		go func(pn panel) {
			v, err := c.queryRange(queryCtx, pn.query(t, win.step), win, pn.by)
			answers <- answer{pn, v, err}
		}(pn)
	}
	got := map[string]map[string][]*float64{}
	var failed []string
	var lastErr string
	for range panels {
		a := <-answers
		if a.err != nil {
			failed, lastErr = append(failed, a.pn.key), a.err.Error()
		} else {
			got[a.pn.key] = a.values
		}
	}

	// A newer refresh cancelled this one: its errors say nothing about Dash0
	if ctx.Err() != nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	// A range changed while asking: its answers are dropped, a range never shows another's data
	if gen != p.gen {
		return
	}
	p.loading = false
	p.failed, p.lastError = failed, lastErr
	if len(got) == 0 {
		return
	}
	// A panel whose query failed keeps what it had, when it is the same window's length
	if p.data == nil || len(p.win.times()) != len(win.times()) {
		p.data = map[string][]*float64{}
	}
	for key, split := range got {
		// A split panel's series are replaced whole: a severity gone from the window goes from the chart
		for k := range p.data {
			if strings.HasPrefix(k, key+":") {
				delete(p.data, k)
			}
		}
		for label, v := range split {
			if label == "" {
				p.data[key] = v
			} else {
				p.data[key+":"+label] = v
			}
		}
	}
	p.win, p.at = win, time.Now()
}

// What the freshness line says (live-parts.jsx freshnessOf)
func (p *page) freshness() ditt0.Line {
	if p.at.IsZero() {
		if len(p.failed) > 0 {
			return ditt0.Line{{Text: "Couldn't reach Dash0. Trying again in 30 seconds.", Style: ditt0.Style{Fg: p.theme.Error.Inline}}, {Text: "  " + p.lastError, Style: ditt0.DimStyle}}
		}
		return ditt0.Text("◌ Loading from Dash0…", ditt0.DimStyle)
	}
	at := p.at.Local().Format("15:04:05") + " (" + zoneOf(p.at) + ")"
	if len(p.failed) > 0 {
		return ditt0.Text(fmt.Sprintf("Updated %s · %d of %d panels show older data", at, len(p.failed), len(panelsFor(p.target, p.perPod))), ditt0.DimStyle)
	}
	return ditt0.Text("Updated "+at, ditt0.DimStyle)
}

func (p *page) metricsLink(query string) string {
	return dash0api.MetricsQueryBuilderURL(p.apiURL, query, "", "", p.dataset)
}

// The traces and logs explorers' filter: the pod's resource, over the page's range
func (p *page) explorerFilter() []dash0api.DeeplinkFilter {
	filter := []dash0api.DeeplinkFilter{{Key: "k8s.namespace.name", Operator: "is", Value: p.target.namespace}, {Key: "k8s." + p.target.kind + ".name", Operator: "is", Value: p.target.name}}
	if p.target.cluster != "" {
		filter = append(filter, dash0api.DeeplinkFilter{Key: "k8s.cluster.name", Operator: "is", Value: p.target.cluster})
	}
	return filter
}

type dash0Link struct{ key, label, url string }

// Metrics always; traces and logs once their sections show. Call with p.mu held.
func (p *page) links() []dash0Link {
	links := []dash0Link{{"o", "Open metrics", p.metricsLink(p.target.query("k8s.pod.memory.working_set"))}}
	if p.data == nil {
		return links
	}
	from := "now-" + ranges[p.rangeIdx].label
	if p.served() {
		links = append(links, dash0Link{"t", "Open traces", dash0api.SpansExplorerURL(p.apiURL, p.explorerFilter(), from, "now", p.dataset)})
	}
	for _, sev := range p.severities() {
		if active(p.data["logs:"+sev.name]) {
			links = append(links, dash0Link{"l", "Open logs", dash0api.LogsExplorerURL(p.apiURL, p.explorerFilter(), from, "now", p.dataset)})
			break
		}
	}
	return links
}

// A row of bordered toggles, the selected one's border in the accent colour, the others dim (ChartControls)
func (p *page) toggles() ditt0.Block {
	var parts []ditt0.Block
	for i, r := range ranges {
		st, label := ditt0.Style{Fg: p.theme.Selected}, ditt0.Style{}
		if i != p.rangeIdx {
			st, label = ditt0.DimStyle, ditt0.DimStyle
		}
		parts = append(parts, ditt0.Border(ditt0.Block{ditt0.Text(r.label, label)}, len(r.label)+4, 1, st))
	}
	// A workload's views: how its pods add up, then a line per pod
	if !p.target.isPod() {
		toggle := func(m string, on bool) {
			st, label := ditt0.Style{Fg: p.theme.Selected}, ditt0.Style{}
			if !on {
				st, label = ditt0.DimStyle, ditt0.DimStyle
			}
			parts = append(parts, ditt0.Border(ditt0.Block{ditt0.Text(m, label)}, len(m)+4, 1, st))
		}
		parts = append(parts, ditt0.Block{ditt0.Text("  ", ditt0.Style{})})
		for _, m := range aggs {
			toggle(m, m == p.target.agg)
		}
		parts = append(parts, ditt0.Block{ditt0.Text("  ", ditt0.Style{})})
		toggle("per pod", p.perPod)
	}
	return ditt0.Row(0, parts...)
}

// Tags: each key dim, its value at full strength (Tag)
func (p *page) tags() ditt0.Line {
	var l ditt0.Line
	add := func(k, v string) {
		if v == "" {
			return
		}
		if len(l) > 0 {
			l = append(l, ditt0.Segment{Text: "   ", Style: ditt0.Style{}})
		}
		l = append(l, ditt0.Segment{Text: k + " ", Style: ditt0.DimStyle}, ditt0.Segment{Text: v, Style: ditt0.Style{}})
	}
	add("profile", p.target.profile)
	add("cluster", p.target.cluster)
	add("context", p.target.context)
	add("namespace", p.target.namespace)
	add(p.target.kind, p.target.name)
	if n := last(p.data["pods"]); !p.target.isPod() && n != nil {
		add("pods", strconv.Itoa(int(*n)))
	}
	return l
}

func ratio(v, of *float64) (float64, bool) {
	if v == nil || of == nil || *of == 0 {
		return 0, false
	}
	return *v / *of, true
}

// The usage against its request and limit, and the status colour of util, the highest share of its limit any pod
// uses: Dash0's thresholds, past 80 percent needs attention and past 90 is critical
func (p *page) against(usage, request, limit, util []*float64, unit string) (string, string) {
	u, r, l, f := last(usage), last(request), last(limit), last(util)
	// A zero limit is none, as Kubernetes has it
	if l != nil && *l == 0 {
		l = nil
	}
	var parts []string
	// Under avg, min or max the usage and the request can be different pods'
	if x, ok := ratio(u, r); ok && p.target.isTotal() {
		parts = append(parts, fmt.Sprintf("%.0f%% of %s requested", x*100, ditt0.FormatValue(r, unit)))
	}
	switch {
	case f == nil || (l == nil && p.target.isPod()):
		return strings.Join(append(parts, "no limit"), " · "), ""
	case p.target.isPod():
		parts = append(parts, fmt.Sprintf("%.0f%% of %s limit", *f*100, ditt0.FormatValue(l, unit)))
	case l == nil:
		// Some pods have no limit, so the workload has none, but those that do can still run out
		parts = append(parts, "not every pod has a limit", fmt.Sprintf("busiest limited pod at %.0f%% of its limit", *f*100))
	default:
		parts = append(parts, fmt.Sprintf("busiest pod at %.0f%% of its limit", *f*100))
	}
	ink := ""
	if *f > 0.9 {
		ink = p.theme.Error.Inline
	} else if *f > 0.8 {
		ink = p.theme.Warning.Inline
	}
	return strings.Join(parts, " · "), ink
}

// The chart's series: usage first, so where it crosses a request or limit line its own stroke stays whole
func (p *page) chartSeries(keys [3]string) []series {
	names := [3]string{"usage", "request", "limit"}
	var out []series
	for i, k := range keys {
		v := p.data[k]
		if first(v) == nil {
			continue
		}
		out = append(out, series{Name: names[i], Values: v, Color: p.theme.ManagedSeriesColor(i)})
	}
	return out
}

func (p *page) card(title, unit string, ss []series, width, rows int) ditt0.Block {
	return p.cardOf(title, unit, ss, width, rows, oneAxis)
}

// multiples: series that would flatten on one axis draw as small multiples (chart)
func (p *page) cardOf(title, unit string, ss []series, width, rows int, a axes) ditt0.Block {
	var body ditt0.Block
	switch {
	case p.data == nil && len(p.failed) > 0:
		body = ditt0.Block{ditt0.Text("Couldn't load "+strings.ToLower(title), ditt0.Style{Fg: p.theme.Error.Inline}), ditt0.Text("Dash0 didn't answer. The page asks again on the next refresh.", ditt0.DimStyle)}
	case p.data == nil:
		body = ditt0.Block{ditt0.Text("◌ Loading "+strings.ToLower(title)+"…", ditt0.DimStyle)}
	case len(ss) == 0:
		body = ditt0.Block{ditt0.Text("No data in this time range.", ditt0.DimStyle)}
	default:
		body = chart(ss, p.win.times(), unit, width-4, rows, a)
	}
	return box(title, body, width)
}

// A card: its title in bold over its body, in a dim border width cells wide
func box(title string, body ditt0.Block, width int) ditt0.Block {
	return ditt0.Border(ditt0.Column(1, ditt0.Block{ditt0.Text(title, ditt0.Style{Bold: true})}, body), width, 1, ditt0.DimStyle)
}

// layout: the page for a terminal w × h, and the plot columns its charts get
func (p *page) render(w, h int) (ditt0.Block, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	inner := w - 2
	side := inner >= 100
	cardW := inner
	if side {
		cardW = (inner - 1) / 2
	}
	header := ditt0.Block{
		ditt0.Text(p.target.namespace+"/"+p.target.name, ditt0.Style{Bold: true}),
		p.tags(),
		p.freshness(),
	}

	// The CTAs: Dash0's purple fill, each label in the CTA ink (Link, variant 'cta'), top right beside the header,
	// else beside the range toggles, else under them
	edge := ditt0.Style{Fg: p.theme.CTA.Bg}
	var buttons []ditt0.Block
	for _, l := range p.links() {
		label := " " + l.label + " "
		buttons = append(buttons, ditt0.Block{
			ditt0.Text("╭"+strings.Repeat("─", len(label))+"╮", edge),
			ditt0.Line{{Text: "│", Style: edge}, {Text: label, Style: ditt0.Style{Fg: p.theme.CTA.Text, Bg: p.theme.CTA.Bg, Bold: true, Link: l.url}}, {Text: "│", Style: edge}},
			ditt0.Text("╰"+strings.Repeat("─", len(label))+"╯", edge),
		})
	}
	cta := ditt0.Row(1, buttons...)
	toggles := p.toggles()
	switch {
	case inner-header.Width()-cta.Width() >= 2:
		header = ditt0.Row(inner-header.Width()-cta.Width(), header, cta)
	case inner-toggles.Width()-cta.Width() >= 2:
		toggles = ditt0.Row(inner-toggles.Width()-cta.Width(), toggles, cta)
	default:
		toggles = ditt0.Column(0, toggles, cta)
	}
	// header 3, toggles 3, stats 7, hints 1, the gaps between them 3; each card's chrome 9
	cards := 1
	if !side {
		cards = 2
	}
	rows := max(5, min(12, (h-17-9*cards)/cards))

	var stats ditt0.Block
	if p.data != nil {
		d := p.data
		cpuNote, cpuInk := p.against(d["cpu"], d["cpuRequest"], d["cpuLimit"], d["cpuUtil"], "cores")
		memNote, memInk := p.against(d["mem"], d["memRequest"], d["memLimit"], d["memUtil"], "bytes")
		since := "vs " + clock(p.win.start)
		stats = ditt0.Row(6,
			stat("CPU"+p.aggNote(), d["cpu"], "cores", cpuInk, since, cpuNote, false, p.theme),
			stat("Memory"+p.aggNote(), d["mem"], "bytes", memInk, since, memNote, false, p.theme),
		)
	}

	cpu := p.card("CPU"+p.aggNote(), "cores", p.chartSeries([3]string{"cpu", "cpuRequest", "cpuLimit"}), cardW, rows)
	mem := p.card("Memory"+p.aggNote(), "bytes", p.chartSeries([3]string{"mem", "memRequest", "memLimit"}), cardW, rows)
	if p.perPod {
		cpu = p.podCard("CPU by pod", "cpuPods", "cpuUtilPods", "cores", cardW, rows)
		mem = p.podCard("Memory by pod", "memPods", "memUtilPods", "bytes", cardW, rows)
	}
	charts := ditt0.Column(1, cpu, mem)
	if side {
		charts = ditt0.Row(1, cpu, mem)
	}

	var red, logs ditt0.Block
	if p.data != nil {
		red = p.redSection(inner, side, rows)
		logs = p.logsSection(inner, rows)
	}

	page := ditt0.Column(1, header, toggles, stats, charts, red, logs)
	// The window's samples fit the narrowest chart: side by side, the Requests row's third of the page
	chartW := cardW
	if side {
		chartW = (inner - 2) / 3
	}
	plotColumns := chartW - 4 - 8 - rightGap
	return ditt0.Indent(1, page), plotColumns
}
