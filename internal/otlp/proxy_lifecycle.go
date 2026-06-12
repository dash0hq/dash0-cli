package otlp

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/pdata/plog"

	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/dash0hq/dash0-cli/internal/config"
)

// proxyShutdownDeadline bounds graceful shutdown after a signal. Receivers
// finish in-flight RPCs and worker queues drain within this window;
// remaining batches are dropped on the floor. Five seconds matches the
// Collector default and is long enough for an SDK to finish a typical
// flush.
const proxyShutdownDeadline = 5 * time.Second

// statsTickInterval is the cadence at which the rate sampler snapshots
// counters and pushes them to the stdout/stderr writers. Hardcoded
// because users have no reason to tune it; the TTY redraw assumes ~1Hz.
const statsTickInterval = 1 * time.Second

// runProxy is the entry point wired to the `dash0 otlp proxy` cobra
// command. It blocks until SIGINT or SIGTERM arrives, at which point it
// best-effort-drains in-flight work within proxyShutdownDeadline and
// returns. A non-nil return value bubbles up to main(), which formats and
// exits non-zero (KTD5).
func runProxy(cmd *cobra.Command, flags *proxyFlags) error {
	cmd.SilenceUsage = true
	ctx := cmd.Context()

	// Apply env-var overrides on top of the parsed flags before any work.
	if err := resolveEnvOverrides(cmd, flags); err != nil {
		return err
	}
	if err := validateFlags(flags); err != nil {
		return err
	}

	cfg, profileName, err := resolveProxyConfig(ctx, flags)
	if err != nil {
		return err
	}

	instanceID := uuid.NewString()
	stats := &Stats{}

	// Wire stdout writer + emitter for agent-mode event stream. In
	// non-agent mode the emitter holds a nil channel and every Emit call
	// is a no-op — no goroutine started, no buffer allocated.
	var stdoutWriter *StdoutWriter
	var stdoutReader chan plog.Logs
	var stdoutCh chan<- plog.Logs
	if agentmode.Enabled {
		w, ch := NewStdoutWriter(os.Stdout)
		stdoutWriter = w
		stdoutReader = ch
		stdoutCh = ch
	}
	emitter := NewEmitter(instanceID, stdoutCh)

	// Wire stderr writer for the TTY stats line + lifecycle messages.
	// stderrWriter handles non-TTY suppression internally.
	stderrWriter, statsCh, lifecycleCh := NewStderrWriter(os.Stderr, int(os.Stderr.Fd()))

	// Per-signal channel between worker pool and stderr (for the throttled
	// auth-error line).
	lifecycleChOut := lifecycleCh

	// Build forwarder (long-lived dash0api.Client with KTD3b max-concurrent).
	apiClient, err := client.NewOtlpClientFromContext(ctx, flags.OtlpUrl, flags.AuthToken)
	if err != nil {
		return fmt.Errorf("construct OTLP client: %w", err)
	}
	defer func() { _ = apiClient.Close(ctx) }()

	dataset := client.ResolveDataset(ctx, flags.Dataset)

	// Consumer: tail rendering goes to stdout in --tail mode (and is
	// disabled in agent mode — agent already sees the rendering through
	// the structured forwarded event stream).
	var tailCh chan<- string = nil
	var tailReader chan string
	if flags.Tail && !agentmode.Enabled {
		tailReader = make(chan string, 16)
		tailCh = tailReader
	}
	consumer := NewProxyConsumer(stats, emitter, tailCh)

	// Build the outbound decorator from the user-facing flags. Empty
	// flags produce an empty decorator whose Decorate* calls short-
	// circuit, so the zero-flag case has no per-batch cost.
	resourceAttrs, err := ParseKeyValuePairs(flags.ResourceAttributes)
	if err != nil {
		return fmt.Errorf("--resource-attribute: %w", err)
	}
	scopeAttrs, err := ParseKeyValuePairs(flags.ScopeAttributes)
	if err != nil {
		return fmt.Errorf("--scope-attribute: %w", err)
	}
	logAttrs, err := ParseKeyValuePairs(flags.LogAttributes)
	if err != nil {
		return fmt.Errorf("--log-attribute: %w", err)
	}
	spanAttrs, err := ParseKeyValuePairs(flags.SpanAttributes)
	if err != nil {
		return fmt.Errorf("--span-attribute: %w", err)
	}
	metricAttrs, err := ParseKeyValuePairs(flags.MetricAttributes)
	if err != nil {
		return fmt.Errorf("--metric-attribute: %w", err)
	}
	decorator := NewDecorator(
		resourceAttrs, scopeAttrs,
		flags.ScopeName, flags.ScopeVersion,
		logAttrs, spanAttrs, metricAttrs,
	)

	workers := NewWorkerPool(apiClient, dataset, stats, emitter, consumer, lifecycleChOut, decorator)

	pipeline, err := BuildPipeline(ctx, flags.HTTPPort, flags.GRPCPort, consumer)
	if err != nil {
		return fmt.Errorf("build OTLP pipeline: %w", err)
	}

	// Launch the supporting goroutines before starting the receiver so
	// any banner / started event lands on the writers immediately.
	supCtx, supCancel := context.WithCancel(ctx)
	defer supCancel()

	var wg sync.WaitGroup
	if stdoutWriter != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = stdoutWriter.Run(supCtx, stdoutReader)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		stderrWriter.Run(supCtx, statsCh, lifecycleCh)
	}()

	// Workers drain channels.
	wg.Add(1)
	go func() {
		defer wg.Done()
		workers.Run(supCtx)
	}()

	// Rate sampler ticks every second and fans snapshots to both writers.
	sampler := NewRateSampler(stats, statsTickInterval, sparklineHistoryCapacity)
	wg.Add(1)
	go func() {
		defer wg.Done()
		sampler.Run(supCtx, statsCh, statsSink(emitter))
	}()

	// --tail stdout writer (if active) — single goroutine consuming the
	// tail channel and writing to os.Stdout. Reuses the WriteLn loop here
	// rather than a separate package since the renderings are already
	// fully formatted strings.
	if tailReader != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-supCtx.Done():
					return
				case s, ok := <-tailReader:
					if !ok {
						return
					}
					fmt.Fprintln(os.Stdout, s)
				}
			}
		}()
	}

	// Start the receiver. After this returns successfully, traffic is
	// being accepted.
	if err := pipeline.Start(supCtx); err != nil {
		supCancel()
		_ = pipeline.Shutdown(context.Background())
		wg.Wait()
		return fmt.Errorf("start OTLP pipeline: %w", err)
	}

	// Banner — humans see it on stderr; agents see the structured
	// `started` event on stdout. Order matters: announce before signaling
	// readiness so a tail running against this process sees the banner.
	endpoints := pipeline.Endpoints()
	lifecycleCh <- LifecycleEvent{
		Kind: LifecycleBanner,
		Message: fmt.Sprintf("dash0 otlp proxy listening — http://%s (OTLP/HTTP), %s (OTLP/gRPC) — profile: %s (dataset: %s)",
			endpoints.HTTPEndpoint, endpoints.GRPCEndpoint, profileName, datasetLabel(cfg, flags.Dataset)),
	}
	emitter.EmitStarted(endpoints.HTTPEndpoint, endpoints.GRPCEndpoint, datasetLabel(cfg, flags.Dataset), profileName)

	// Block on signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	select {
	case <-supCtx.Done():
		// upstream cancellation (e.g. test fixture)
	case <-sigCh:
		lifecycleCh <- LifecycleEvent{
			Kind: LifecycleInfo,
			Message: fmt.Sprintf("Shutting down — draining in-flight telemetry (up to %s)",
				proxyShutdownDeadline),
		}
	}

	// Shutdown sequence with bounded deadline.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), proxyShutdownDeadline)
	defer shutdownCancel()

	// 1. Stop accepting new traffic.
	_ = pipeline.Shutdown(shutdownCtx)

	// 2. Cancel the supervisor context so workers and writers wind down.
	supCancel()

	// 3. Emit shutdown event with final cumulative totals.
	finalTotals := [signalCount]int64{
		stats.Forwarded(SignalLogs),
		stats.Forwarded(SignalSpans),
		stats.Forwarded(SignalMetrics),
	}
	reason := "signal"
	if shutdownCtx.Err() != nil {
		reason = "deadline"
	}
	emitter.EmitShutdown(reason, finalTotals)

	// 4. Wait for all goroutines, bounded.
	waitWithDeadline(&wg, shutdownCtx)

	return nil
}

