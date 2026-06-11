package otlp

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestProxyFlags_Defaults(t *testing.T) {
	t.Setenv(envHTTPPort, "")
	t.Setenv(envGRPCPort, "")
	t.Setenv(envMaxConcurrent, "")

	cmd := newProxyCmd()
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}
	flags := readFlagsFromCmd(t, cmd)

	if flags.HTTPPort != defaultHTTPPort {
		t.Errorf("default HTTPPort = %d; want %d", flags.HTTPPort, defaultHTTPPort)
	}
	if flags.GRPCPort != defaultGRPCPort {
		t.Errorf("default GRPCPort = %d; want %d", flags.GRPCPort, defaultGRPCPort)
	}
	if flags.StatsInterval != defaultStatsInterval {
		t.Errorf("default StatsInterval = %s; want %s", flags.StatsInterval, defaultStatsInterval)
	}
	if flags.MaxConcurrent != defaultMaxConcurrent {
		t.Errorf("default MaxConcurrent = %d; want %d", flags.MaxConcurrent, defaultMaxConcurrent)
	}
	if flags.Tail {
		t.Errorf("default Tail = true; want false")
	}
	if flags.TailStderr {
		t.Errorf("default TailStderr = true; want false")
	}
}

func TestProxyFlags_FlagOverridesDefault(t *testing.T) {
	t.Setenv(envHTTPPort, "")

	cmd := newProxyCmd()
	if err := cmd.ParseFlags([]string{"--http-port", "9999"}); err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}
	flags := readFlagsFromCmd(t, cmd)

	if flags.HTTPPort != 9999 {
		t.Errorf("HTTPPort = %d; want 9999", flags.HTTPPort)
	}
}

func TestProxyFlags_EnvOverridesDefault(t *testing.T) {
	t.Setenv(envHTTPPort, "8888")
	t.Setenv(envGRPCPort, "8889")
	t.Setenv(envMaxConcurrent, "5")

	cmd := newProxyCmd()
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}
	flags := readFlagsFromCmd(t, cmd)
	if err := resolveEnvOverrides(cmd, flags); err != nil {
		t.Fatalf("resolveEnvOverrides: %v", err)
	}

	if flags.HTTPPort != 8888 {
		t.Errorf("HTTPPort = %d; want 8888 from %s", flags.HTTPPort, envHTTPPort)
	}
	if flags.GRPCPort != 8889 {
		t.Errorf("GRPCPort = %d; want 8889 from %s", flags.GRPCPort, envGRPCPort)
	}
	if flags.MaxConcurrent != 5 {
		t.Errorf("MaxConcurrent = %d; want 5 from %s", flags.MaxConcurrent, envMaxConcurrent)
	}
}

func TestProxyFlags_FlagWinsOverEnv(t *testing.T) {
	t.Setenv(envHTTPPort, "8888")

	cmd := newProxyCmd()
	if err := cmd.ParseFlags([]string{"--http-port", "9999"}); err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}
	flags := readFlagsFromCmd(t, cmd)
	if err := resolveEnvOverrides(cmd, flags); err != nil {
		t.Fatalf("resolveEnvOverrides: %v", err)
	}

	if flags.HTTPPort != 9999 {
		t.Errorf("HTTPPort = %d; want 9999 (explicit flag) over 8888 (env)", flags.HTTPPort)
	}
}

func TestProxyFlags_OTELEnvHTTPFallback(t *testing.T) {
	t.Setenv(envHTTPPort, "")
	t.Setenv(envGRPCPort, "")
	t.Setenv(envOTELEndpoint, "http://localhost:5318")
	t.Setenv(envOTELProtocol, "http/protobuf")

	cmd := newProxyCmd()
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}
	flags := readFlagsFromCmd(t, cmd)
	if err := resolveEnvOverrides(cmd, flags); err != nil {
		t.Fatalf("resolveEnvOverrides: %v", err)
	}

	if flags.HTTPPort != 5318 {
		t.Errorf("HTTPPort = %d; want 5318 from OTEL_EXPORTER_OTLP_ENDPOINT", flags.HTTPPort)
	}
	// gRPC untouched because protocol is http.
	if flags.GRPCPort != defaultGRPCPort {
		t.Errorf("GRPCPort = %d; want default %d (OTEL HTTP shouldn't touch gRPC)", flags.GRPCPort, defaultGRPCPort)
	}
}

