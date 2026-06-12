package otlp

import (
	"testing"

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
	if !NewDecorator(nil, nil, "", "").IsEmpty() {
		t.Error("decorator with zero fields should be empty")
	}
	if NewDecorator(map[string]string{"k": "v"}, nil, "", "").IsEmpty() {
		t.Error("decorator with a resource attr should not be empty")
	}
	if NewDecorator(nil, map[string]string{"k": "v"}, "", "").IsEmpty() {
		t.Error("decorator with a scope attr should not be empty")
	}
	if NewDecorator(nil, nil, "name", "").IsEmpty() {
		t.Error("decorator with scope name should not be empty")
	}
	if NewDecorator(nil, nil, "", "ver").IsEmpty() {
		t.Error("decorator with scope version should not be empty")
	}
}

func TestDecorator_UpsertsResourceAttributes(t *testing.T) {
	d := NewDecorator(
		map[string]string{
			"deployment.environment.name": "dev",
			"service.name":                "override", // overwrites SDK-set
		}, nil, "", "")

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
	d := NewDecorator(nil, map[string]string{"session.id": "abc"}, "", "")

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
	d := NewDecorator(nil, nil, "", "")

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
	d2 := NewDecorator(nil, nil, "my-app", "v9")
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
		"", "")

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

func TestDecorator_EmptyBatchIsSafe(t *testing.T) {
	// An empty plog.Logs (no resources) should not panic when decorated.
	d := NewDecorator(map[string]string{"k": "v"}, nil, "", "")
	d.DecorateLogs(plog.NewLogs())
	d.DecorateTraces(ptrace.NewTraces())
	d.DecorateMetrics(pmetric.NewMetrics())
}
