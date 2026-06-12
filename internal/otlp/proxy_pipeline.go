package otlp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"

	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
)

// proxyListenHost is the bind address for both HTTP and gRPC listeners. v1
// is localhost-only (KTD7) — the proxy is a local-dev shortcut, not a
// network-exposed gateway. Promoting to wildcard bind requires an explicit
// future flag with auth gating.
const proxyListenHost = "127.0.0.1"

// proxyComponentName is the name suffix for the receiver's component ID.
// The full ID is `otlp/<name>` — the otlpreceiver factory validates that
// the ID's type half equals its own type ("otlp"), so we must use the
// factory's type and only customize the name. The otlpreceiver uses
// sharedcomponent.LoadOrStore keyed on (cfg, id) so all three signal
// receivers share one underlying instance.
const proxyComponentName = "dash0_proxy"

// proxyGRPCMaxRecvMiB caps inbound gRPC messages at 16 MiB. Matches the
// Collector's own default; large enough for realistic OTLP batches, small
// enough that a single batch can't exhaust memory on a developer machine.
const proxyGRPCMaxRecvMiB = 16

// PipelineEndpoints reports the actual host:port each listener bound to,
// after pre-bind fallback to OS-assigned ports. The supervisor (U5) uses
// these to compose the banner and the agent-mode `started` event.
type PipelineEndpoints struct {
	HTTPEndpoint string // e.g. "127.0.0.1:4318" or "127.0.0.1:53291"
	GRPCEndpoint string // e.g. "127.0.0.1:4317"
}

// Pipeline owns the otlpreceiver-based listener stack. Construction
// pre-binds ports (so the supervisor can decide collision behavior before
// the receiver internals fire), builds the receiver Config, and creates
// the three signal-receiver instances (logs/traces/metrics). Start
// transfers control to the receiver, which actually binds the HTTP and
// gRPC servers.
type Pipeline struct {
	endpoints PipelineEndpoints
	host      component.Host

	// receivers all wrap the same underlying *otlpReceiver via
	// sharedcomponent.LoadOrStore. Holding all three lets us shut them all
	// down in case the receiver's lifecycle ever changes to per-signal.
	logsRecv    receiver.Logs
	tracesRecv  receiver.Traces
	metricsRecv receiver.Metrics
}

// BuildPipeline pre-binds the requested ports (falling back per
// PortRequest semantics), assembles the receiver Config, and constructs
// the three signal receivers wired to the supplied consumer. The returned
// Pipeline has not started any servers yet; call Start to bind the
// listeners and accept traffic.
//
// On failure mid-construction the function returns an error; no resources
// are leaked because pre-bound listeners are closed before construction
// returns.
func BuildPipeline(ctx context.Context, httpPort, grpcPort int, consumer *ProxyConsumer) (*Pipeline, error) {
	resolvedHTTP, err := preBindPort("http", httpPort, defaultHTTPPort)
	if err != nil {
		return nil, err
	}
	resolvedGRPC, err := preBindPort("grpc", grpcPort, defaultGRPCPort)
	if err != nil {
		return nil, err
	}

	cfg := buildReceiverConfig(resolvedHTTP, resolvedGRPC)
	settings := newReceiverSettings()

	factory := otlpreceiver.NewFactory()
	logsRecv, err := factory.CreateLogs(ctx, settings, cfg, consumer)
	if err != nil {
		return nil, fmt.Errorf("create OTLP logs receiver: %w", err)
	}
	tracesRecv, err := factory.CreateTraces(ctx, settings, cfg, consumer)
	if err != nil {
		return nil, fmt.Errorf("create OTLP traces receiver: %w", err)
	}
	metricsRecv, err := factory.CreateMetrics(ctx, settings, cfg, consumer)
	if err != nil {
		return nil, fmt.Errorf("create OTLP metrics receiver: %w", err)
	}

	return &Pipeline{
		endpoints: PipelineEndpoints{
			HTTPEndpoint: net.JoinHostPort(proxyListenHost, strconv.Itoa(resolvedHTTP)),
			GRPCEndpoint: net.JoinHostPort(proxyListenHost, strconv.Itoa(resolvedGRPC)),
		},
		host:        newNopHost(),
		logsRecv:    logsRecv,
		tracesRecv:  tracesRecv,
		metricsRecv: metricsRecv,
	}, nil
}

// Endpoints returns the bound HTTP and gRPC host:port pairs. Stable after
// BuildPipeline returns successfully.
func (p *Pipeline) Endpoints() PipelineEndpoints {
	return p.endpoints
}

// Start brings up both HTTP and gRPC listeners via the receiver. Because
// the three signal-receivers returned by the otlpreceiver factory share a
// single underlying instance (sharedcomponent semantics), one Start call
// is sufficient — but we Start all three for symmetry and to surface any
// future divergence in the upstream library quickly.
//
// On any partial-start failure, Start best-effort-Shutdowns the others
// and returns the first error. The caller's supervisor (U5) treats this
// as a fatal startup error per KTD5.
func (p *Pipeline) Start(ctx context.Context) error {
	if err := p.logsRecv.Start(ctx, p.host); err != nil {
		return fmt.Errorf("start OTLP logs receiver: %w", err)
	}
	if err := p.tracesRecv.Start(ctx, p.host); err != nil {
		_ = p.logsRecv.Shutdown(ctx)
		return fmt.Errorf("start OTLP traces receiver: %w", err)
	}
	if err := p.metricsRecv.Start(ctx, p.host); err != nil {
		_ = p.tracesRecv.Shutdown(ctx)
		_ = p.logsRecv.Shutdown(ctx)
		return fmt.Errorf("start OTLP metrics receiver: %w", err)
	}
	return nil
}

