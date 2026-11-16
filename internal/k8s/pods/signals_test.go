package pods

import (
	"strings"
	"testing"
	"time"
)

func TestActiveIgnoresDash0Zeros(t *testing.T) {
	if active([]*float64{f(0), f(0), nil}) {
		t.Error("zeros are no data")
	}
	if !active([]*float64{f(0), f(0.02)}) {
		t.Error("a small rate is data")
	}
}

func TestAddSeries(t *testing.T) {
	got := addSeries(addSeries(nil, []*float64{f(1), nil}), []*float64{f(2), nil})
	if *got[0] != 3 || got[1] != nil {
		t.Errorf("got %v", got)
	}
}

func TestREDUnits(t *testing.T) {
	cases := []struct {
		v    float64
		unit string
		want string
	}{
		{0.139, "seconds", "139 ms"},
		{1.5, "seconds", "1.5 s"},
		{23.58, "/s", "23.6/s"},
		{12500, "/s", "12.5K/s"},
		{0.014, "percent", "0.01%"},
	}
	for _, c := range cases {
		if got := formatValue(f(c.v), c.unit); got != c.want {
			t.Errorf("formatValue(%v, %q) = %q, want %q", c.v, c.unit, got, c.want)
		}
	}
}

func TestPanelQueries(t *testing.T) {
	tg := target{namespace: "shop", kind: "pod", name: "checkout-84d7c98f5d-m7tjp"}
	sel := `k8s_namespace_name="shop", k8s_pod_name="checkout-84d7c98f5d-m7tjp"`
	pod := "k8s_cluster_name, k8s_namespace_name, k8s_pod_name"
	limit := `max by (` + pod + `, k8s_container_name) ({otel_metric_name="k8s.container.cpu_limit", ` + sel + `})`
	perPodLimit := `sum by (` + pod + `) (` + limit + `) and on (` + pod + `) (count by (` + pod + `) (` + limit + `) == count by (` + pod + `) (max by (` + pod + `, k8s_container_name) ({otel_metric_name="k8s.container.restarts", ` + sel + `})))`
	usage := `max by (` + pod + `) ({otel_metric_name="k8s.pod.cpu.usage", ` + sel + `})`
	want := map[string]string{
		"requests":  `sum(sum by (` + pod + `) (rate({otel_metric_name="dash0.spans", ` + sel + `, otel_span_kind="SERVER"}[120s])))`,
		"errorRate": `100 * sum(rate({otel_metric_name="dash0.spans", ` + sel + `, otel_span_kind="SERVER", otel_span_status_code="ERROR"}[120s])) / sum(rate({otel_metric_name="dash0.spans", ` + sel + `, otel_span_kind="SERVER"}[120s]))`,
		"p95":       `histogram_quantile(0.95, sum by (le) (rate({otel_metric_name="dash0.spans.duration", ` + sel + `, otel_span_kind="SERVER"}[120s])))`,
		"cpu":       `sum(` + usage + `)`,
		"cpuLimit":  `sum(` + perPodLimit + `)`,
		"cpuUtil":   `max(` + usage + ` / (` + perPodLimit + `))`,
		"logs":      `sum by (otel_log_severity_range) (increase({otel_metric_name="dash0.logs", ` + sel + `}[120s]))`,
	}
	for _, pn := range panels {
		if q, ok := want[pn.key]; ok {
			if got := pn.query(tg, 2*time.Minute); got != q {
				t.Errorf("%s:\n got %s\nwant %s", pn.key, got, q)
			}
			delete(want, pn.key)
		}
	}
	if len(want) > 0 {
		t.Errorf("no panels %v", want)
	}
}

