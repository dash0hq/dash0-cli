package otlp

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// proxy_tail.go renders forwarded OTLP records in a human-readable form
// modeled on the OTel Collector debug exporter's `detailed` verbosity. The
// renderer is called from the consumer (U13) at enqueue time, before the
// worker pool (U4) attempts the outbound forward, so there is no per-record
// success/fail status marker — async-forward means the outcome isn't known
// yet, and the Collector debug exporter doesn't annotate outcomes either.
//
// Output goes to stdout in TTY mode; agent mode + --tail is rejected at
// startup (KTD11), so this renderer never runs in agent mode.

// RenderLogs returns the multi-line debug-exporter-style rendering for a
// pdata Logs batch. An empty batch (zero ResourceLogs) returns the empty
// string so the caller can skip writing.
func RenderLogs(ld plog.Logs) string {
	if ld.ResourceLogs().Len() == 0 {
		return ""
	}
	var b strings.Builder
	rls := ld.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		rl := rls.At(i)
		fmt.Fprintf(&b, "ResourceLogs #%d\n", i)
		writeSchemaURL(&b, "", rl.SchemaUrl())
		writeAttributes(&b, "  Resource attributes:", rl.Resource().Attributes(), "    ")

		sls := rl.ScopeLogs()
		for j := 0; j < sls.Len(); j++ {
			sl := sls.At(j)
			fmt.Fprintf(&b, "  ScopeLogs #%d\n", j)
			writeSchemaURL(&b, "  ", sl.SchemaUrl())
			writeScope(&b, "    ", sl.Scope())

			lrs := sl.LogRecords()
			for k := 0; k < lrs.Len(); k++ {
				lr := lrs.At(k)
				fmt.Fprintf(&b, "    LogRecord #%d\n", k)
				writeLogRecord(&b, "      ", lr)
			}
		}
	}
	return b.String()
}

// RenderTraces returns the multi-line debug-exporter-style rendering for a
// pdata Traces batch.
func RenderTraces(td ptrace.Traces) string {
	if td.ResourceSpans().Len() == 0 {
		return ""
	}
	var b strings.Builder
	rss := td.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		fmt.Fprintf(&b, "ResourceSpans #%d\n", i)
		writeSchemaURL(&b, "", rs.SchemaUrl())
		writeAttributes(&b, "  Resource attributes:", rs.Resource().Attributes(), "    ")

		sss := rs.ScopeSpans()
		for j := 0; j < sss.Len(); j++ {
			ss := sss.At(j)
			fmt.Fprintf(&b, "  ScopeSpans #%d\n", j)
			writeSchemaURL(&b, "  ", ss.SchemaUrl())
			writeScope(&b, "    ", ss.Scope())

			spans := ss.Spans()
			for k := 0; k < spans.Len(); k++ {
				sp := spans.At(k)
				fmt.Fprintf(&b, "    Span #%d\n", k)
				writeSpan(&b, "      ", sp)
			}
		}
	}
	return b.String()
}

// RenderMetrics returns the multi-line debug-exporter-style rendering for a
// pdata Metrics batch.
func RenderMetrics(md pmetric.Metrics) string {
	if md.ResourceMetrics().Len() == 0 {
		return ""
	}
	var b strings.Builder
	rms := md.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		rm := rms.At(i)
		fmt.Fprintf(&b, "ResourceMetrics #%d\n", i)
		writeSchemaURL(&b, "", rm.SchemaUrl())
		writeAttributes(&b, "  Resource attributes:", rm.Resource().Attributes(), "    ")

		sms := rm.ScopeMetrics()
		for j := 0; j < sms.Len(); j++ {
			sm := sms.At(j)
			fmt.Fprintf(&b, "  ScopeMetrics #%d\n", j)
			writeSchemaURL(&b, "  ", sm.SchemaUrl())
			writeScope(&b, "    ", sm.Scope())

			metrics := sm.Metrics()
			for k := 0; k < metrics.Len(); k++ {
				m := metrics.At(k)
				fmt.Fprintf(&b, "    Metric #%d\n", k)
				writeMetric(&b, "      ", m)
			}
		}
	}
	return b.String()
}