// statsSink wraps an *Emitter into a chan<- SnapshotWithRate that pushes
// each snapshot through EmitStats. RateSampler.Run accepts arbitrary
// sinks; this is the bridge between sampler and agent-mode event stream.
func statsSink(emitter *Emitter) chan<- SnapshotWithRate {
	if emitter == nil {
		return nil
	}
	ch := make(chan SnapshotWithRate, 4)
	go func() {
		for snap := range ch {
			emitter.EmitStats(snap)
		}
	}()
	return ch
}

// waitWithDeadline waits for wg with a deadline. If the deadline expires
// before wg is done, the function returns and leaks the in-flight
// goroutines — they will be torn down by process exit. This is the
// "exceeded drain deadline" path called out in AE15.
func waitWithDeadline(wg *sync.WaitGroup, ctx context.Context) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// resolveProxyConfig validates that the active profile (or env/flag
// override chain) yields enough credentials to run the proxy. Returns
// the resolved configuration plus a display-friendly profile name for
// the banner. Errors are user-facing: missing profile gets the
// `dash0 config profiles create` hint per AE7.
func resolveProxyConfig(ctx context.Context, flags *proxyFlags) (*profiles.Configuration, string, error) {
	cfg := profiles.FromContext(ctx)

	// Effective values after env-var + flag overrides.
	otlpURL := flags.OtlpUrl
	authToken := flags.AuthToken
	if cfg != nil {
		if otlpURL == "" {
			otlpURL = cfg.OtlpUrl
		}
		if authToken == "" {
			authToken = cfg.AuthToken
		}
	}

	if otlpURL == "" || authToken == "" {
		hint := "\n  Hint: create a profile with `dash0 config profiles create`"
		return nil, "", fmt.Errorf("missing required configuration: otlp-url and auth-token must be set%s", hint)
	}

	profileName := resolveProfileNameForBanner(ctx, cfg)
	return cfg, profileName, nil
}

