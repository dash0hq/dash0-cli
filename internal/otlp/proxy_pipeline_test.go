package otlp

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
)

// pickFreePort asks the kernel for an available 127.0.0.1 TCP port,
// closes the listener, and returns the port number. Used by tests that
// want to claim a real port before exercising the pre-bind logic.
func pickFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen 127.0.0.1:0: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("close pick-listener: %v", err)
	}
	return port
}

func TestPreBindPort_DefaultPortAvailable(t *testing.T) {
	// Use a freshly-picked free port as the "default", so the bind succeeds
	// on the first try.
	port := pickFreePort(t)
	resolved, err := preBindPort("test", port, port)
	if err != nil {
		t.Fatalf("preBindPort: %v", err)
	}
	if resolved != port {
		t.Errorf("resolved port = %d; want %d (default was available)", resolved, port)
	}
}

func TestPreBindPort_DefaultPortInUseFallsBackToZero(t *testing.T) {
	// Hold a port to simulate collision.
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = occupied.Close() }()
	port := occupied.Addr().(*net.TCPAddr).Port

	// Same port as both "requested" and "default" → fallback path fires.
	resolved, err := preBindPort("test", port, port)
	if err != nil {
		t.Fatalf("preBindPort fallback: %v", err)
	}
	if resolved == port {
		t.Errorf("fallback should yield a different port; got the same %d", resolved)
	}
	if resolved == 0 {
		t.Errorf("fallback port should be non-zero (OS-assigned); got 0")
	}
}

func TestPreBindPort_ExplicitOverrideInUseErrors(t *testing.T) {
	// When the requested port is explicitly set (i.e., requested !=
	// defaultPort) and is taken, the function must error rather than
	// silently fall back. AE6 covers this.
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = occupied.Close() }()
	port := occupied.Addr().(*net.TCPAddr).Port

	// requested != defaultPort triggers strict mode (no fallback).
	_, err = preBindPort("test", port, port+1)
	if err == nil {
		t.Fatal("expected error on explicit-override collision; got nil")
	}
	if !strings.Contains(err.Error(), "bind test port") {
		t.Errorf("error message should mention the bind failure; got: %v", err)
	}
}

func TestBuildReceiverConfig_PatchesEndpoints(t *testing.T) {
	cfg := buildReceiverConfig(18181, 18182)
	httpServer := cfg.Protocols.HTTP.Get()
	if httpServer == nil {
		t.Fatal("HTTP protocol unset")
	}
	wantHTTP := "127.0.0.1:18181"
	if got := httpServer.ServerConfig.NetAddr.Endpoint; got != wantHTTP {
		t.Errorf("HTTP endpoint = %q; want %q", got, wantHTTP)
	}

	grpcServer := cfg.Protocols.GRPC.Get()
	if grpcServer == nil {
		t.Fatal("gRPC protocol unset")
	}
	wantGRPC := "127.0.0.1:18182"
	if got := grpcServer.NetAddr.Endpoint; got != wantGRPC {
		t.Errorf("gRPC endpoint = %q; want %q", got, wantGRPC)
	}
	if grpcServer.MaxRecvMsgSizeMiB != proxyGRPCMaxRecvMiB {
		t.Errorf("MaxRecvMsgSizeMiB = %d; want %d",
			grpcServer.MaxRecvMsgSizeMiB, proxyGRPCMaxRecvMiB)
	}
}

func TestBuildPipeline_AssemblesAllSignalReceivers(t *testing.T) {
	stats := &Stats{}
	consumer := NewProxyConsumer(stats, NewEmitter("inst", nil), nil)

	pipeline, err := BuildPipeline(context.Background(), pickFreePort(t), pickFreePort(t), consumer)
	if err != nil {
		t.Fatalf("BuildPipeline: %v", err)
	}
	if pipeline.logsRecv == nil || pipeline.tracesRecv == nil || pipeline.metricsRecv == nil {
		t.Errorf("not all signal receivers were created: logs=%v traces=%v metrics=%v",
			pipeline.logsRecv != nil, pipeline.tracesRecv != nil, pipeline.metricsRecv != nil)
	}
	ep := pipeline.Endpoints()
	if !strings.HasPrefix(ep.HTTPEndpoint, "127.0.0.1:") {
		t.Errorf("HTTP endpoint = %q; want 127.0.0.1:<port>", ep.HTTPEndpoint)
	}
	if !strings.HasPrefix(ep.GRPCEndpoint, "127.0.0.1:") {
		t.Errorf("gRPC endpoint = %q; want 127.0.0.1:<port>", ep.GRPCEndpoint)
	}
}

func TestPipeline_StartShutdown(t *testing.T) {
	stats := &Stats{}
	consumer := NewProxyConsumer(stats, NewEmitter("inst", nil), nil)

	pipeline, err := BuildPipeline(context.Background(), pickFreePort(t), pickFreePort(t), consumer)
	if err != nil {
		t.Fatalf("BuildPipeline: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pipeline.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := pipeline.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestPipeline_HTTPLogsRoundTripsToConsumer(t *testing.T) {
	// End-to-end smoke: post an OTLP/HTTP logs request and assert our
	// consumer enqueued it. Covers R7 (paths inherited), R9 (content
	// types inherited via the receiver), and the U13/U12 wiring.
	stats := &Stats{}
	eventCh := make(chan plog.Logs, 4)
	consumer := NewProxyConsumer(stats, NewEmitter("inst", eventCh), nil)

	httpPort := pickFreePort(t)
	pipeline, err := BuildPipeline(context.Background(), httpPort, pickFreePort(t), consumer)
	if err != nil {
		t.Fatalf("BuildPipeline: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := pipeline.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = pipeline.Shutdown(ctx) }()

	// Construct a one-record OTLP/JSON request and post it.
	ld := newLogsBatch(1)
	body, err := plogotlp.NewExportRequestFromLogs(ld).MarshalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	url := "http://127.0.0.1:" + strconv.Itoa(httpPort) + "/v1/logs"

	// Wait a beat for the listener to be ready, then try a few times.
	var resp *http.Response
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		req, mkErr := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
		if mkErr != nil {
			t.Fatalf("NewRequest: %v", mkErr)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err = http.DefaultClient.Do(req)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d; want 200", resp.StatusCode)
	}

	// Forwarded event should have fired exactly once.
	select {
	case <-eventCh:
	case <-time.After(500 * time.Millisecond):
		t.Error("no forwarded event fired within 500ms after POST")
	}

	if got := stats.Forwarded(SignalLogs); got != 1 {
		t.Errorf("Forwarded(logs) after POST = %d; want 1", got)
	}

	// Drain the logs channel so Shutdown doesn't block.
	select {
	case <-consumer.LogsChannel():
	default:
	}
}
