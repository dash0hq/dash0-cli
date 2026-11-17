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
	sel := []string{"otel_metric_name=" + quote(metric), "k8s_namespace_name=" + quote(t.namespace), "k8s_" + t.kind + "_name=" + quote(t.name)}
	if t.cluster != "" {
		sel = append(sel, "k8s_cluster_name="+quote(t.cluster))
	}
	return "{" + strings.Join(append(sel, extra...), ", ") + "}"
}

const podLabels = "k8s_cluster_name, k8s_namespace_name, k8s_pod_name"

// A gauge once per pod, or per pod and container for the containers' requests and limits (the k8s_cluster receiver
// reports them per container). A pod whose resource attributes change mid-life has two series for a while, the same
// value under two dash0_resource_hash; summing them would count it twice. The cluster stays in the grouping, because
// without --cluster the selector matches a pod of the same namespace and name in every cluster, and those are summed
func (t target) once(metric string) string {
	labels := podLabels
	if strings.HasPrefix(metric, "k8s.container.") {
		labels += ", k8s_container_name"
	}
	return by("max", labels, t.selector(metric))
}

// fn by (labels) (expr)
func by(fn, labels, expr string) string { return fn + " by (" + labels + ") (" + expr + ")" }

// A series per pod: its usage, or its containers' requests or limits added up. A limit counts only when every
// container has one above zero, as Dash0's pod pages have it: a container without one can use the whole node, and
// Kubernetes reads a zero limit as none
func (t target) perPod(metric string) string {
	if !strings.HasPrefix(metric, "k8s.container.") {
		return t.once(metric)
	}
	q := by("sum", podLabels, t.once(metric))
	if strings.HasSuffix(metric, "_limit") {
		// k8s.container.restarts has a series per container, from the same receiver as the limits; a pending pod
		// missing from it has no usage to hold against a limit anyway
		limited := by("count", podLabels, t.once(metric)+" > 0")
		containers := by("count", podLabels, t.once("k8s.container.restarts"))
		q += " and on (" + podLabels + ") (" + limited + " == " + containers + ")"
	}
	return q
}

// The target's pods combined under its aggregation: their total, or the mean, least or most of those running
func (t target) query(metric string) string { return t.aggFn() + "(" + t.perPod(metric) + ")" }

func (t target) aggFn() string {
	if t.isTotal() {
		return "sum"
	}
	return t.agg
}

// A query grouped by its by labels: the whole target with none, else a series per pod
type grouped func(t target, step time.Duration, by string) string

func whole(q grouped) func(target, time.Duration) string {
	return func(t target, step time.Duration) string { return q(t, step, "") }
}

// " by (labels) ", or nothing for no labels
func groupBy(labels string) string {
	if labels == "" {
		return ""
	}
	return " by (" + labels + ") "
}

// The highest share of its limit any pod uses, Dash0's "% of limit"; none when no pod has a limit
func utilization(usage, limit string) grouped {
	return func(t target, _ time.Duration, by string) string {
		return "max" + groupBy(by) + "(" + t.once(usage) + " / (" + t.perPod(limit) + "))"
	}
}

func gauge(metric string) func(target, time.Duration) string {
	return func(t target, _ time.Duration) string { return t.query(metric) }
}

// A workload's limit only when every pod using CPU or memory has one: a pod without one can use the whole node. The
// counts are never zero, so Dash0's and keeps them
func limit(metric, usage string) func(target, time.Duration) string {
	return func(t target, _ time.Duration) string {
		q := t.query(metric)
		if t.isPod() {
			return q
		}
		return q + " and on () (count(" + t.once(usage) + " / (" + t.perPod(metric) + ")) == count(" + t.once(usage) + "))"
	}
}

func rangeOf(step time.Duration) string { return fmt.Sprintf("[%ds]", int(step.Seconds())) }

// RED from the spans the pod serves, as Ditt0's Services Overview asks them (connectors/dash0/overview.js): SERVER
// spans only, a rate over the step, the duration from dash0.spans.duration's histogram
func spans(t target, extra ...string) string {
	return t.selector("dash0.spans", append([]string{`otel_span_kind="SERVER"`}, extra...)...)
}

func quantile(q string) grouped {
	return func(t target, step time.Duration, by string) string {
		if by != "" {
			by = ", " + by
		}
		return "histogram_quantile(" + q + ", sum by (le" + by + ") (rate(" + t.selector("dash0.spans.duration", `otel_span_kind="SERVER"`) + rangeOf(step) + ")))"
	}
}

