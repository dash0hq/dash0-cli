package otlp

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dash0hq/dash0-cli/internal/experimental"
	"github.com/spf13/cobra"
)

// Default values for proxy flags.
const (
	defaultHTTPPort      = 4318
	defaultGRPCPort      = 4317
	defaultStatsInterval = time.Second
	defaultMaxConcurrent = 10

	// maxAllowedConcurrent matches dash0-api-client-go's hard ceiling.
	maxAllowedConcurrent = 10
)

// Environment-variable names for proxy flag overrides.
//
// Precedence (high to low) on each port flag:
//
//  1. explicit --flag on the command line
//  2. DASH0_OTLP_PROXY_* env var
//  3. OTEL_EXPORTER_OTLP_ENDPOINT (parsed; routed to HTTP or gRPC based on
//     OTEL_EXPORTER_OTLP_PROTOCOL)
//  4. built-in default (4318 HTTP, 4317 gRPC)
const (
	envHTTPPort      = "DASH0_OTLP_PROXY_HTTP_PORT"
	envGRPCPort      = "DASH0_OTLP_PROXY_GRPC_PORT"
	envMaxConcurrent = "DASH0_OTLP_PROXY_MAX_CONCURRENT"

	envOTELEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"
	envOTELProtocol = "OTEL_EXPORTER_OTLP_PROTOCOL"
)

// proxyFlags captures all CLI flags for the `dash0 otlp proxy` command.
//
// The flag struct is the single source of truth: env-var overrides are
// resolved into this struct post-parse via resolveEnvOverrides, and the
// supervisor reads only this struct.
type proxyFlags struct {
	// Connection
	OtlpUrl   string
	AuthToken string
	Dataset   string

	// Listener
	HTTPPort int
	GRPCPort int

	// Visibility
	Tail          bool
	TailStderr    bool
	StatsInterval time.Duration

	// Outbound
	MaxConcurrent int
}

// newProxyCmd creates the experimental `dash0 otlp proxy` command.
func newProxyCmd() *cobra.Command {
	flags := &proxyFlags{}

	cmd := &cobra.Command{
		Use:   "proxy",
		Short: "[experimental] Run a local OTLP forwarder to Dash0",
		Long: `Listen on the standard OTLP/HTTP and OTLP/gRPC endpoints and forward inbound
telemetry to Dash0 using the active profile's credentials.

The proxy binds 127.0.0.1:4318 (HTTP) and 127.0.0.1:4317 (gRPC) by default
so an OpenTelemetry SDK at default endpoint configuration connects with no
env-var change. If a default port is in use, the proxy falls back to an
OS-assigned port and prints the actual endpoint on stderr.

The proxy is a local-dev shortcut, not a replacement for the OpenTelemetry
Collector. It does not buffer outbound on Dash0 outages; backpressure
surfaces to SDKs as HTTP 503 / gRPC UNAVAILABLE.`,
		Example: `  # Just run it. SDK defaults already point at 127.0.0.1:4318 / 4317.
  dash0 -X otlp proxy

  # Override the HTTP port (the gRPC default is preserved).
  dash0 -X otlp proxy --http-port 8318

  # Watch each forwarded record in the terminal.
  dash0 -X otlp proxy --tail

  # Run under agent mode for structured event consumption.
  dash0 --agent-mode -X otlp proxy`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := experimental.RequireExperimental(cmd); err != nil {
				return err
			}
			if err := resolveEnvOverrides(cmd, flags); err != nil {
				return err
			}
			if err := validateFlags(flags); err != nil {
				return err
			}
			return runProxy(cmd, flags)
		},
	}

	cmd.Flags().StringVar(&flags.OtlpUrl, "otlp-url", "", "OTLP endpoint URL (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token (overrides active profile)")
	cmd.Flags().StringVar(&flags.Dataset, "dataset", "", "Dataset to route forwarded telemetry to (overrides active profile)")
	cmd.Flags().IntVar(&flags.HTTPPort, "http-port", defaultHTTPPort,
		"OTLP/HTTP listener port; set 0 for OS-assigned; env: "+envHTTPPort)
	cmd.Flags().IntVar(&flags.GRPCPort, "grpc-port", defaultGRPCPort,
		"OTLP/gRPC listener port; set 0 for OS-assigned; env: "+envGRPCPort)
	cmd.Flags().BoolVar(&flags.Tail, "tail", false,
		"Print each forwarded record in collector-debug-exporter style (to stdout in TTY mode; requires --tail-stderr in agent mode)")
	cmd.Flags().BoolVar(&flags.TailStderr, "tail-stderr", false,
		"Route --tail output to stderr instead of stdout (required when combining --tail with agent mode)")
	cmd.Flags().DurationVar(&flags.StatsInterval, "stats-interval", defaultStatsInterval,
		"Interval between live-stats updates")
	cmd.Flags().IntVar(&flags.MaxConcurrent, "max-concurrent", defaultMaxConcurrent,
		"Maximum number of concurrent outbound requests to Dash0 (1-10); env: "+envMaxConcurrent)

	return cmd
}

