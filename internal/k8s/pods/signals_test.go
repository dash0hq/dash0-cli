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
	tg := target{namespace: "shop", pod: "checkout-84d7c98f5d-m7tjp"}
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
