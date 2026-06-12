package otlp

import (
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestRenderLogs_Empty(t *testing.T) {
	if got := RenderLogs(plog.NewLogs()); got != "" {
		t.Errorf("RenderLogs(empty) = %q; want empty string", got)
	}
}

func TestRenderTraces_Empty(t *testing.T) {
	if got := RenderTraces(ptrace.NewTraces()); got != "" {
		t.Errorf("RenderTraces(empty) = %q; want empty string", got)
	}
}

func TestRenderMetrics_Empty(t *testing.T) {
	if got := RenderMetrics(pmetric.NewMetrics()); got != "" {
		t.Errorf("RenderMetrics(empty) = %q; want empty string", got)
	}
}

func TestRenderLogs_HappyPath(t *testing.T) {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "my-service")
	sl := rl.ScopeLogs().AppendEmpty()
	sl.Scope().SetName("my-scope")
	sl.Scope().SetVersion("1.2.3")
	lr := sl.LogRecords().AppendEmpty()
	lr.SetTimestamp(pcommon.NewTimestampFromTime(time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)))
	lr.SetSeverityNumber(plog.SeverityNumberInfo)
	lr.SetSeverityText("INFO")
	lr.Body().SetStr("hello world")
	lr.Attributes().PutStr("http.method", "GET")
	lr.Attributes().PutInt("http.status_code", 200)

	got := RenderLogs(ld)
	mustContain(t, got, []string{
		"ResourceLogs #0",
		"Resource attributes:",
		"-> service.name: Str(\"my-service\")",
		"ScopeLogs #0",
		"InstrumentationScope my-scope 1.2.3",
		"LogRecord #0",
		"Timestamp: 2026-06-11T10:00:00Z",
		"SeverityText: INFO",
		"SeverityNumber: 9",
		"Body: Str(\"hello world\")",
		"-> http.method: Str(\"GET\")",
		"-> http.status_code: Int(200)",
	})
}

func TestRenderLogs_TraceContext(t *testing.T) {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	lr := sl.LogRecords().AppendEmpty()
	tid := pcommon.TraceID([16]byte{0x0a, 0xf7, 0x65, 0x19, 0x16, 0xcd, 0x43, 0xdd, 0x84, 0x48, 0xeb, 0x21, 0x1c, 0x80, 0x31, 0x9c})
	sid := pcommon.SpanID([8]byte{0xb7, 0xad, 0x6b, 0x71, 0x69, 0x20, 0x33, 0x31})
	lr.SetTraceID(tid)
	lr.SetSpanID(sid)
	lr.Body().SetStr("traced")

	got := RenderLogs(ld)
	mustContain(t, got, []string{
		"Trace ID: 0af7651916cd43dd8448eb211c80319c",
		"Span ID: b7ad6b7169203331",
	})
}

func TestRenderTraces_HappyPath(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "frontend")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("net/http")
	sp := ss.Spans().AppendEmpty()
	tid := pcommon.TraceID([16]byte{0x0a, 0xf7, 0x65, 0x19, 0x16, 0xcd, 0x43, 0xdd, 0x84, 0x48, 0xeb, 0x21, 0x1c, 0x80, 0x31, 0x9c})
	sid := pcommon.SpanID([8]byte{0xb7, 0xad, 0x6b, 0x71, 0x69, 0x20, 0x33, 0x31})
	pid := pcommon.SpanID([8]byte{0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7})
	sp.SetTraceID(tid)
	sp.SetSpanID(sid)
	sp.SetParentSpanID(pid)
	sp.SetName("GET /api/users")
	sp.SetKind(ptrace.SpanKindServer)
	sp.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)))
	sp.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Date(2026, 6, 11, 10, 0, 0, 150_000_000, time.UTC)))
	sp.Status().SetCode(ptrace.StatusCodeOk)
	sp.Attributes().PutStr("http.method", "GET")
	sp.Attributes().PutInt("http.status_code", 200)

	got := RenderTraces(td)
	mustContain(t, got, []string{
		"ResourceSpans #0",
		"-> service.name: Str(\"frontend\")",
		"InstrumentationScope net/http",
		"Span #0",
		"Trace ID: 0af7651916cd43dd8448eb211c80319c",
		"Parent ID: 00f067aa0ba902b7",
		"Span ID: b7ad6b7169203331",
		"Name: GET /api/users",
		"Kind: Server",
		"Start time: 2026-06-11T10:00:00Z",
		"End time: 2026-06-11T10:00:00.15Z",
		"Status code: Ok",
		"-> http.method: Str(\"GET\")",
		"-> http.status_code: Int(200)",
	})
}

func TestRenderTraces_EventsAndLinks(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	sp := ss.Spans().AppendEmpty()
	sp.SetName("test")

	ev := sp.Events().AppendEmpty()
	ev.SetName("exception")
	ev.SetTimestamp(pcommon.NewTimestampFromTime(time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)))
	ev.Attributes().PutStr("exception.type", "ValueError")

	link := sp.Links().AppendEmpty()
	link.SetTraceID(pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
	link.SetSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8}))
	link.Attributes().PutStr("link.reason", "follow_from")

	got := RenderTraces(td)
	mustContain(t, got, []string{
		"Events:",
		"Event #0",
		"Name: exception",
		"-> exception.type: Str(\"ValueError\")",
		"Links:",
		"Link #0",
		"Trace ID: 0102030405060708090a0b0c0d0e0f10",
		"Span ID: 0102030405060708",
		"-> link.reason: Str(\"follow_from\")",
	})
}

