package otlp

import (
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestDecorator_NilReceiverIsNoOp(t *testing.T) {
	// Calling DecorateX on a nil *Decorator must not panic; the proxy's
	// worker pool passes a nil decorator when no flags were provided.
	var d *Decorator
	d.DecorateLogs(plog.NewLogs())
	d.DecorateTraces(ptrace.NewTraces())
	d.DecorateMetrics(pmetric.NewMetrics())
}

func TestDecorator_IsEmpty(t *testing.T) {
	if !(*Decorator)(nil).IsEmpty() {
		t.Error("nil decorator should be empty")
	}
	if !NewDecorator(nil, nil, "", "", nil, nil, nil).IsEmpty() {
		t.Error("decorator with zero fields should be empty")
	}
	if NewDecorator(map[string]string{"k": "v"}, nil, "", "", nil, nil, nil).IsEmpty() {
		t.Error("decorator with a resource attr should not be empty")
	}
	if NewDecorator(nil, map[string]string{"k": "v"}, "", "", nil, nil, nil).IsEmpty() {
		t.Error("decorator with a scope attr should not be empty")
	}
	if NewDecorator(nil, nil, "name", "", nil, nil, nil).IsEmpty() {
		t.Error("decorator with scope name should not be empty")
	}
	if NewDecorator(nil, nil, "", "ver", nil, nil, nil).IsEmpty() {
		t.Error("decorator with scope version should not be empty")
	}
	if NewDecorator(nil, nil, "", "", map[string]string{"k": "v"}, nil, nil).IsEmpty() {
		t.Error("decorator with a log attr should not be empty")
	}
	if NewDecorator(nil, nil, "", "", nil, map[string]string{"k": "v"}, nil).IsEmpty() {
		t.Error("decorator with a span attr should not be empty")
	}
	if NewDecorator(nil, nil, "", "", nil, nil, map[string]string{"k": "v"}).IsEmpty() {
		t.Error("decorator with a metric attr should not be empty")
	}
}

func TestDecorator_UpsertsResourceAttributes(t *testing.T) {
	d := NewDecorator(
		map[string]string{
			"deployment.environment.name": "dev",
			"service.name":                "override", // overwrites SDK-set
		}, nil, "", "", nil, nil, nil)

	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "from-sdk")
	rl.Resource().Attributes().PutStr("service.instance.id", "sdk-instance")
	rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()

	d.DecorateLogs(ld)

	got := rl.Resource().Attributes()
	if v, _ := got.Get("deployment.environment.name"); v.AsString() != "dev" {
		t.Errorf("missing or wrong deployment.environment.name; got %v", v.AsString())
	}
	if v, _ := got.Get("service.name"); v.AsString() != "override" {
		t.Errorf("service.name should be overwritten to 'override'; got %v", v.AsString())
	}
	if v, _ := got.Get("service.instance.id"); v.AsString() != "sdk-instance" {
		t.Errorf("untouched SDK attribute lost; got %v", v.AsString())
	}
}

func TestDecorator_UpsertsScopeAttributes(t *testing.T) {
	d := NewDecorator(nil, map[string]string{"session.id": "abc"}, "", "", nil, nil, nil)

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().Attributes().PutStr("preexisting", "kept")
	ss.Spans().AppendEmpty().SetName("test-span")

	d.DecorateTraces(td)

	gotAttrs := ss.Scope().Attributes()
	if v, _ := gotAttrs.Get("session.id"); v.AsString() != "abc" {
		t.Errorf("scope.session.id not set; got %v", v.AsString())
	}
	if v, _ := gotAttrs.Get("preexisting"); v.AsString() != "kept" {
		t.Errorf("preexisting scope attr was clobbered; got %v", v.AsString())
	}
}