func TestAggregateInPromQL(t *testing.T) {
	tg := target{namespace: "shop", kind: "deployment", name: "checkout", agg: "max"}
	for _, pn := range panels {
		q := pn.query(tg, time.Minute)
		switch pn.key {
		case "cpu", "cpuRequest", "memLimit", "requests", "errors":
			if !strings.HasPrefix(q, "max(") {
				t.Errorf("%s: %s", pn.key, q)
			}
		case "cpuUtil", "errorRate":
			// The busiest pod's share of its own limit, and the share of all requests, whatever the aggregation
			if strings.HasPrefix(q, "max(") != (pn.key == "cpuUtil") {
				t.Errorf("%s: %s", pn.key, q)
			}
		}
	}
	tg.agg = "total"
	if q := tg.query("k8s.pod.cpu.usage"); !strings.HasPrefix(q, "sum(") {
		t.Error(q)
	}
	// A workload's limit only when every pod with usage has one
	for _, pn := range panels {
		if pn.key == "memLimit" {
			if q := pn.query(tg, time.Minute); !strings.Contains(q, ") and on () (count(") {
				t.Error(q)
			}
		}
	}
}

func TestPerPodKeepsClusters(t *testing.T) {
	tg := target{namespace: "shop", kind: "deployment", name: "checkout"}
	q := podPanels[0].query(tg, time.Minute)
	if !strings.HasPrefix(q, `label_join(sum by (k8s_cluster_name, k8s_pod_name) (`) {
		t.Error(q)
	}
	tg.cluster = "eu"
	if q := podPanels[0].query(tg, time.Minute); !strings.HasPrefix(q, `sum by (k8s_pod_name) (`) {
		t.Error(q)
	}
	if q := podPanels[len(podPanels)-1].query(tg, time.Minute); !strings.HasPrefix(q, `histogram_quantile(0.95, sum by (le, k8s_pod_name) (`) {
		t.Error(q)
	}

	// One cluster goes unsaid, two are told apart
	p := &page{theme: dark, data: map[string][]*float64{"cpuPods:eu/checkout-a": {f(1)}, "cpuPods:eu/checkout-b": {f(2)}}}
	if got, _ := p.podSeries("cpuPods", ""); got[0].name != "checkout-b" {
		t.Errorf("got %+v", got)
	}
	p.data["cpuPods:us/checkout-a"] = []*float64{f(3)}
	if got, _ := p.podSeries("cpuPods", ""); got[0].name != "us/checkout-a" {
		t.Errorf("got %+v", got)
	}
}

func TestPodColorFollowsTheName(t *testing.T) {
	p := &page{theme: dark, data: map[string][]*float64{"cpuPods:checkout-a": {f(1)}, "cpuPods:checkout-b": {f(2)}}}
	before, _ := p.podSeries("cpuPods", "")
	p.data["cpuPods:checkout-a"] = []*float64{f(3)}
	after, _ := p.podSeries("cpuPods", "")
	if before[0].name != after[1].name || before[0].color != after[1].color || before[1].color != after[0].color {
		t.Errorf("before %+v, after %+v", before, after)
	}
	for i := 0; i < maxPods; i++ {
		p.data["cpuPods:pod-"+string(rune('a'+i))] = []*float64{f(9)}
	}
	got, _ := p.podSeries("cpuPods", "")
	seen := map[string]bool{}
	for _, s := range got {
		seen[s.color] = true
	}
	if len(seen) != len(got) {
		t.Errorf("%d pods share %d colours", len(got), len(seen))
	}
}

func TestServedWithoutErrorRate(t *testing.T) {
	p := &page{data: map[string][]*float64{"requests": {f(0), f(2)}}}
	if !p.served() {
		t.Error("requests without an error rate are served")
	}
	// Under min the requests can be zeros while other pods serve
	p.data = map[string][]*float64{"requests": {f(0), f(0)}, "errorRate": {nil, f(0)}}
	if !p.served() {
		t.Error("an error rate means requests")
	}
	p.data = map[string][]*float64{"requests": {f(0), f(0)}}
	if p.served() {
		t.Error("zeros are no requests")
	}
}