// resolveProfileNameForBanner picks a human-readable name for the banner
// + started event. Resolution order:
//   1. Explicit profile selector (--profile flag or DASH0_PROFILE env)
//      — use that name verbatim.
//   2. Active profile on disk — query the profiles store for the actual
//      name so the banner reads `profile: dev` instead of a placeholder.
//   3. Fallback `env/flags` — only reached when no profile is on disk
//      and connection settings came from env vars or CLI flags.
func resolveProfileNameForBanner(ctx context.Context, cfg *profiles.Configuration) string {
	if sel := config.ProfileSelectorFromContext(ctx); sel.IsSet() {
		return sel.Name
	}
	if cfg != nil {
		if name := lookupActiveProfileName(); name != "" {
			return name
		}
		return "active"
	}
	return "env/flags"
}

// lookupActiveProfileName reads the active profile from the on-disk
// store and returns its name. Errors (no store, no active profile)
// produce an empty string so callers can fall back gracefully.
func lookupActiveProfileName() string {
	store, err := profiles.NewStore()
	if err != nil {
		return ""
	}
	p, err := store.GetActiveProfile()
	if err != nil || p == nil {
		return ""
	}
	return p.Name
}

// datasetLabel returns the dataset string for human-facing banners. The
// resolved-dataset *string in dash0api is nil for "default"; we expand
// that for readability.
func datasetLabel(cfg *profiles.Configuration, flagDataset string) string {
	if flagDataset != "" {
		return flagDataset
	}
	if cfg != nil && cfg.Dataset != "" {
		return cfg.Dataset
	}
	return "default"
}

// Forwarder compatibility with dash0api.Client is asserted at the
// NewWorkerPool call site above by the type-check on the apiClient
// argument; no explicit var-assertion is needed.
