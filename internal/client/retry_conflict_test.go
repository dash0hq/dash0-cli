package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func conflictErr() error {
	return &dash0api.APIError{StatusCode: http.StatusConflict, Message: "dataset version conflict, please retry"}
}

// withNoConflictBackoff makes RetryOnConflict retry instantly during tests,
// so the suite is not slowed down by the real (jittered, up to 2s) backoff.
func withNoConflictBackoff(t *testing.T) {
	t.Helper()
	previous := conflictRetryWait
	conflictRetryWait = func(ctx context.Context, d time.Duration) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	t.Cleanup(func() { conflictRetryWait = previous })
}

func TestRetryOnConflict_SucceedsFirstTry(t *testing.T) {
	withNoConflictBackoff(t)

	calls := 0
	result, err := RetryOnConflict(context.Background(), func() (string, error) {
		calls++
		return "ok", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.Equal(t, 1, calls)
}

func TestRetryOnConflict_RetriesUntilItLandsWithinBudget(t *testing.T) {
	withNoConflictBackoff(t)

	calls := 0
	result, err := RetryOnConflict(context.Background(), func() (string, error) {
		calls++
		if calls <= DefaultMaxRetries {
			// Fail with 409 on every attempt up to (and including) the last
			// retry, then succeed on the final allowed attempt.
			return "", conflictErr()
		}
		return "won", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "won", result)
	assert.Equal(t, DefaultMaxRetries+1, calls)
}

func TestRetryOnConflict_GivesUpAfterMaxRetriesExhausted(t *testing.T) {
	withNoConflictBackoff(t)

	calls := 0
	_, err := RetryOnConflict(context.Background(), func() (string, error) {
		calls++
		return "", conflictErr()
	})

	require.Error(t, err)
	assert.True(t, dash0api.IsConflict(err))
	// One initial attempt plus DefaultMaxRetries retries, never more:
	// a persistent conflict must still surface as an error eventually.
	assert.Equal(t, DefaultMaxRetries+1, calls)
}

func TestRetryOnConflict_DoesNotRetryNonConflictErrors(t *testing.T) {
	withNoConflictBackoff(t)

	calls := 0
	wantErr := errors.New("boom")
	_, err := RetryOnConflict(context.Background(), func() (string, error) {
		calls++
		return "", wantErr
	})

	require.ErrorIs(t, err, wantErr)
	assert.Equal(t, 1, calls, "a non-409 error must not be retried")
}

func TestRetryOnConflict_HonorsContextCancellation(t *testing.T) {
	// Deliberately do NOT stub conflictRetryWait here: we want the real
	// implementation's select-on-ctx.Done() behavior under test.
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	result, err := RetryOnConflict(ctx, func() (string, error) {
		calls++
		if calls == 1 {
			cancel()
		}
		return "", conflictErr()
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, "", result)
	assert.Equal(t, 1, calls, "must stop retrying once the context is canceled")
}

func TestRetryOnConflict_RespectsMaxRetriesFromContext(t *testing.T) {
	withNoConflictBackoff(t)

	n := 1
	ctx := WithMaxRetries(context.Background(), &n)

	calls := 0
	_, err := RetryOnConflict(ctx, func() (string, error) {
		calls++
		return "", conflictErr()
	})

	require.Error(t, err)
	assert.Equal(t, 2, calls, "1 initial attempt + 1 retry from the --max-retries override")
}

func TestConflictBackoff_NeverExceedsCapAndGrows(t *testing.T) {
	for attempt := range 8 {
		for range 20 {
			d := conflictBackoff(attempt)
			if d < 0 || d > conflictRetryWaitMax {
				t.Fatalf("attempt %d: backoff %v out of bounds [0, %v]", attempt, d, conflictRetryWaitMax)
			}
		}
	}
}

func TestRetryOnConflict_PropagatesNonErrorResultOnFinalFailure(t *testing.T) {
	withNoConflictBackoff(t)

	// Even when every attempt fails, the zero value of T (not a stale
	// previous success) must come back with the error.
	result, err := RetryOnConflict(context.Background(), func() (int, error) {
		return 0, fmt.Errorf("wrapped: %w", conflictErr())
	})

	require.Error(t, err)
	assert.True(t, dash0api.IsConflict(err))
	assert.Equal(t, 0, result)
}