func TestProxyFlags_OTELEnvGRPCFallback(t *testing.T) {
	t.Setenv(envHTTPPort, "")
	t.Setenv(envGRPCPort, "")
	t.Setenv(envOTELEndpoint, "http://localhost:5317")
	t.Setenv(envOTELProtocol, "grpc")

	cmd := newProxyCmd()
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}
	flags := readFlagsFromCmd(t, cmd)
	if err := resolveEnvOverrides(cmd, flags); err != nil {
		t.Fatalf("resolveEnvOverrides: %v", err)
	}

	if flags.GRPCPort != 5317 {
		t.Errorf("GRPCPort = %d; want 5317 from OTEL_EXPORTER_OTLP_ENDPOINT", flags.GRPCPort)
	}
	if flags.HTTPPort != defaultHTTPPort {
		t.Errorf("HTTPPort = %d; want default %d", flags.HTTPPort, defaultHTTPPort)
	}
}

func TestProxyFlags_DASH0EnvWinsOverOTELEnv(t *testing.T) {
	// DASH0_OTLP_PROXY_HTTP_PORT must outrank OTEL_EXPORTER_OTLP_ENDPOINT.
	t.Setenv(envHTTPPort, "9999")
	t.Setenv(envOTELEndpoint, "http://localhost:5318")
	t.Setenv(envOTELProtocol, "http/protobuf")

	cmd := newProxyCmd()
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}
	flags := readFlagsFromCmd(t, cmd)
	if err := resolveEnvOverrides(cmd, flags); err != nil {
		t.Fatalf("resolveEnvOverrides: %v", err)
	}

	if flags.HTTPPort != 9999 {
		t.Errorf("HTTPPort = %d; want 9999 (DASH0_OTLP_PROXY_HTTP_PORT outranks OTEL_EXPORTER_OTLP_ENDPOINT)", flags.HTTPPort)
	}
}

func TestPortFromOTELEndpoint(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		want     int
		wantErr  bool
	}{
		{"http URL with port", "http://localhost:4318", 4318, false},
		{"https URL with port", "https://example.com:4319", 4319, false},
		{"http URL with IPv6 port", "http://[::1]:4318", 4318, false},
		{"https URL with IPv6 port", "https://[2001:db8::1]:4319", 4319, false},
		{"http URL with trailing slash", "http://localhost:4318/", 4318, false},
		{"http URL with path", "http://localhost:4318/v1/traces", 4318, false},
		{"bare host:port", "localhost:4317", 4317, false},
		{"bare IPv6 host:port", "[::1]:4317", 4317, false},
		{"bare IPv6 unbracketed (ambiguous)", "::1:4317", 0, true},
		{"grpc:// scheme", "grpc://host:4317", 4317, false},
		{"dns:/// scheme", "dns:///my-collector:4317", 4317, false},
		{"dns:// with authority", "dns://authority/my-collector:4317", 4317, false},
		{"unix:// scheme rejected", "unix:///run/otelcol.sock", 0, true},
		{"scheme with no port (rejected)", "http://localhost", 0, true},
		{"empty string", "", 0, true},
		{"no port at all", "localhost", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := portFromOTELEndpoint(tc.endpoint)
			if tc.wantErr {
				if err == nil {
					t.Errorf("portFromOTELEndpoint(%q) = %d; want error", tc.endpoint, got)
				}
				return
			}
			if err != nil {
				t.Errorf("portFromOTELEndpoint(%q) unexpected error: %v", tc.endpoint, err)
				return
			}
			if got != tc.want {
				t.Errorf("portFromOTELEndpoint(%q) = %d; want %d", tc.endpoint, got, tc.want)
			}
		})
	}
}

func TestProxyFlags_OTELEnvProtocolUnsetSkipped(t *testing.T) {
	// Without OTEL_EXPORTER_OTLP_PROTOCOL, the endpoint can't be routed to HTTP vs gRPC.
	t.Setenv(envHTTPPort, "")
	t.Setenv(envGRPCPort, "")
	t.Setenv(envOTELEndpoint, "http://localhost:5318")
	t.Setenv(envOTELProtocol, "")

	cmd := newProxyCmd()
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}
	flags := readFlagsFromCmd(t, cmd)
	if err := resolveEnvOverrides(cmd, flags); err != nil {
		t.Fatalf("resolveEnvOverrides: %v", err)
	}

	if flags.HTTPPort != defaultHTTPPort || flags.GRPCPort != defaultGRPCPort {
		t.Errorf("OTEL endpoint without PROTOCOL should leave defaults intact; got HTTP=%d GRPC=%d",
			flags.HTTPPort, flags.GRPCPort)
	}
}

func TestProxyFlags_EnvInvalidIntegerErrors(t *testing.T) {
	t.Setenv(envHTTPPort, "not-a-port")

	cmd := newProxyCmd()
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}
	flags := readFlagsFromCmd(t, cmd)

	err := resolveEnvOverrides(cmd, flags)
	if err == nil {
		t.Fatal("resolveEnvOverrides should have errored on non-integer env value")
	}
	if !strings.Contains(err.Error(), envHTTPPort) {
		t.Errorf("error should name the env var; got %q", err.Error())
	}
}