func TestDecorator_SetsScopeNameAndVersionOnlyWhenProvided(t *testing.T) {
	// When ScopeName is empty, the SDK's name must survive unchanged.
	// Same for ScopeVersion.
	d := NewDecorator(nil, nil, "", "", nil, nil, nil)

	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("io.opentelemetry.metric-sdk")
	sm.Scope().SetVersion("1.30.0")
	sm.Metrics().AppendEmpty().SetEmptyGauge().DataPoints().AppendEmpty().SetIntValue(1)

	d.DecorateMetrics(md)

	if got := sm.Scope().Name(); got != "io.opentelemetry.metric-sdk" {
		t.Errorf("empty ScopeName should not overwrite; got %q", got)
	}
	if got := sm.Scope().Version(); got != "1.30.0" {
		t.Errorf("empty ScopeVersion should not overwrite; got %q", got)
	}

	// When explicitly set, both must overwrite.
	d2 := NewDecorator(nil, nil, "my-app", "v9", nil, nil, nil)
	d2.DecorateMetrics(md)
	if got := sm.Scope().Name(); got != "my-app" {
		t.Errorf("explicit ScopeName should overwrite; got %q", got)
	}
	if got := sm.Scope().Version(); got != "v9" {
		t.Errorf("explicit ScopeVersion should overwrite; got %q", got)
	}
}

func TestDecorator_AppliesToAllResourcesAndScopes(t *testing.T) {
	// A batch with multiple ResourceLogs entries and multiple ScopeLogs
	// per resource — every one must receive the upsert.
	d := NewDecorator(
		map[string]string{"r.k": "r.v"},
		map[string]string{"s.k": "s.v"},
		"", "", nil, nil, nil)

	ld := plog.NewLogs()
	for i := 0; i < 2; i++ {
		rl := ld.ResourceLogs().AppendEmpty()
		for j := 0; j < 3; j++ {
			rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
		}
	}

	d.DecorateLogs(ld)

	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		rl := ld.ResourceLogs().At(i)
		if v, _ := rl.Resource().Attributes().Get("r.k"); v.AsString() != "r.v" {
			t.Errorf("resource[%d] missing r.k", i)
		}
		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)
			if v, _ := sl.Scope().Attributes().Get("s.k"); v.AsString() != "s.v" {
				t.Errorf("resource[%d].scope[%d] missing s.k", i, j)
			}
		}
	}
}

func TestDecorator_UpsertsLogAttributes(t *testing.T) {
	// --log-attribute upserts onto each LogRecord's attributes, not the
	// resource or scope. Verify the right level gets the key.
	d := NewDecorator(nil, nil, "", "",
		map[string]string{"http.target": "/api/health"},
		nil, nil)

	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	lr := sl.LogRecords().AppendEmpty()
	lr.Attributes().PutStr("preexisting", "kept")
	lr.Body().SetStr("hi")

	d.DecorateLogs(ld)

	gotAttrs := lr.Attributes()
	if v, _ := gotAttrs.Get("http.target"); v.AsString() != "/api/health" {
		t.Errorf("log record missing http.target; got %v", v.AsString())
	}
	if v, _ := gotAttrs.Get("preexisting"); v.AsString() != "kept" {
		t.Errorf("preexisting log attr was clobbered; got %v", v.AsString())
	}
	// Resource and scope must NOT have the log attribute.
	if _, ok := rl.Resource().Attributes().Get("http.target"); ok {
		t.Error("log attr leaked into resource attributes")
	}
	if _, ok := sl.Scope().Attributes().Get("http.target"); ok {
		t.Error("log attr leaked into scope attributes")
	}
}

func TestDecorator_UpsertsSpanAttributes(t *testing.T) {
	d := NewDecorator(nil, nil, "", "",
		nil,
		map[string]string{"http.route": "/widget/:id"},
		nil)

	td := ptrace.NewTraces()
	ss := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("GET /widget/:id")
	span.Attributes().PutStr("preexisting", "kept")

	d.DecorateTraces(td)

	gotAttrs := span.Attributes()
	if v, _ := gotAttrs.Get("http.route"); v.AsString() != "/widget/:id" {
		t.Errorf("span missing http.route; got %v", v.AsString())
	}
	if v, _ := gotAttrs.Get("preexisting"); v.AsString() != "kept" {
		t.Errorf("preexisting span attr was clobbered; got %v", v.AsString())
	}
}

