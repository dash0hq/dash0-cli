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
}

// NewDecorator returns a Decorator with the supplied user-controlled
// upserts. Any zero-valued field is a no-op when Decorate* is called.
func NewDecorator(resourceAttrs, scopeAttrs map[string]string, scopeName, scopeVersion string) *Decorator {
	return &Decorator{
		resourceAttrs: resourceAttrs,
		scopeAttrs:    scopeAttrs,
		scopeName:     scopeName,
		scopeVersion:  scopeVersion,
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
			d.scopeVersion == "")
}

// DecorateLogs upserts the configured fields onto each ResourceLogs +
// ScopeLogs pair in ld.
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
			d.upsertScope(sls.At(j).Scope())
		}
	}
}

// DecorateTraces upserts the configured fields onto each ResourceSpans +
// ScopeSpans pair in td.
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
			d.upsertScope(sss.At(j).Scope())
		}
	}
}

// DecorateMetrics upserts the configured fields onto each ResourceMetrics
// + ScopeMetrics pair in md.
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
			d.upsertScope(sms.At(j).Scope())
		}
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