// resolveEnvOverrides applies environment-variable overrides for flags that
// were not explicitly set on the command line.
//
// Precedence per port: --flag > DASH0_OTLP_PROXY_* > OTEL_EXPORTER_OTLP_* > default.
// Standard OpenTelemetry exporter env vars (OTEL_EXPORTER_OTLP_ENDPOINT plus
// OTEL_EXPORTER_OTLP_PROTOCOL) feed in as the lowest-priority override so a
// shell already configured for an SDK against a non-default port carries
// through to the proxy listener without explicit flags.
func resolveEnvOverrides(cmd *cobra.Command, flags *proxyFlags) error {
	httpFromOTEL, grpcFromOTEL, err := parseOTELExporterEnv()
	if err != nil {
		return err
	}

	if !cmd.Flags().Changed("http-port") {
		if v, ok := nonEmptyEnv(envHTTPPort); ok {
			port, err := parsePort(envHTTPPort, v)
			if err != nil {
				return err
			}
			flags.HTTPPort = port
		} else if httpFromOTEL != nil {
			flags.HTTPPort = *httpFromOTEL
		}
	}
	if !cmd.Flags().Changed("grpc-port") {
		if v, ok := nonEmptyEnv(envGRPCPort); ok {
			port, err := parsePort(envGRPCPort, v)
			if err != nil {
				return err
			}
			flags.GRPCPort = port
		} else if grpcFromOTEL != nil {
			flags.GRPCPort = *grpcFromOTEL
		}
	}
	if !cmd.Flags().Changed("max-concurrent") {
		if v, ok := nonEmptyEnv(envMaxConcurrent); ok {
			n, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("%s: invalid integer %q", envMaxConcurrent, v)
			}
			flags.MaxConcurrent = n
		}
	}
	return nil
}

// nonEmptyEnv returns the env var value and true only when the var is set to
// a non-empty string. A set-but-empty env var (`DASH0_OTLP_PROXY_HTTP_PORT=`)
// is treated as "not set" so the next precedence tier applies.
func nonEmptyEnv(name string) (string, bool) {
	if v, ok := os.LookupEnv(name); ok && v != "" {
		return v, true
	}
	return "", false
}

// parseOTELExporterEnv extracts port hints from OTEL_EXPORTER_OTLP_ENDPOINT,
// disambiguated by OTEL_EXPORTER_OTLP_PROTOCOL. Returns (httpPort, grpcPort)
// pointers — non-nil when the env var contributed a value for that side.
//
// The OTel spec defines OTEL_EXPORTER_OTLP_PROTOCOL with values "grpc",
// "http/protobuf", and "http/json". The default is implementation-specific
// (most SDKs default to grpc); when unset, this function leaves both sides
// nil and relies on the next precedence tier.
func parseOTELExporterEnv() (*int, *int, error) {
	endpoint, ok := nonEmptyEnv(envOTELEndpoint)
	if !ok {
		return nil, nil, nil
	}
	port, err := portFromOTELEndpoint(endpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", envOTELEndpoint, err)
	}
	protocol, _ := nonEmptyEnv(envOTELProtocol)
	switch {
	case strings.HasPrefix(strings.ToLower(protocol), "http"):
		return &port, nil, nil
	case strings.EqualFold(protocol, "grpc"):
		return nil, &port, nil
	default:
		// Unset or unrecognised protocol: do not infer which side the user
		// meant. Leave both nil so the user has to set DASH0_OTLP_PROXY_*
		// explicitly when their shell has a bare OTEL_EXPORTER_OTLP_ENDPOINT
		// without a protocol.
		return nil, nil, nil
	}
}

