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

func TestPreBindPort_AvailablePortSucceeds(t *testing.T) {
	port := pickFreePort(t)
	resolved, err := preBindPort("http", port)
	if err != nil {
		t.Fatalf("preBindPort: %v", err)
	}
	if resolved != port {
		t.Errorf("resolved port = %d; want %d (port was available)", resolved, port)
	}
}

func TestPreBindPort_PortInUseFailsWithActionableError(t *testing.T) {
	// Default-port collisions now fail loudly with a process-identifying
	// error message; the silent OS-assigned fallback was removed because
	// it was too easy to miss when the SDK was still pointed at the
	// original default.
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = occupied.Close() }()
	port := occupied.Addr().(*net.TCPAddr).Port

	_, err = preBindPort("http", port)
	if err == nil {
		t.Fatal("expected error on port collision; got nil")
	}
	msg := err.Error()

	// The error must name the port and the override flag so the developer
	// can act on it without a separate investigation.
	if !strings.Contains(msg, "is already in use") {
		t.Errorf("error should say the port is in use; got: %v", err)
	}
	if !strings.Contains(msg, "--http-port") {
		t.Errorf("error should mention the override flag; got: %v", err)
	}
}

func TestPreBindPort_GRPCErrorReferencesGRPCFlag(t *testing.T) {
	// The label drives which flag is named in the error message. The
	// "grpc" label must produce a --grpc-port hint.
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = occupied.Close() }()
	port := occupied.Addr().(*net.TCPAddr).Port

	_, err = preBindPort("grpc", port)
	if err == nil {
		t.Fatal("expected error; got nil")
	}
	if !strings.Contains(err.Error(), "--grpc-port") {
		t.Errorf("gRPC error should mention --grpc-port; got: %v", err)
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