func TestDecorator_UpsertsMetricAttributes_AllFlavors(t *testing.T) {
	// pmetric supports five metric flavors; the decorator must upsert
	// onto every data point's attributes regardless of which flavor.
	d := NewDecorator(nil, nil, "", "", nil, nil,
		map[string]string{"region": "us-east-1"})

	md := pmetric.NewMetrics()
	sm := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty()

	// One metric of each flavor with a single data point.
	gauge := sm.Metrics().AppendEmpty()
	gauge.SetName("g")
	gauge.SetEmptyGauge().DataPoints().AppendEmpty().SetIntValue(1)

	sum := sm.Metrics().AppendEmpty()
	sum.SetName("s")
	sum.SetEmptySum().DataPoints().AppendEmpty().SetDoubleValue(2.0)

	hist := sm.Metrics().AppendEmpty()
	hist.SetName("h")
	hist.SetEmptyHistogram().DataPoints().AppendEmpty().SetCount(3)

	exph := sm.Metrics().AppendEmpty()
	exph.SetName("e")
	exph.SetEmptyExponentialHistogram().DataPoints().AppendEmpty().SetCount(4)

	summ := sm.Metrics().AppendEmpty()
	summ.SetName("u")
	summ.SetEmptySummary().DataPoints().AppendEmpty().SetCount(5)

	d.DecorateMetrics(md)

	// Walk every data point of every metric and assert the region tag is
	// present.
	check := func(name string, get func() (string, bool)) {
		val, ok := get()
		if !ok {
			t.Errorf("%s data point missing 'region' attribute", name)
			return
		}
		if val != "us-east-1" {
			t.Errorf("%s data point region = %q; want us-east-1", name, val)
		}
	}
	pull := func(attrs interface {
		Get(string) (pcommon.Value, bool)
	}) func() (string, bool) {
		return func() (string, bool) {
			v, ok := attrs.Get("region")
			return v.AsString(), ok
		}
	}
	check("gauge", pull(gauge.Gauge().DataPoints().At(0).Attributes()))
	check("sum", pull(sum.Sum().DataPoints().At(0).Attributes()))
	check("histogram", pull(hist.Histogram().DataPoints().At(0).Attributes()))
	check("exp-histogram", pull(exph.ExponentialHistogram().DataPoints().At(0).Attributes()))
	check("summary", pull(summ.Summary().DataPoints().At(0).Attributes()))
}

func TestDecorator_PerSignalAttrsDoNotCrossSignals(t *testing.T) {
	// A LogAttribute must not appear on spans; a SpanAttribute must not
	// appear on log records; a MetricAttribute must not appear on
	// either. The decorator applies each map only at its signal's
	// record level.
	d := NewDecorator(nil, nil, "", "",
		map[string]string{"only.on.log": "true"},
		map[string]string{"only.on.span": "true"},
		map[string]string{"only.on.metric": "true"},
	)

	ld := plog.NewLogs()
	logRec := ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	d.DecorateLogs(ld)
	if _, ok := logRec.Attributes().Get("only.on.span"); ok {
		t.Error("span attr leaked into log record")
	}
	if _, ok := logRec.Attributes().Get("only.on.metric"); ok {
		t.Error("metric attr leaked into log record")
	}

	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	d.DecorateTraces(td)
	if _, ok := span.Attributes().Get("only.on.log"); ok {
		t.Error("log attr leaked into span")
	}
	if _, ok := span.Attributes().Get("only.on.metric"); ok {
		t.Error("metric attr leaked into span")
	}

	md := pmetric.NewMetrics()
	m := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	dp := m.SetEmptyGauge().DataPoints().AppendEmpty()
	d.DecorateMetrics(md)
	if _, ok := dp.Attributes().Get("only.on.log"); ok {
		t.Error("log attr leaked into metric data point")
	}
	if _, ok := dp.Attributes().Get("only.on.span"); ok {
		t.Error("span attr leaked into metric data point")
	}
}

func TestDecorator_EmptyBatchIsSafe(t *testing.T) {
	// An empty plog.Logs (no resources) should not panic when decorated.
	d := NewDecorator(map[string]string{"k": "v"}, nil, "", "", nil, nil, nil)
	d.DecorateLogs(plog.NewLogs())
	d.DecorateTraces(ptrace.NewTraces())
	d.DecorateMetrics(pmetric.NewMetrics())
}