// writeLogRecord prints a LogRecord with the given indent prefix on every
// field line.
func writeLogRecord(b *strings.Builder, indent string, lr plog.LogRecord) {
	if ts := lr.Timestamp().AsTime(); !ts.IsZero() {
		fmt.Fprintf(b, "%sTimestamp: %s\n", indent, ts.UTC().Format(time.RFC3339Nano))
	}
	if ts := lr.ObservedTimestamp().AsTime(); !ts.IsZero() {
		fmt.Fprintf(b, "%sObservedTimestamp: %s\n", indent, ts.UTC().Format(time.RFC3339Nano))
	}
	if name := lr.EventName(); name != "" {
		fmt.Fprintf(b, "%sEventName: %s\n", indent, name)
	}
	if sev := lr.SeverityText(); sev != "" {
		fmt.Fprintf(b, "%sSeverityText: %s\n", indent, colorSeverity(sev, 0))
	}
	if num := lr.SeverityNumber(); num != plog.SeverityNumberUnspecified {
		fmt.Fprintf(b, "%sSeverityNumber: %d (%s)\n", indent, num, colorSeverity(SeverityNumberToRange(int32(num)), 0))
	}
	fmt.Fprintf(b, "%sBody: %s\n", indent, renderValue(lr.Body()))
	if tid := lr.TraceID(); !tid.IsEmpty() {
		fmt.Fprintf(b, "%sTrace ID: %s\n", indent, hex.EncodeToString(tid[:]))
	}
	if sid := lr.SpanID(); !sid.IsEmpty() {
		fmt.Fprintf(b, "%sSpan ID: %s\n", indent, hex.EncodeToString(sid[:]))
	}
	writeAttributes(b, indent+"Attributes:", lr.Attributes(), indent+"  ")
}

// writeSpan prints a Span with the given indent prefix on every field line.
func writeSpan(b *strings.Builder, indent string, sp ptrace.Span) {
	tid := sp.TraceID()
	sid := sp.SpanID()
	pid := sp.ParentSpanID()
	fmt.Fprintf(b, "%sTrace ID: %s\n", indent, hex.EncodeToString(tid[:]))
	fmt.Fprintf(b, "%sParent ID: %s\n", indent, hex.EncodeToString(pid[:]))
	fmt.Fprintf(b, "%sSpan ID: %s\n", indent, hex.EncodeToString(sid[:]))
	fmt.Fprintf(b, "%sName: %s\n", indent, sp.Name())
	fmt.Fprintf(b, "%sKind: %s\n", indent, sp.Kind().String())
	if ts := sp.StartTimestamp().AsTime(); !ts.IsZero() {
		fmt.Fprintf(b, "%sStart time: %s\n", indent, ts.UTC().Format(time.RFC3339Nano))
	}
	if ts := sp.EndTimestamp().AsTime(); !ts.IsZero() {
		fmt.Fprintf(b, "%sEnd time: %s\n", indent, ts.UTC().Format(time.RFC3339Nano))
	}
	status := sp.Status()
	fmt.Fprintf(b, "%sStatus code: %s\n", indent, colorSpanStatus(status.Code().String(), 0))
	if msg := status.Message(); msg != "" {
		fmt.Fprintf(b, "%sStatus message: %s\n", indent, msg)
	}
	writeAttributes(b, indent+"Attributes:", sp.Attributes(), indent+"  ")
	writeSpanEvents(b, indent, sp.Events())
	writeSpanLinks(b, indent, sp.Links())
}

// writeSpanEvents emits each event under the span if any are present.
func writeSpanEvents(b *strings.Builder, indent string, events ptrace.SpanEventSlice) {
	if events.Len() == 0 {
		return
	}
	fmt.Fprintf(b, "%sEvents:\n", indent)
	for i := 0; i < events.Len(); i++ {
		ev := events.At(i)
		fmt.Fprintf(b, "%s  Event #%d\n", indent, i)
		if ts := ev.Timestamp().AsTime(); !ts.IsZero() {
			fmt.Fprintf(b, "%s    Timestamp: %s\n", indent, ts.UTC().Format(time.RFC3339Nano))
		}
		fmt.Fprintf(b, "%s    Name: %s\n", indent, ev.Name())
		writeAttributes(b, indent+"    Attributes:", ev.Attributes(), indent+"      ")
	}
}

// writeSpanLinks emits each link under the span if any are present.
func writeSpanLinks(b *strings.Builder, indent string, links ptrace.SpanLinkSlice) {
	if links.Len() == 0 {
		return
	}
	fmt.Fprintf(b, "%sLinks:\n", indent)
	for i := 0; i < links.Len(); i++ {
		lk := links.At(i)
		fmt.Fprintf(b, "%s  Link #%d\n", indent, i)
		tid := lk.TraceID()
		sid := lk.SpanID()
		fmt.Fprintf(b, "%s    Trace ID: %s\n", indent, hex.EncodeToString(tid[:]))
		fmt.Fprintf(b, "%s    Span ID: %s\n", indent, hex.EncodeToString(sid[:]))
		writeAttributes(b, indent+"    Attributes:", lk.Attributes(), indent+"      ")
	}
}

