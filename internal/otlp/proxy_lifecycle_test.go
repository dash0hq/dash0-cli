package otlp

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dash0hq/dash0-api-client-go/profiles"
	"go.opentelemetry.io/collector/pdata/plog"

	"github.com/dash0hq/dash0-cli/internal/config"
)

func TestResolveProxyConfig_FromContextProfile(t *testing.T) {
	// Isolate from the developer's real ~/.dash0/ so the active-profile
	// store lookup returns nothing and we exercise the fallback branch.
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	cfg := &profiles.Configuration{
		OtlpUrl:   "https://ingress.example.com",
		AuthToken: "tok",
		Dataset:   "staging",
	}
	ctx := profiles.WithConfiguration(context.Background(), cfg)
	flags := &proxyFlags{}

	got, name, err := resolveProxyConfig(ctx, flags)
	if err != nil {
		t.Fatalf("resolveProxyConfig: %v", err)
	}
	if got != cfg {
		t.Errorf("returned cfg != input cfg")
	}
	if name != "active" {
		t.Errorf("profile name = %q; want 'active' (no on-disk profile in temp config dir)", name)
	}
}

func TestResolveProxyConfig_FlagOverridesProfile(t *testing.T) {
	cfg := &profiles.Configuration{
		OtlpUrl:   "https://ingress.profile.com",
		AuthToken: "profile-tok",
	}
	ctx := profiles.WithConfiguration(context.Background(), cfg)
	flags := &proxyFlags{
		OtlpUrl:   "https://ingress.override.com",
		AuthToken: "override-tok",
	}

	_, _, err := resolveProxyConfig(ctx, flags)
	if err != nil {
		t.Fatalf("flag override should succeed: %v", err)
	}
}

func TestResolveProxyConfig_NoProfileNoFlagsFails(t *testing.T) {
	flags := &proxyFlags{}
	_, _, err := resolveProxyConfig(context.Background(), flags)
	if err == nil {
		t.Fatal("expected error when no profile + no flags; got nil")
	}
	if !strings.Contains(err.Error(), "dash0 config profiles create") {
		t.Errorf("error should hint at profile creation; got: %v", err)
	}
}

func TestResolveProxyConfig_MissingTokenFails(t *testing.T) {
	cfg := &profiles.Configuration{
		OtlpUrl: "https://ingress.example.com",
		// AuthToken missing
	}
	ctx := profiles.WithConfiguration(context.Background(), cfg)
	flags := &proxyFlags{}

	_, _, err := resolveProxyConfig(ctx, flags)
	if err == nil {
		t.Fatal("expected error when auth-token is missing; got nil")
	}
}

func TestResolveProxyConfig_MissingOtlpURLFails(t *testing.T) {
	cfg := &profiles.Configuration{
		AuthToken: "tok",
		// OtlpUrl missing
	}
	ctx := profiles.WithConfiguration(context.Background(), cfg)
	flags := &proxyFlags{}

	_, _, err := resolveProxyConfig(ctx, flags)
	if err == nil {
		t.Fatal("expected error when otlp-url is missing; got nil")
	}
}

func TestResolveProfileNameForBanner_ExplicitSelectorWins(t *testing.T) {
	ctx := config.WithProfileSelector(context.Background(), config.ProfileSelector{
		Name:   "prod",
		Source: config.ProfileSourceFlag,
	})
	if got := resolveProfileNameForBanner(ctx, &profiles.Configuration{}); got != "prod" {
		t.Errorf("got %q; want prod", got)
	}
}

func TestResolveProfileNameForBanner_ConfigOnly_NoStore(t *testing.T) {
	// Empty config dir → lookupActiveProfileName returns "" → fallback
	// label "active" is emitted.
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())
	if got := resolveProfileNameForBanner(context.Background(), &profiles.Configuration{}); got != "active" {
		t.Errorf("got %q; want 'active' (fallback when store has no active profile)", got)
	}
}

func TestResolveProfileNameForBanner_NoConfigNoSelector(t *testing.T) {
	if got := resolveProfileNameForBanner(context.Background(), nil); got != "env/flags" {
		t.Errorf("got %q; want 'env/flags'", got)
	}
}

func TestDatasetLabel_FlagOverridesConfig(t *testing.T) {
	cfg := &profiles.Configuration{Dataset: "from-config"}
	if got := datasetLabel(cfg, "from-flag"); got != "from-flag" {
		t.Errorf("got %q; want from-flag", got)
	}
}

func TestDatasetLabel_ConfigUsedWhenNoFlag(t *testing.T) {
	cfg := &profiles.Configuration{Dataset: "staging"}
	if got := datasetLabel(cfg, ""); got != "staging" {
		t.Errorf("got %q; want staging", got)
	}
}

func TestDatasetLabel_FallsBackToDefault(t *testing.T) {
	if got := datasetLabel(nil, ""); got != "default" {
		t.Errorf("got %q; want default", got)
	}
	if got := datasetLabel(&profiles.Configuration{}, ""); got != "default" {
		t.Errorf("got %q for empty cfg; want default", got)
	}
}

func TestWaitWithDeadline_FinishesBeforeDeadline(t *testing.T) {
	// WaitGroup finishes before context expires.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		time.Sleep(10 * time.Millisecond)
		wg.Done()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	waitWithDeadline(&wg, ctx)
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("waitWithDeadline took too long: %v (should return immediately when wg done)", elapsed)
	}
}

func TestWaitWithDeadline_GivesUpOnDeadline(t *testing.T) {
	// WaitGroup never finishes; deadline expires.
	var wg sync.WaitGroup
	wg.Add(1)
	// never wg.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	start := time.Now()
	waitWithDeadline(&wg, ctx)
	elapsed := time.Since(start)
	if elapsed < 20*time.Millisecond || elapsed > 100*time.Millisecond {
		t.Errorf("waitWithDeadline elapsed = %v; expected ~30ms", elapsed)
	}
}

func TestStatsSink_ForwardsToEmitter(t *testing.T) {
	// statsSink wraps an emitter into a chan<- SnapshotWithRate that
	// pushes each snapshot through EmitStats.
	eventCh := make(chan plog.Logs, 4)
	emitter := NewEmitter("inst", eventCh)

	sink := statsSink(emitter)
	if sink == nil {
		t.Fatal("statsSink returned nil for non-nil emitter")
	}

	snap := SnapshotWithRate{
		Snapshot: Snapshot{Forwarded: [signalCount]int64{1, 2, 3}},
		Rate:     [signalCount]float64{4, 5, 6},
	}
	sink <- snap

	select {
	case ld := <-eventCh:
		// One stats event should have flowed through the emitter.
		rl := ld.ResourceLogs().At(0)
		sl := rl.ScopeLogs().At(0)
		if sl.LogRecords().At(0).EventName() != eventStats {
			t.Errorf("event name = %q; want %q",
				sl.LogRecords().At(0).EventName(), eventStats)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("emitter received no stats event from sink")
	}
}

func TestStatsSink_NilEmitterReturnsNil(t *testing.T) {
	if got := statsSink(nil); got != nil {
		t.Error("statsSink(nil) should return nil so RateSampler.Run skips this fan-out")
	}
}