// Shutdown stops both listeners. Safe to call multiple times and without a
// prior Start (per the component.Component contract).
func (p *Pipeline) Shutdown(ctx context.Context) error {
	// Tear down in reverse-start order so any cleanup ordering assumptions
	// in the upstream library are respected.
	var firstErr error
	if err := p.metricsRecv.Shutdown(ctx); err != nil {
		firstErr = err
	}
	if err := p.tracesRecv.Shutdown(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := p.logsRecv.Shutdown(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if firstErr != nil {
		return fmt.Errorf("shutdown OTLP receivers: %w", firstErr)
	}
	return nil
}

// preBindPort resolves the actual port the proxy should advertise. When
// `requested` is the default, a port-in-use collision falls back to an
// OS-assigned port (per KTD4 / AE3). When `requested` is an explicit
// non-default, a collision is fatal — the user asked for this specific
// port and silently moving would surprise them.
//
// Implementation strategy: bind, capture the actual port (which equals
// `requested` on success or an OS-assigned port after fallback), then
// close the listener so the receiver internals can re-bind. The race
// window between our close and the receiver's bind is microseconds; on
// 127.0.0.1 the OS won't reassign that port to anything else in that
// window in practice. The plan acknowledges this tradeoff in OQ5.
func preBindPort(label string, requested, defaultPort int) (int, error) {
	address := net.JoinHostPort(proxyListenHost, strconv.Itoa(requested))
	ln, err := net.Listen("tcp", address)
	if err != nil {
		if requested != defaultPort {
			return 0, fmt.Errorf("bind %s port %d: %w", label, requested, err)
		}
		// Fallback for the default port: try OS-assigned.
		fallback, fallbackErr := net.Listen("tcp", net.JoinHostPort(proxyListenHost, "0"))
		if fallbackErr != nil {
			return 0, fmt.Errorf("bind %s default port %d failed (%v) and OS-assigned fallback also failed: %w",
				label, defaultPort, err, fallbackErr)
		}
		port := fallback.Addr().(*net.TCPAddr).Port
		_ = fallback.Close()
		return port, nil
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port, nil
}

// buildReceiverConfig assembles an otlpreceiver.Config from scratch with
// our resolved endpoints, no auth, no TLS, and a pinned 16 MiB gRPC
// max-recv. URL paths inherit the OTLP spec defaults
// (/v1/logs, /v1/traces, /v1/metrics) — we set them explicitly because
// the receiver does not back-fill empty SanitizedURLPath values at start
// time.
//
// Why we do not start from the factory's default config: the factory
// wraps both protocol blocks in configoptional.Default(), and Default
// flavor's HasValue() returns false — Get() on such a value yields nil.
// Some flavor is the one the receiver iterates over when it builds
// servers, so we construct fresh ServerConfigs and wrap them in
// configoptional.Some directly.
func buildReceiverConfig(httpPort, grpcPort int) *otlpreceiver.Config {
	httpServer := confighttp.NewDefaultServerConfig()
	httpServer.NetAddr.Endpoint = net.JoinHostPort(proxyListenHost, strconv.Itoa(httpPort))

	grpcServer := configgrpc.NewDefaultServerConfig()
	grpcServer.NetAddr.Endpoint = net.JoinHostPort(proxyListenHost, strconv.Itoa(grpcPort))
	grpcServer.MaxRecvMsgSizeMiB = proxyGRPCMaxRecvMiB

	httpCfg := otlpreceiver.HTTPConfig{
		ServerConfig:   httpServer,
		LogsURLPath:    "/v1/logs",
		TracesURLPath:  "/v1/traces",
		MetricsURLPath: "/v1/metrics",
	}

	return &otlpreceiver.Config{
		Protocols: otlpreceiver.Protocols{
			GRPC: configoptional.Some(grpcServer),
			HTTP: configoptional.Some(httpCfg),
		},
	}
}

// newReceiverSettings constructs a receiver.Settings populated with
// nop-everything telemetry (the proxy doesn't need the receiver to report
// its own metrics; we have our own Stats). The ID is stable so the
// sharedcomponent map inside otlpreceiver gives us a single shared
// instance across all three CreateX calls.
func newReceiverSettings() receiver.Settings {
	return receiver.Settings{
		ID: component.MustNewIDWithName("otlp", proxyComponentName),
		TelemetrySettings: component.TelemetrySettings{
			Logger:         zap.NewNop(),
			TracerProvider: nooptrace.NewTracerProvider(),
			MeterProvider:  noopmetric.NewMeterProvider(),
			Resource:       pcommon.NewResource(),
		},
		BuildInfo: component.BuildInfo{
			Command:     "dash0",
			Description: "Dash0 CLI OTLP proxy",
			Version:     "v1",
		},
	}
}

// nopHost is a minimal component.Host implementation. The otlpreceiver
// only calls GetExtensions on the host (verified by reading the upstream
// source); we return an empty map. Patterned on
// componenttest.NewNopHost, but inlined to avoid pulling a test-only
// package into production code.
type nopHost struct{}

func newNopHost() component.Host { return &nopHost{} }

func (nopHost) GetExtensions() map[component.ID]component.Component {
	return map[component.ID]component.Component{}
}

// errPipelineUnbuilt is returned when callers try to use Pipeline methods
// before a successful BuildPipeline. Reserved for future use; the current
// builder returns a fully-initialized struct or an error.
var errPipelineUnbuilt = errors.New("pipeline not built")

var _ component.Host = nopHost{}
var _ error = errPipelineUnbuilt