// writeMetric prints a Metric with the given indent prefix on every field line.
func writeMetric(b *strings.Builder, indent string, m pmetric.Metric) {
	fmt.Fprintf(b, "%sName: %s\n", indent, m.Name())
	if desc := m.Description(); desc != "" {
		fmt.Fprintf(b, "%sDescription: %s\n", indent, desc)
	}
	if unit := m.Unit(); unit != "" {
		fmt.Fprintf(b, "%sUnit: %s\n", indent, unit)
	}
	fmt.Fprintf(b, "%sKind: %s\n", indent, m.Type().String())
	switch m.Type() {
	case pmetric.MetricTypeGauge:
		writeNumberDataPoints(b, indent+"  ", m.Gauge().DataPoints())
	case pmetric.MetricTypeSum:
		sum := m.Sum()
		fmt.Fprintf(b, "%sIsMonotonic: %t\n", indent, sum.IsMonotonic())
		fmt.Fprintf(b, "%sAggregationTemporality: %s\n", indent, sum.AggregationTemporality().String())
		writeNumberDataPoints(b, indent+"  ", sum.DataPoints())
	case pmetric.MetricTypeHistogram:
		hist := m.Histogram()
		fmt.Fprintf(b, "%sAggregationTemporality: %s\n", indent, hist.AggregationTemporality().String())
		writeHistogramDataPoints(b, indent+"  ", hist.DataPoints())
	case pmetric.MetricTypeExponentialHistogram:
		eh := m.ExponentialHistogram()
		fmt.Fprintf(b, "%sAggregationTemporality: %s\n", indent, eh.AggregationTemporality().String())
		writeExponentialHistogramDataPoints(b, indent+"  ", eh.DataPoints())
	case pmetric.MetricTypeSummary:
		writeSummaryDataPoints(b, indent+"  ", m.Summary().DataPoints())
	}
}

func writeNumberDataPoints(b *strings.Builder, indent string, dps pmetric.NumberDataPointSlice) {
	for i := 0; i < dps.Len(); i++ {
		dp := dps.At(i)
		fmt.Fprintf(b, "%sDataPoint #%d\n", indent, i)
		writeDataPointTimestamps(b, indent+"  ", dp.StartTimestamp(), dp.Timestamp())
		switch dp.ValueType() {
		case pmetric.NumberDataPointValueTypeInt:
			fmt.Fprintf(b, "%s  Value: Int(%d)\n", indent, dp.IntValue())
		case pmetric.NumberDataPointValueTypeDouble:
			fmt.Fprintf(b, "%s  Value: Double(%s)\n", indent, strconv.FormatFloat(dp.DoubleValue(), 'f', -1, 64))
		}
		writeAttributes(b, indent+"  Attributes:", dp.Attributes(), indent+"    ")
	}
}

func writeHistogramDataPoints(b *strings.Builder, indent string, dps pmetric.HistogramDataPointSlice) {
	for i := 0; i < dps.Len(); i++ {
		dp := dps.At(i)
		fmt.Fprintf(b, "%sDataPoint #%d\n", indent, i)
		writeDataPointTimestamps(b, indent+"  ", dp.StartTimestamp(), dp.Timestamp())
		fmt.Fprintf(b, "%s  Count: %d\n", indent, dp.Count())
		if dp.HasSum() {
			fmt.Fprintf(b, "%s  Sum: %s\n", indent, strconv.FormatFloat(dp.Sum(), 'f', -1, 64))
		}
		if dp.HasMin() {
			fmt.Fprintf(b, "%s  Min: %s\n", indent, strconv.FormatFloat(dp.Min(), 'f', -1, 64))
		}
		if dp.HasMax() {
			fmt.Fprintf(b, "%s  Max: %s\n", indent, strconv.FormatFloat(dp.Max(), 'f', -1, 64))
		}
		writeAttributes(b, indent+"  Attributes:", dp.Attributes(), indent+"    ")
	}
}

func writeExponentialHistogramDataPoints(b *strings.Builder, indent string, dps pmetric.ExponentialHistogramDataPointSlice) {
	for i := 0; i < dps.Len(); i++ {
		dp := dps.At(i)
		fmt.Fprintf(b, "%sDataPoint #%d\n", indent, i)
		writeDataPointTimestamps(b, indent+"  ", dp.StartTimestamp(), dp.Timestamp())
		fmt.Fprintf(b, "%s  Count: %d\n", indent, dp.Count())
		fmt.Fprintf(b, "%s  Scale: %d\n", indent, dp.Scale())
		if dp.HasSum() {
			fmt.Fprintf(b, "%s  Sum: %s\n", indent, strconv.FormatFloat(dp.Sum(), 'f', -1, 64))
		}
		writeAttributes(b, indent+"  Attributes:", dp.Attributes(), indent+"    ")
	}
}

