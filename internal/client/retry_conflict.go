package client

import (
	"context"
	"math/rand"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

const (
	// conflictRetryWaitMin is the starting point for the exponential backoff
	// between dataset-version-conflict retries. It is shorter than the general
	// request retry transport's 500ms floor because a dataset-version race is
	// expected to resolve almost immediately — the other writer, not a rate
	// limiter or an overloaded server, is what is being waited on.
	conflictRetryWaitMin = 100 * time.Millisecond
	// conflictRetryWaitMax caps the backoff between retries.
	conflictRetryWaitMax = 2 * time.Second
)

// conflictRetryWait pauses for d, honoring context cancellation. Tests
// override this so retry backoff does not slow down the suite.
var conflictRetryWait = func(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// RetryOnConflict runs fn and retries it while it fails with a 409 dataset
// version conflict (dash0api.IsConflict), up to the --max-retries /
// DASH0_MAX_RETRIES budget (see resolveMaxRetries), with jittered exponential
// backoff between attempts.
//
// A Dash0 dataset is guarded by an optimistic-concurrency "dataset version":
// two writes to the same dataset that race one another leave exactly one
// winner, and the loser gets a 409 whose message literally says "please
// retry". Nothing below this call retries that: the api-client-go transport's
// own retry logic covers only 429 and 5xx (see shouldRetry in that module's
// transport.go). Every dataset-scoped asset write (dashboards, check rules,
// views, recording rules, synthetic checks, spam filters, notification
// channels, teams) must go through here instead of calling the API client
// directly, so a losing write converges on retry instead of surfacing as a
// fatal error.
//
// This retries the CLI's own losing write; it does not prevent the race
// itself, and it does not need to — the dash0 CLI has no intra-process
// concurrency in its asset-write paths (apply applies documents from a single
// sequential loop, and no other command parallelizes writes), so the only way
// this race occurs is across separate `dash0` process invocations: a CI
// matrix, or a script/agent running concurrent `apply`/`create`/`update`
// commands against the same dataset. An in-process lock (the fix used in the
// Terraform provider, which does have intra-process concurrency via
// Terraform's own parallel resource graph) would not address that. See
// https://github.com/dash0hq/dash0-cli/issues/261.
func RetryOnConflict[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	maxRetries, err := resolveMaxRetries(ctx)
	if err != nil {
		// --max-retries/DASH0_MAX_RETRIES was already validated once when the
		// API client was constructed for this invocation; a failure here would
		// mean that validation regressed. Fail safe by not retrying at all
		// rather than looping on a config error.
		maxRetries = 0
	}

	var result T
	for attempt := 0; ; attempt++ {
		result, err = fn()
		if err == nil || !dash0api.IsConflict(err) || attempt >= maxRetries {
			return result, err
		}
		if waitErr := conflictRetryWait(ctx, conflictBackoff(attempt)); waitErr != nil {
			return result, waitErr
		}
	}
}

// conflictBackoff returns a jittered backoff for the given (0-based) retry
// attempt: full jitter (a random duration in [0, cap]) over an exponential
// cap that doubles from conflictRetryWaitMin and saturates at
// conflictRetryWaitMax. Full jitter spreads out retries from multiple racing
// CLI processes so they do not all wake up and collide again at once.
func conflictBackoff(attempt int) time.Duration {
	ceiling := conflictRetryWaitMin * time.Duration(1<<attempt)
	if ceiling <= 0 || ceiling > conflictRetryWaitMax {
		ceiling = conflictRetryWaitMax
	}
	return time.Duration(rand.Int63n(int64(ceiling)))
}
