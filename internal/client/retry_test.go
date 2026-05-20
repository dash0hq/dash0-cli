package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// throttledServer returns a handler that responds with 429 for the first
// failCount requests, then responds with 200 and a valid JSON body.
func throttledServer(failCount int, requestCount *atomic.Int32) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requestCount.Add(1)
		if int(n) <= failCount {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"rate limited"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"kind":"Dashboard","metadata":{"name":"test"},"spec":{}}`))
	})
}

func newTestClient(t *testing.T, serverURL string, maxRetries int) dash0api.Client {
	t.Helper()
	c, err := dash0api.NewClient(
		dash0api.WithApiUrl(serverURL),
		dash0api.WithAuthToken("auth_test-token-12345"),
		dash0api.WithUserAgent("dash0-cli/test"),
		dash0api.WithMaxRetries(maxRetries),
		dash0api.WithRetryWaitMin(1*time.Millisecond),
		dash0api.WithRetryWaitMax(5*time.Millisecond),
	)
	require.NoError(t, err)
	return c
}

func TestRetry_SucceedsAfterTransientThrottling(t *testing.T) {
	tests := []struct {
		name       string
		maxRetries int
		failCount  int
		wantOK     bool
		wantTotal  int32
	}{
		{
			name:       "succeeds on first attempt (no 429s)",
			maxRetries: 3,
			failCount:  0,
			wantOK:     true,
			wantTotal:  1,
		},
		{
			name:       "succeeds after 1 retry",
			maxRetries: 3,
			failCount:  1,
			wantOK:     true,
			wantTotal:  2,
		},
		{
			name:       "succeeds after all retries exhausted",
			maxRetries: 3,
			failCount:  3,
			wantOK:     true,
			wantTotal:  4, // 1 initial + 3 retries
		},
		{
			name:       "fails when retries not enough",
			maxRetries: 2,
			failCount:  3,
			wantOK:     false,
			wantTotal:  3, // 1 initial + 2 retries, all got 429
		},
		{
			name:       "no retries configured",
			maxRetries: 0,
			failCount:  1,
			wantOK:     false,
			wantTotal:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requestCount atomic.Int32
			server := httptest.NewServer(throttledServer(tt.failCount, &requestCount))
			t.Cleanup(server.Close)

			c := newTestClient(t, server.URL, tt.maxRetries)
			_, err := c.GetDashboard(t.Context(), "test-id", nil)

			if tt.wantOK {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), fmt.Sprintf("%d", http.StatusTooManyRequests))
			}
			assert.Equal(t, tt.wantTotal, requestCount.Load())
		})
	}
}