func writeSummaryDataPoints(b *strings.Builder, indent string, dps pmetric.SummaryDataPointSlice) {
	for i := 0; i < dps.Len(); i++ {
		dp := dps.At(i)
		fmt.Fprintf(b, "%sDataPoint #%d\n", indent, i)
		writeDataPointTimestamps(b, indent+"  ", dp.StartTimestamp(), dp.Timestamp())
		fmt.Fprintf(b, "%s  Count: %d\n", indent, dp.Count())
		fmt.Fprintf(b, "%s  Sum: %s\n", indent, strconv.FormatFloat(dp.Sum(), 'f', -1, 64))
		writeAttributes(b, indent+"  Attributes:", dp.Attributes(), indent+"    ")
	}
}

// writeDataPointTimestamps emits start and observed timestamps when set.
func writeDataPointTimestamps(b *strings.Builder, indent string, start, obs pcommon.Timestamp) {
	if t := start.AsTime(); !t.IsZero() {
		fmt.Fprintf(b, "%sStart time: %s\n", indent, t.UTC().Format(time.RFC3339Nano))
	}
	if t := obs.AsTime(); !t.IsZero() {
		fmt.Fprintf(b, "%sTimestamp: %s\n", indent, t.UTC().Format(time.RFC3339Nano))
	}
}

// writeSchemaURL writes "<indent>SchemaURL: <url>" when url is non-empty.
func writeSchemaURL(b *strings.Builder, indent, url string) {
	if url == "" {
		return
	}
	fmt.Fprintf(b, "%sSchemaURL: %s\n", indent, url)
}

// writeScope emits the instrumentation scope identifier and any attributes.
func writeScope(b *strings.Builder, indent string, sc pcommon.InstrumentationScope) {
	if sc.Name() != "" {
		fmt.Fprintf(b, "%sInstrumentationScope %s %s\n", indent, sc.Name(), sc.Version())
	}
	writeAttributes(b, indent+"Attributes:", sc.Attributes(), indent+"  ")
}

// writeAttributes prints a `header` line followed by an indented sorted list
// of `-> key: TypePrefix(value)` lines for each attribute. When the map is
// empty the header is suppressed.
func writeAttributes(b *strings.Builder, header string, m pcommon.Map, indent string) {
	if m.Len() == 0 {
		return
	}
	keys := make([]string, 0, m.Len())
	m.Range(func(k string, _ pcommon.Value) bool {
		keys = append(keys, k)
		return true
	})
	sort.Strings(keys)

	fmt.Fprintf(b, "%s\n", header)
	for _, k := range keys {
		v, _ := m.Get(k)
		fmt.Fprintf(b, "%s-> %s: %s\n", indent, k, renderValue(v))
	}
}

// renderValue returns the typed string form of a pcommon.Value. Matches the
// Collector debug exporter idiom: `Str("x")`, `Int(42)`, `Double(1.5)`,
// `Bool(true)`, `Bytes(0a1b…)`, `Map({…})`, `Slice([…])`, `Empty` for unset.
func renderValue(v pcommon.Value) string {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		return fmt.Sprintf("Str(%s)", strconv.Quote(v.Str()))
	case pcommon.ValueTypeInt:
		return fmt.Sprintf("Int(%d)", v.Int())
	case pcommon.ValueTypeDouble:
		return fmt.Sprintf("Double(%s)", strconv.FormatFloat(v.Double(), 'f', -1, 64))
	case pcommon.ValueTypeBool:
		return fmt.Sprintf("Bool(%t)", v.Bool())
	case pcommon.ValueTypeBytes:
		raw := v.Bytes().AsRaw()
		return fmt.Sprintf("Bytes(%s)", hex.EncodeToString(raw))
	case pcommon.ValueTypeMap:
		return fmt.Sprintf("Map(%s)", renderMap(v.Map()))
	case pcommon.ValueTypeSlice:
		return fmt.Sprintf("Slice(%s)", renderSlice(v.Slice()))
	case pcommon.ValueTypeEmpty:
		return "Empty"
	default:
		return "Unknown"
	}
}

func renderMap(m pcommon.Map) string {
	keys := make([]string, 0, m.Len())
	m.Range(func(k string, _ pcommon.Value) bool {
		keys = append(keys, k)
		return true
	})
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		v, _ := m.Get(k)
		parts = append(parts, fmt.Sprintf("%s:%s", k, renderValue(v)))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func renderSlice(s pcommon.Slice) string {
	parts := make([]string, 0, s.Len())
	for i := 0; i < s.Len(); i++ {
		parts = append(parts, renderValue(s.At(i)))
	}
	return "[" + strings.Join(parts, ",") + "]"
}