func TestValidateFlags_SamePortRejected(t *testing.T) {
	// Covers AE9.
	flags := &proxyFlags{
		HTTPPort:      4318,
		GRPCPort:      4318,
		MaxConcurrent: defaultMaxConcurrent,
	}
	err := validateFlags(flags)
	if err == nil {
		t.Fatal("validateFlags should reject same explicit port for HTTP and gRPC")
	}
	if !strings.Contains(err.Error(), "cannot share a port") {
		t.Errorf("error message should explain the same-port conflict; got %q", err.Error())
	}
}

func TestValidateFlags_SamePortZeroAllowed(t *testing.T) {
	// Port 0 means OS-assigned for both; the kernel hands out distinct ports.
	flags := &proxyFlags{
		HTTPPort:      0,
		GRPCPort:      0,
		MaxConcurrent: defaultMaxConcurrent,
	}
	if err := validateFlags(flags); err != nil {
		t.Errorf("validateFlags should allow both ports being 0 (OS-assigned); got %v", err)
	}
}

func TestValidateFlags_MaxConcurrentRange(t *testing.T) {
	cases := []struct {
		val     int
		wantErr bool
	}{
		{0, true},
		{1, false},
		{10, false},
		{11, true},
		{100, true},
	}
	for _, tc := range cases {
		flags := &proxyFlags{
			HTTPPort:      defaultHTTPPort,
			GRPCPort:      defaultGRPCPort,
			MaxConcurrent: tc.val,
		}
		err := validateFlags(flags)
		if tc.wantErr && err == nil {
			t.Errorf("validateFlags(MaxConcurrent=%d) should error", tc.val)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("validateFlags(MaxConcurrent=%d) unexpected error: %v", tc.val, err)
		}
	}
}

func TestValidateFlags_PortRange(t *testing.T) {
	cases := []struct {
		name       string
		http, grpc int
		wantErr    bool
	}{
		{"defaults", 4318, 4317, false},
		{"http negative", -1, 4317, true},
		{"http over max", 65536, 4317, true},
		{"grpc negative", 4318, -1, true},
		{"grpc over max", 4318, 65536, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flags := &proxyFlags{
				HTTPPort:      tc.http,
				GRPCPort:      tc.grpc,
				MaxConcurrent: defaultMaxConcurrent,
			}
			err := validateFlags(flags)
			if tc.wantErr && err == nil {
				t.Errorf("expected error; got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRequiresExperimentalFlag(t *testing.T) {
	root := &cobra.Command{Use: "dash0"}
	root.PersistentFlags().BoolP("experimental", "X", false, "")
	root.AddCommand(NewOtlpCmd())

	root.SetArgs([]string{"otlp", "proxy"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when -X is not set")
	}
	if !strings.Contains(err.Error(), "experimental") {
		t.Errorf("error should mention experimental flag; got %q", err.Error())
	}
}

// readFlagsFromCmd reconstructs a proxyFlags struct from the parsed cobra
// flags. It mirrors the variable bindings inside newProxyCmd so tests can
// introspect the parsed state without depending on the closure capture.
func readFlagsFromCmd(t *testing.T, cmd *cobra.Command) *proxyFlags {
	t.Helper()
	f := cmd.Flags()

	mustString := func(name string) string {
		v, err := f.GetString(name)
		if err != nil {
			t.Fatalf("GetString(%q): %v", name, err)
		}
		return v
	}
	mustInt := func(name string) int {
		v, err := f.GetInt(name)
		if err != nil {
			t.Fatalf("GetInt(%q): %v", name, err)
		}
		return v
	}
	mustBool := func(name string) bool {
		v, err := f.GetBool(name)
		if err != nil {
			t.Fatalf("GetBool(%q): %v", name, err)
		}
		return v
	}

	d, err := f.GetDuration("stats-interval")
	if err != nil {
		t.Fatalf("GetDuration(stats-interval): %v", err)
	}

	return &proxyFlags{
		OtlpUrl:       mustString("otlp-url"),
		AuthToken:     mustString("auth-token"),
		Dataset:       mustString("dataset"),
		HTTPPort:      mustInt("http-port"),
		GRPCPort:      mustInt("grpc-port"),
		Tail:          mustBool("tail"),
		TailStderr:    mustBool("tail-stderr"),
		StatsInterval: d,
		MaxConcurrent: mustInt("max-concurrent"),
	}
}