func TestSectionsHideWithoutData(t *testing.T) {
	p := &page{theme: dark, data: map[string][]*float64{"requests": {f(0), f(0)}, "logs:INFO": {f(0)}}}
	if p.redSection(60, false, 5) != nil || p.logsSection(60, 5) != nil {
		t.Error("a pod with no spans or logs gets no RED or Logs section")
	}
	p.data["logs:ERROR"] = []*float64{f(3), f(0)}
	p.win = window{time.Unix(0, 0), time.Unix(60, 0), time.Minute}
	logs := p.logsSection(60, 5)
	if logs == nil || !strings.Contains(logs[0][0].text, "Logs") {
		t.Errorf("logs section missing")
	}
}

func TestKindOf(t *testing.T) {
	for in, want := range map[string]string{"deployments": "deployment", "StatefulSet": "statefulset", "pods": "pod", "deploy": "deployment", "sts": "statefulset", "services": ""} {
		if got := kindOf(in); got != want {
			t.Errorf("kindOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPodSeriesRanksBusiestFirst(t *testing.T) {
	p := &page{theme: dark, data: map[string][]*float64{"cpu": {f(99)}}}
	for i := 0; i < maxPods+2; i++ {
		p.data["cpuPods:pod-"+string(rune('a'+i))] = []*float64{f(float64(i)), nil}
	}
	p.data["cpuPods:gone"] = []*float64{nil, nil}
	got, n := p.podSeries("cpuPods", "")
	if len(got) != maxPods || n != maxPods+2 || got[0].name != "pod-j" || got[maxPods-1].name != "pod-c" {
		t.Errorf("got %d of %d: %+v", len(got), n, got)
	}

	// Nearest its limit first, then pods without a limit by usage
	p.data["cpuUtilPods:pod-a"] = []*float64{f(0.95), f(0.1)}
	p.data["cpuUtilPods:pod-b"] = []*float64{f(0.5), nil}
	got, _ = p.podSeries("cpuPods", "cpuUtilPods")
	if got[0].name != "pod-a" || got[1].name != "pod-b" || got[2].name != "pod-j" {
		t.Errorf("got %+v", got[:3])
	}
}

func TestWorkloadSelector(t *testing.T) {
	q := target{namespace: "ns", kind: "deployment", name: "checkout"}.selector("m")
	if !strings.Contains(q, `k8s_deployment_name="checkout"`) || strings.Contains(q, "k8s_pod_name") {
		t.Error(q)
	}
}

func TestAgainstTheBusiestPodsLimit(t *testing.T) {
	for _, c := range []struct {
		kind string
		util *float64
		note string
		warn bool
	}{
		{"pod", f(0.85), "85% of 10 cores limit", true},
		{"pod", f(0.5), "50% of 10 cores limit", false},
		{"deployment", f(0.95), "busiest pod at 95% of its limit", true},
		{"deployment", nil, "no limit", false},
	} {
		p := &page{theme: dark, target: target{kind: c.kind}}
		note, ink := p.against([]*float64{f(5)}, []*float64{f(4)}, []*float64{f(10)}, []*float64{c.util}, "cores")
		if !strings.Contains(note, c.note) || (ink != "") != c.warn {
			t.Errorf("%s %v: %q %q", c.kind, c.util, note, ink)
		}
	}

	// A zero limit is none
	p := &page{theme: dark, target: target{kind: "pod"}}
	if note, _ := p.against([]*float64{f(5)}, nil, []*float64{f(0)}, []*float64{f(0.5)}, "cores"); note != "no limit" {
		t.Errorf("%q", note)
	}
	// A workload without a limit on every pod, its limited pods still counted
	p.target = target{kind: "deployment", agg: "total"}
	if note, ink := p.against([]*float64{f(5)}, []*float64{f(4)}, nil, []*float64{f(0.95)}, "cores"); note != "125% of 4 cores requested · not every pod has a limit · busiest limited pod at 95% of its limit" || ink == "" {
		t.Errorf("%q %q", note, ink)
	}
	// Under max the usage and the request can be different pods'
	p.target.agg = "max"
	if note, _ := p.against([]*float64{f(5)}, []*float64{f(4)}, []*float64{f(10)}, []*float64{f(0.5)}, "cores"); strings.Contains(note, "requested") {
		t.Errorf("%q", note)
	}
}
