package otlp

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Decorator upserts user-provided resource attributes, scope attributes,
// scope name, and scope version onto every pdata batch flowing through
// the proxy before it is forwarded upstream. The mirror flags
// (`--resource-attribute`, `--scope-attribute`, `--scope-name`,
// `--scope-version`) on the proxy command match the same flags on
// `dash0 logs send` and `dash0 spans send` so the user experience is
// consistent across signal-authoring and signal-forwarding workflows.
//
// All upserts are no-ops when the corresponding flag was not provided
// — the proxy does NOT default scope name/version to `dash0-cli` /
// CLI version the way the send commands do, because the inbound pdata
// already carries the SDK's own instrumentation-library identity and
// silently overwriting it would erase useful diagnostic context.
//
// Decoration happens in the worker pool, after the consumer has
// returned 200 to the SDK. The pdata is therefore mutated only by the
// proxy's own goroutines; the receiver-side contract reported by
// ProxyConsumer.Capabilities is unaffected.
type Decorator struct {
	resourceAttrs map[string]string
	scopeAttrs    map[string]string
	scopeName     string
	scopeVersion  string

	// Per-signal record-level attribute upserts. Each is applied to
	// every individual record of its respective signal type:
	//   logAttrs    → every LogRecord.Attributes()
	//   spanAttrs   → every Span.Attributes()
	//   metricAttrs → every metric data point's Attributes(), for all
	//                 metric flavors (Gauge, Sum, Histogram,
	//                 ExponentialHistogram, Summary)
	logAttrs    map[string]string
	spanAttrs   map[string]string
	metricAttrs map[string]string
}

// NewDecorator returns a Decorator with the supplied user-controlled
// upserts. Any zero-valued field is a no-op when Decorate* is called.
func NewDecorator(
	resourceAttrs, scopeAttrs map[string]string,
	scopeName, scopeVersion string,
	logAttrs, spanAttrs, metricAttrs map[string]string,
) *Decorator {
	return &Decorator{
		resourceAttrs: resourceAttrs,
		scopeAttrs:    scopeAttrs,
		scopeName:     scopeName,
		scopeVersion:  scopeVersion,
		logAttrs:      logAttrs,
		spanAttrs:     spanAttrs,
		metricAttrs:   metricAttrs,
	}
}

// IsEmpty reports whether the decorator carries any user-provided
// changes. Callers can skip the decoration call entirely when true,
// avoiding the iteration cost.
func (d *Decorator) IsEmpty() bool {
	return d == nil ||
		(len(d.resourceAttrs) == 0 &&
			len(d.scopeAttrs) == 0 &&
			d.scopeName == "" &&
			d.scopeVersion == "" &&
			len(d.logAttrs) == 0 &&
			len(d.spanAttrs) == 0 &&
			len(d.metricAttrs) == 0)
}

// DecorateLogs upserts the configured fields onto each ResourceLogs,
// each ScopeLogs scope, and each LogRecord in ld.
func (d *Decorator) DecorateLogs(ld plog.Logs) {
	if d.IsEmpty() {
		return
	}
	rls := ld.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		rl := rls.At(i)
		d.upsertResource(rl.Resource().Attributes())
		sls := rl.ScopeLogs()
		for j := 0; j < sls.Len(); j++ {
			sl := sls.At(j)
			d.upsertScope(sl.Scope())
			if len(d.logAttrs) > 0 {
				lrs := sl.LogRecords()
				for k := 0; k < lrs.Len(); k++ {
					d.upsertMap(lrs.At(k).Attributes(), d.logAttrs)
				}
			}
		}
	}
}

// DecorateTraces upserts the configured fields onto each ResourceSpans,
// each ScopeSpans scope, and each Span in td.
func (d *Decorator) DecorateTraces(td ptrace.Traces) {
	if d.IsEmpty() {
		return
	}
	rss := td.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		d.upsertResource(rs.Resource().Attributes())
		sss := rs.ScopeSpans()
		for j := 0; j < sss.Len(); j++ {
			ss := sss.At(j)
			d.upsertScope(ss.Scope())
			if len(d.spanAttrs) > 0 {
				spans := ss.Spans()
				for k := 0; k < spans.Len(); k++ {
					d.upsertMap(spans.At(k).Attributes(), d.spanAttrs)
				}
			}
		}
	}
}

// DecorateMetrics upserts the configured fields onto each ResourceMetrics,
// each ScopeMetrics scope, and each metric data point in md. Metric data
// points come in five flavors — Gauge, Sum, Histogram, ExponentialHistogram,
// Summary — and the decorator iterates the appropriate slice for each.
func (d *Decorator) DecorateMetrics(md pmetric.Metrics) {
	if d.IsEmpty() {
		return
	}
	rms := md.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		rm := rms.At(i)
		d.upsertResource(rm.Resource().Attributes())
		sms := rm.ScopeMetrics()
		for j := 0; j < sms.Len(); j++ {
			sm := sms.At(j)
			d.upsertScope(sm.Scope())
			if len(d.metricAttrs) > 0 {
				ms := sm.Metrics()
				for k := 0; k < ms.Len(); k++ {
					d.upsertMetricDataPoints(ms.At(k))
				}
			}
		}
	}
}

// upsertMetricDataPoints applies the metricAttrs map to every data point
// of m, fanning out across the five metric type variants. A metric in a
// pdata batch always has exactly one populated variant (the others are
// zero-length), so iterating all five is safe and the unused branches
// short-circuit at the Len() check.
func (d *Decorator) upsertMetricDataPoints(m pmetric.Metric) {
	switch m.Type() {
	case pmetric.MetricTypeGauge:
		dps := m.Gauge().DataPoints()
		for i := 0; i < dps.Len(); i++ {
			d.upsertMap(dps.At(i).Attributes(), d.metricAttrs)
		}
	case pmetric.MetricTypeSum:
		dps := m.Sum().DataPoints()
		for i := 0; i < dps.Len(); i++ {
			d.upsertMap(dps.At(i).Attributes(), d.metricAttrs)
		}
	case pmetric.MetricTypeHistogram:
		dps := m.Histogram().DataPoints()
		for i := 0; i < dps.Len(); i++ {
			d.upsertMap(dps.At(i).Attributes(), d.metricAttrs)
		}
	case pmetric.MetricTypeExponentialHistogram:
		dps := m.ExponentialHistogram().DataPoints()
		for i := 0; i < dps.Len(); i++ {
			d.upsertMap(dps.At(i).Attributes(), d.metricAttrs)
		}
	case pmetric.MetricTypeSummary:
		dps := m.Summary().DataPoints()
		for i := 0; i < dps.Len(); i++ {
			d.upsertMap(dps.At(i).Attributes(), d.metricAttrs)
		}
	}
}

// upsertMap is a small helper to keep the four call sites consistent.
func (d *Decorator) upsertMap(attrs pcommon.Map, src map[string]string) {
	for k, v := range src {
		attrs.PutStr(k, v)
	}
}

func (d *Decorator) upsertResource(attrs pcommon.Map) {
	for k, v := range d.resourceAttrs {
		attrs.PutStr(k, v)
	}
}

func (d *Decorator) upsertScope(scope pcommon.InstrumentationScope) {
	if d.scopeName != "" {
		scope.SetName(d.scopeName)
	}
	if d.scopeVersion != "" {
		scope.SetVersion(d.scopeVersion)
	}
	if len(d.scopeAttrs) > 0 {
		attrs := scope.Attributes()
		for k, v := range d.scopeAttrs {
			attrs.PutStr(k, v)
		}
	}
}