// portFromOTELEndpoint extracts the port from an OTEL endpoint string.
//
// Endpoint shapes seen in the wild across SDK implementations:
//
//   - "http://localhost:4318" — standard URL, accepted by every SDK
//   - "https://example.com:4318" — TLS form
//   - "http://[::1]:4318" — IPv6 in brackets, URL form
//   - "localhost:4317" — bare host:port; Python tolerates, Java rejects
//   - "[::1]:4317" — bare IPv6 host:port
//   - "dns:///my-collector:4317" — gRPC name-resolver scheme (the empty
//     authority between `:` and `/` is required by gRPC's dns:// grammar)
//   - "grpc://host:4317" — informal gRPC URL form
//   - "unix:///run/otelcol.sock" — Unix domain socket; no port concept,
//     dropped explicitly
//
// We do not try to infer a default port from a scheme-without-port shape like
// "http://localhost" — that requires SDK-implementation knowledge (gRPC default
// is 4317, HTTP default is 4318) and the user is better served by setting the
// port explicitly. We return an error in that case so the OTEL signal is
// dropped and the next precedence tier kicks in.
//
// Sources: OpenTelemetry exporter spec, opentelemetry-python urlparse-based
// parsing, opentelemetry-java URL validator, opentelemetry-go envconfig.go.
func portFromOTELEndpoint(endpoint string) (int, error) {
	if u, err := url.Parse(endpoint); err == nil && u.Scheme != "" {
		switch strings.ToLower(u.Scheme) {
		case "unix":
			return 0, errors.New("unix-socket endpoint has no port")
		case "dns":
			// gRPC `dns:///host:port` or `dns://authority/host:port`. The
			// authority can be empty (the `///` case); url.Parse puts the
			// target path in u.Opaque when there is no authority, or in
			// u.Path when there is. Recurse on whichever holds the target.
			target := strings.TrimPrefix(strings.TrimPrefix(u.Opaque+u.Path, "//"), "/")
			if target == "" {
				return 0, errors.New("dns:// endpoint has no target")
			}
			return parseBarePort(target)
		}
		if u.Host != "" {
			if p := u.Port(); p != "" {
				return parsePort(envOTELEndpoint, p)
			}
			return 0, errors.New("endpoint URL has no explicit port")
		}
	}
	return parseBarePort(endpoint)
}

// parseBarePort extracts the port from a bare `host:port` or `[ipv6]:port`
// shape using Go's canonical net.SplitHostPort, which rejects ambiguous
// unbracketed IPv6 input.
func parseBarePort(target string) (int, error) {
	_, port, err := net.SplitHostPort(target)
	if err != nil {
		return 0, fmt.Errorf("cannot extract port from %q: %w", target, err)
	}
	return parsePort(envOTELEndpoint, port)
}

// parsePort returns the validated port number for source (which appears in
// error messages so the user can identify whether the value came from a flag
// or env var).
func parsePort(source, raw string) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q", source, raw)
	}
	if n < 0 || n > 65535 {
		return 0, fmt.Errorf("%s: port %d is out of range (0-65535)", source, n)
	}
	return n, nil
}

// validateFlags performs cross-flag validation after env-var overrides have
// been applied.
//
// Listed validations:
//   - HTTP and gRPC listeners cannot share a TCP port (KTD6).
//   - MaxConcurrent must be within the dash0api.Client's supported range.
//   - HTTPPort / GRPCPort must be within the valid TCP port range.
func validateFlags(flags *proxyFlags) error {
	if flags.HTTPPort < 0 || flags.HTTPPort > 65535 {
		return fmt.Errorf("--http-port %d is out of range (0-65535)", flags.HTTPPort)
	}
	if flags.GRPCPort < 0 || flags.GRPCPort > 65535 {
		return fmt.Errorf("--grpc-port %d is out of range (0-65535)", flags.GRPCPort)
	}
	// Same-port collision: never allow two listeners on the same explicit port.
	// Port 0 is OS-assigned, so both listeners using 0 is fine — they'll get
	// distinct OS-assigned ports.
	if flags.HTTPPort != 0 && flags.HTTPPort == flags.GRPCPort {
		return errors.New("HTTP and gRPC listeners cannot share a port (--http-port and --grpc-port must differ)")
	}
	if flags.MaxConcurrent < 1 || flags.MaxConcurrent > maxAllowedConcurrent {
		return fmt.Errorf("--max-concurrent %d is out of range (1-%d)", flags.MaxConcurrent, maxAllowedConcurrent)
	}
	if flags.Tail && flags.TailStderr {
		// Both flags set is fine — TailStderr only kicks in if agent mode is also
		// on; in TTY mode it has no effect. No validation needed here.
		_ = flags
	}
	return nil
}

// runProxy is the entrypoint for the proxy command. The lifecycle supervisor
// (U5) takes over from here once it lands.
func runProxy(cmd *cobra.Command, flags *proxyFlags) error {
	return errors.New("dash0 otlp proxy: implementation pending (U5+ lifecycle, U12 pipeline, U13 consumer, U4 workers)")
}