func TestRenderMetrics_Gauge(t *testing.T) {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	m := sm.Metrics().AppendEmpty()
	m.SetName("cpu.usage")
	m.SetUnit("%")
	m.SetDescription("CPU usage percent")
	gauge := m.SetEmptyGauge()
	dp := gauge.DataPoints().AppendEmpty()
	dp.SetDoubleValue(42.5)
	dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)))
	dp.Attributes().PutStr("cpu", "0")

	got := RenderMetrics(md)
	mustContain(t, got, []string{
		"ResourceMetrics #0",
		"Metric #0",
		"Name: cpu.usage",
		"Description: CPU usage percent",
		"Unit: %",
		"Kind: Gauge",
		"DataPoint #0",
		"Value: Double(42.5)",
		"-> cpu: Str(\"0\")",
	})
}

func TestRenderMetrics_SumIsMonotonic(t *testing.T) {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	m := sm.Metrics().AppendEmpty()
	m.SetName("requests.total")
	sum := m.SetEmptySum()
	sum.SetIsMonotonic(true)
	sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	dp := sum.DataPoints().AppendEmpty()
	dp.SetIntValue(1234)

	got := RenderMetrics(md)
	mustContain(t, got, []string{
		"Name: requests.total",
		"Kind: Sum",
		"IsMonotonic: true",
		"AggregationTemporality: Cumulative",
		"Value: Int(1234)",
	})
}

func TestRenderMetrics_Histogram(t *testing.T) {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	m := sm.Metrics().AppendEmpty()
	m.SetName("latency")
	hist := m.SetEmptyHistogram()
	hist.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp := hist.DataPoints().AppendEmpty()
	dp.SetCount(100)
	dp.SetSum(2500.0)
	dp.SetMin(0.5)
	dp.SetMax(120.0)

	got := RenderMetrics(md)
	mustContain(t, got, []string{
		"Name: latency",
		"Kind: Histogram",
		"AggregationTemporality: Delta",
		"Count: 100",
		"Sum: 2500",
		"Min: 0.5",
		"Max: 120",
	})
}

func TestRenderValue_AllTypes(t *testing.T) {
	cases := []struct {
		name string
		set  func(v pcommon.Value)
		want string
	}{
		{"string", func(v pcommon.Value) { v.SetStr("hello") }, `Str("hello")`},
		{"string with quotes", func(v pcommon.Value) { v.SetStr(`he said "hi"`) }, `Str("he said \"hi\"")`},
		{"int", func(v pcommon.Value) { v.SetInt(42) }, `Int(42)`},
		{"double", func(v pcommon.Value) { v.SetDouble(3.14) }, `Double(3.14)`},
		{"bool true", func(v pcommon.Value) { v.SetBool(true) }, `Bool(true)`},
		{"bool false", func(v pcommon.Value) { v.SetBool(false) }, `Bool(false)`},
		{"bytes", func(v pcommon.Value) { v.SetEmptyBytes().FromRaw([]byte{0xde, 0xad, 0xbe, 0xef}) }, `Bytes(deadbeef)`},
		{"empty", func(_ pcommon.Value) {}, `Empty`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := pcommon.NewValueEmpty()
			tc.set(v)
			if got := renderValue(v); got != tc.want {
				t.Errorf("renderValue(%s) = %q; want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestRenderValue_MapAndSlice(t *testing.T) {
	v := pcommon.NewValueMap()
	m := v.Map()
	m.PutStr("b", "second")
	m.PutInt("a", 1)
	got := renderValue(v)
	// Keys must be sorted alphabetically — `a` before `b`.
	want := `Map({a:Int(1),b:Str("second")})`
	if got != want {
		t.Errorf("renderValue(map) = %q; want %q", got, want)
	}

	v = pcommon.NewValueSlice()
	s := v.Slice()
	s.AppendEmpty().SetStr("first")
	s.AppendEmpty().SetInt(2)
	got = renderValue(v)
	want = `Slice([Str("first"),Int(2)])`
	if got != want {
		t.Errorf("renderValue(slice) = %q; want %q", got, want)
	}
}

func TestRenderAttributes_SortedAlphabetically(t *testing.T) {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	lr := sl.LogRecords().AppendEmpty()
	lr.Attributes().PutStr("zeta", "z")
	lr.Attributes().PutStr("alpha", "a")
	lr.Attributes().PutStr("mu", "m")

	got := RenderLogs(ld)
	posAlpha := strings.Index(got, "-> alpha:")
	posMu := strings.Index(got, "-> mu:")
	posZeta := strings.Index(got, "-> zeta:")
	if !(posAlpha < posMu && posMu < posZeta) {
		t.Errorf("attributes should render in alpha order; got alpha@%d mu@%d zeta@%d", posAlpha, posMu, posZeta)
	}
}

func TestRenderLogs_EmptyAttributesSuppressed(t *testing.T) {
	// A LogRecord with no attributes should not produce an empty
	// "Attributes:" header line.
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	lr := sl.LogRecords().AppendEmpty()
	lr.Body().SetStr("no attrs")

	got := RenderLogs(ld)
	if strings.Contains(got, "Attributes:\n") {
		t.Errorf("empty-attributes header should be suppressed; got:\n%s", got)
	}
}

// mustContain asserts every needle appears in haystack. Reports all missing
// substrings on first failure so the developer fixes them in one pass rather
// than chasing each in turn.
func mustContain(t *testing.T, haystack string, needles []string) {
	t.Helper()
	var missing []string
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		t.Errorf("missing substrings in rendered output:\n  %s\n\nfull output:\n%s",
			strings.Join(missing, "\n  "), haystack)
	}
}
