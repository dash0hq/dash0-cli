package pods

import (
	"context"
	"fmt"
	"net/url"
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

type target struct{ cluster, namespace, pod, context, profile string }

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
	theme     theme
	scroll    int
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

func (p *page) fetch(c promClient, columns int) {
	p.mu.Lock()
	p.gen++
	gen := p.gen
	win := windowFor(ranges[p.rangeIdx].d, columns, time.Now())
	t := p.target
	p.loading = true
	p.mu.Unlock()

	type answer struct {
		pn     panel
		values map[string][]*float64
		err    error
	}
	answers := make(chan answer, len(panels))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, pn := range panels {
		go func(pn panel) {
			v, err := c.queryRange(ctx, pn.query(t, win.step), win, pn.by)
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
func (p *page) freshness() line {
	if p.at.IsZero() {
		if len(p.failed) > 0 {
			return line{{"Couldn't reach Dash0. Trying again in 30 seconds.", style{fg: p.theme.errorInk}}, {"  " + p.lastError, dimStyle}}
		}
		return text("◌ Loading from Dash0…", dimStyle)
	}
	at := p.at.Local().Format("15:04:05") + " (" + zoneOf(p.at) + ")"
	if len(p.failed) > 0 {
		return text(fmt.Sprintf("Updated %s · %d of %d panels show older data", at, len(p.failed), len(panels)), dimStyle)
	}
	return text("Updated "+at, dimStyle)
}

// The metrics query builder, under the dataset when set
func (p *page) metricsLink(query string) string {
	q := url.Values{"query": {query}}
	if p.dataset != nil && *p.dataset != "" {
		q.Set("dataset", *p.dataset)
	}
	return dash0api.AppBaseURL(p.apiURL) + "/goto/metrics/query-builder?" + q.Encode()
}

// The traces and logs explorers' filter: the pod's resource, over the page's range
func (p *page) explorerFilter() []dash0api.DeeplinkFilter {
	filter := []dash0api.DeeplinkFilter{{Key: "k8s.namespace.name", Operator: "is", Value: p.target.namespace}, {Key: "k8s.pod.name", Operator: "is", Value: p.target.pod}}
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
	if active(p.data["requests"]) {
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
func (p *page) toggles() block {
	var parts []block
	for i, r := range ranges {
		st, label := style{fg: p.theme.selected}, style{}
		if i != p.rangeIdx {
			st, label = dimStyle, dimStyle
		}
		parts = append(parts, border(block{text(r.label, label)}, len(r.label)+4, 1, st))
	}
	return hjoin(0, parts...)
}

// Tags: each key dim, its value at full strength (Tag)
func (p *page) tags() line {
	var l line
	add := func(k, v string) {
		if v == "" {
			return
		}
		if len(l) > 0 {
			l = append(l, seg{"   ", style{}})
		}
		l = append(l, seg{k + " ", dimStyle}, seg{v, style{}})
	}
	add("profile", p.target.profile)
	add("cluster", p.target.cluster)
	add("context", p.target.context)
	add("namespace", p.target.namespace)
	add("pod", p.target.pod)
	return l
}

func ratio(v, of *float64) (float64, bool) {
	if v == nil || of == nil || *of == 0 {
		return 0, false
	}
	return *v / *of, true
}

// The usage against its request and limit, and the status colour it reaches against the limit
func (p *page) against(usage, request, limit []*float64, unit string) (string, string) {
	u, r, l := last(usage), last(request), last(limit)
	var parts []string
	if f, ok := ratio(u, r); ok {
		parts = append(parts, fmt.Sprintf("%.0f%% of %s requested", f*100, formatValue(r, unit)))
	}
	ink := ""
	if f, ok := ratio(u, l); ok {
		parts = append(parts, fmt.Sprintf("%.0f%% of %s limit", f*100, formatValue(l, unit)))
		if f >= 0.95 {
			ink = p.theme.errorInk
		} else if f >= 0.8 {
			ink = p.theme.warningInk
		}
	} else {
		parts = append(parts, "no limit")
	}
	return strings.Join(parts, " · "), ink
}

// The chart's series: usage first, so where it crosses a request or limit line its own stroke stays whole
func (p *page) chartSeries(keys [3]string) []series {
	names := [3]string{"usage", "request", "limit"}
	var out []series
	for i, k := range keys {
		v, ok := p.data[k]
		if !ok || last(v) == nil && first(v) == nil {
			continue
		}
		out = append(out, series{names[i], v, p.theme.series[i]})
	}
	return out
}

func (p *page) card(title, unit string, ss []series, width, rows int) block {
	return p.cardOf(title, unit, ss, width, rows, false)
}

// multiples: series that would flatten on one axis draw as small multiples (chart)
func (p *page) cardOf(title, unit string, ss []series, width, rows int, multiples bool) block {
	head := text(title, style{bold: true})
	var body block
	switch {
	case p.data == nil && len(p.failed) > 0:
		body = block{text("Couldn't load "+strings.ToLower(title), style{fg: p.theme.errorInk}), text("Dash0 didn't answer. The page asks again on the next refresh.", dimStyle)}
	case p.data == nil:
		body = block{text("◌ Loading "+strings.ToLower(title)+"…", dimStyle)}
	case len(ss) == 0:
		body = block{text("No data in this time range.", dimStyle)}
	default:
		body = chart(ss, p.win.times(), unit, width-4, rows, multiples)
	}
	return border(vjoin(1, block{head}, body), width, 1, dimStyle)
}

// layout: the page for a terminal w × h, and the plot columns its charts get
func (p *page) render(w, h int) (block, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	inner := w - 2
	side := inner >= 100
	cardW := inner
	if side {
		cardW = (inner - 1) / 2
	}
	header := block{
		text(p.target.namespace+"/"+p.target.pod, style{bold: true}),
		p.tags(),
		p.freshness(),
	}

	// The CTAs: Dash0's purple fill, each label in the CTA ink (Link, variant 'cta'), top right beside the header,
	// else beside the range toggles, else under them
	edge := style{fg: p.theme.ctaBg}
	var buttons []block
	for _, l := range p.links() {
		label := " " + l.label + " "
		buttons = append(buttons, block{
			text("╭"+strings.Repeat("─", len(label))+"╮", edge),
			line{{"│", edge}, {label, style{fg: p.theme.ctaText, bg: p.theme.ctaBg, bold: true, link: l.url}}, {"│", edge}},
			text("╰"+strings.Repeat("─", len(label))+"╯", edge),
		})
	}
	cta := hjoin(1, buttons...)
	toggles := p.toggles()
	switch {
	case inner-header.width()-cta.width() >= 2:
		header = hjoin(inner-header.width()-cta.width(), header, cta)
	case inner-toggles.width()-cta.width() >= 2:
		toggles = hjoin(inner-toggles.width()-cta.width(), toggles, cta)
	default:
		toggles = vjoin(0, toggles, cta)
	}
	// header 3, toggles 3, stats 7, hints 1, the gaps between them 3; each card's chrome 9
	cards := 1
	if !side {
		cards = 2
	}
	rows := max(5, min(12, (h-17-9*cards)/cards))

	var stats block
	if p.data != nil {
		cpuNote, cpuInk := p.against(p.data["cpu"], p.data["cpuRequest"], p.data["cpuLimit"], "cores")
		memNote, memInk := p.against(p.data["mem"], p.data["memRequest"], p.data["memLimit"], "bytes")
		since := "vs " + clock(p.win.start)
		stats = hjoin(6,
			stat("CPU", p.data["cpu"], "cores", cpuInk, since, cpuNote, false, p.theme),
			stat("Memory", p.data["mem"], "bytes", memInk, since, memNote, false, p.theme),
		)
	}

	cpu := p.card("CPU", "cores", p.chartSeries([3]string{"cpu", "cpuRequest", "cpuLimit"}), cardW, rows)
	mem := p.card("Memory", "bytes", p.chartSeries([3]string{"mem", "memRequest", "memLimit"}), cardW, rows)
	charts := vjoin(1, cpu, mem)
	if side {
		charts = hjoin(1, cpu, mem)
	}

	var red, logs block
	if p.data != nil {
		red = p.redSection(cardW, side, rows)
		logs = p.logsSection(inner, rows)
	}

	page := vjoin(1, header, toggles, stats, charts, red, logs)
	plotColumns := cardW - 4 - 8 - rightGap
	return indent(1, page), plotColumns
}