var panels = []panel{
	{"cpu", gauge("k8s.pod.cpu.usage"), ""}, {"cpuRequest", gauge("k8s.container.cpu_request"), ""}, {"cpuLimit", limit("k8s.container.cpu_limit", "k8s.pod.cpu.usage"), ""},
	{"cpuUtil", whole(utilization("k8s.pod.cpu.usage", "k8s.container.cpu_limit")), ""},
	{"mem", gauge("k8s.pod.memory.working_set"), ""}, {"memRequest", gauge("k8s.container.memory_request"), ""}, {"memLimit", limit("k8s.container.memory_limit", "k8s.pod.memory.working_set"), ""},
	{"memUtil", whole(utilization("k8s.pod.memory.working_set", "k8s.container.memory_limit")), ""},
	{"requests", func(t target, step time.Duration) string {
		return t.aggFn() + "(sum by (" + podLabels + ") (rate(" + spans(t) + rangeOf(step) + ")))"
	}, ""},
	{"errors", func(t target, step time.Duration) string {
		return t.aggFn() + "(sum by (" + podLabels + ") (rate(" + spans(t, `otel_span_status_code="ERROR"`) + rangeOf(step) + ")))"
	}, ""},
	// The share of all the target's requests that failed, whatever the aggregation: one pod's errors over another's
	// requests would mean nothing. No requests make it 0 / 0, a gap
	{"errorRate", func(t target, step time.Duration) string {
		return "100 * sum(rate(" + spans(t, `otel_span_status_code="ERROR"`) + rangeOf(step) + ")) / sum(rate(" + spans(t) + rangeOf(step) + "))"
	}, ""},
	{"p50", whole(quantile("0.5")), ""}, {"p95", whole(quantile("0.95")), ""}, {"p99", whole(quantile("0.99")), ""},
	// Log records per step, by severity
	{"logs", func(t target, step time.Duration) string {
		return "sum by (otel_log_severity_range) (increase(" + t.selector("dash0.logs") + rangeOf(step) + "))"
	}, "otel_log_severity_range"},
}

// A workload's running pods: the k8s_cluster receiver's k8s.pod.phase is 2 for Running
var podCount = panel{"pods", func(t target, _ time.Duration) string { return "count(" + t.once("k8s.pod.phase") + " == 2)" }, ""}

// A workload's panels split by pod, for the per-pod view; the shares of their limits rank them
var podPanels = []panel{
	byPod("cpuPods", usageByPod("k8s.pod.cpu.usage")),
	byPod("memPods", usageByPod("k8s.pod.memory.working_set")),
	byPod("cpuUtilPods", utilization("k8s.pod.cpu.usage", "k8s.container.cpu_limit")),
	byPod("memUtilPods", utilization("k8s.pod.memory.working_set", "k8s.container.memory_limit")),
	byPod("requestsPods", func(t target, step time.Duration, by string) string {
		return "sum by (" + by + ") (rate(" + spans(t) + rangeOf(step) + "))"
	}),
	byPod("p95Pods", quantile("0.95")),
}

// A panel with a series per pod. Without --cluster, pods of the same name in different clusters stay apart, named
// <cluster>/<pod>
func byPod(key string, q grouped) panel {
	return panel{key, func(t target, step time.Duration) string {
		if t.cluster != "" {
			return q(t, step, "k8s_pod_name")
		}
		return `label_join(` + q(t, step, "k8s_cluster_name, k8s_pod_name") + `, "k8s_pod_name", "/", "k8s_cluster_name", "k8s_pod_name")`
	}, "k8s_pod_name"}
}

func usageByPod(metric string) grouped {
	return func(t target, _ time.Duration, by string) string {
		return "sum by (" + by + ") (" + t.once(metric) + ")"
	}
}

// The panels a target asks for: a pod's; a workload's adds its pod count, and the per-pod view its pods' series
func panelsFor(t target, perPod bool) []panel {
	if t.isPod() {
		return panels
	}
	var out []panel
	for _, pn := range panels {
		// A pod that served no ERROR span has no series here, so avg and min would skip it and read too high; Dash0's
		// and, or and unless drop zero samples, so `or 0 * requests` can't fill it in. The error rate covers them
		if pn.key == "errors" && (t.agg == "avg" || t.agg == "min") {
			continue
		}
		out = append(out, pn)
	}
	out = append(out, podCount)
	if perPod {
		out = append(out, podPanels...)
	}
	return out
}

// How a workload's pods add up in its stats and charts: their total, or the mean, least or most across the pods at
// each step, taken for usage, requests and limits apart, so under max each can be another pod's. Dash0 combines them,
// so switching asks it again
var aggs = []string{"total", "avg", "min", "max"}
