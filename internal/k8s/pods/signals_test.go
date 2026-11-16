package pods

import (
	"strings"
	"testing"
	"time"
)

func TestErrorRate(t *testing.T) {
	got := errorRate([]*float64{f(10), f(0), nil, f(4)}, []*float64{f(1), f(0), nil, nil})
	if *got[0] != 10 || got[1] != nil || got[2] != nil || *got[3] != 0 {
		t.Errorf("got %v", got)
	}
}

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
	want := map[string]string{
		"requests": `sum(rate({otel_metric_name="dash0.spans", k8s_namespace_name="shop", k8s_pod_name="checkout-84d7c98f5d-m7tjp", otel_span_kind="SERVER"}[120s]))`,
		"p95":      `histogram_quantile(0.95, sum by (le) (rate({otel_metric_name="dash0.spans.duration", k8s_namespace_name="shop", k8s_pod_name="checkout-84d7c98f5d-m7tjp", otel_span_kind="SERVER"}[120s])))`,
		"cpu":      `sum(max by (k8s_cluster_name, k8s_namespace_name, k8s_pod_name) ({otel_metric_name="k8s.pod.cpu.usage", k8s_namespace_name="shop", k8s_pod_name="checkout-84d7c98f5d-m7tjp"}))`,
		"cpuLimit": `sum(max by (k8s_cluster_name, k8s_namespace_name, k8s_pod_name, k8s_container_name) ({otel_metric_name="k8s.container.cpu_limit", k8s_namespace_name="shop", k8s_pod_name="checkout-84d7c98f5d-m7tjp"}))`,
		"logs":     `sum by (otel_log_severity_range) (increase({otel_metric_name="dash0.logs", k8s_namespace_name="shop", k8s_pod_name="checkout-84d7c98f5d-m7tjp"}[120s]))`,
	}
	for _, pn := range panels {
		if q, ok := want[pn.key]; ok {
			if got := pn.query(tg, 2*time.Minute); got != q {
				t.Errorf("%s:\n got %s\nwant %s", pn.key, got, q)
			}
		}
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
	for in, want := range map[string]string{"deployments": "deployment", "StatefulSet": "statefulset", "pods": "pod", "services": ""} {
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
	got, n := p.podSeries("cpuPods")
	if len(got) != maxPods || n != maxPods+2 || got[0].name != "pod-j" || got[maxPods-1].name != "pod-c" {
		t.Errorf("got %d of %d: %+v", len(got), n, got)
	}
}

func TestWorkloadSelector(t *testing.T) {
	q := target{namespace: "ns", kind: "deployment", name: "checkout"}.selector("m")
	if !strings.Contains(q, `k8s_deployment_name="checkout"`) || strings.Contains(q, "k8s_pod_name") {
		t.Error(q)
	}
}

func TestAggregateOverRunningPods(t *testing.T) {
	pods := [][]*float64{{f(1), f(4), nil}, {f(3), nil, nil}}
	for how, want := range map[string][2]float64{"avg": {2, 4}, "min": {1, 4}, "max": {3, 4}} {
		got := aggregate(pods, how)
		if *got[0] != want[0] || *got[1] != want[1] || got[2] != nil {
			t.Errorf("%s: got %v %v %v", how, *got[0], *got[1], got[2])
		}
	}
	p := &page{agg: 3, data: map[string][]*float64{"cpu": {f(9)}, "cpuPods:a": {f(2)}, "cpuPods:b": {f(5)}, "cpuRequestPods:a": {f(7)}}}
	if got := p.of("cpu"); *got[0] != 5 {
		t.Errorf("max cpu %v", *got[0])
	}
}

func TestLimitsOnlyWhereTheyHold(t *testing.T) {
	data := map[string][]*float64{"cpu": {f(9)}, "cpuRequest": {f(10)}, "cpuLimit": {f(9.5)},
		"cpuPods:a": {f(2)}, "cpuRequestPods:a": {f(4)}, "cpuLimitPods:a": {f(2.1)}}
	for _, c := range []struct {
		kind string
		agg  int
		want int
	}{{"pod", 0, 3}, {"deployment", 0, 2}, {"deployment", 1, 2}, {"deployment", 3, 3}} {
		p := &page{theme: dark, target: target{kind: c.kind}, agg: c.agg, data: data}
		if n := len(p.chartSeries([3]string{"cpu", "cpuRequest", "cpuLimit"})); n != c.want {
			t.Errorf("%s %s: %d series, want %d", c.kind, aggs[c.agg], n, c.want)
		}
		note, ink := p.against(p.of("cpu"), p.of("cpuRequest"), p.of("cpuLimit"), "cores")
		if strings.Contains(note, "limit") != (c.want == 3) || (c.want == 2 && ink != "") {
			t.Errorf("%s %s: %q %q", c.kind, aggs[c.agg], note, ink)
		}
	}
}
