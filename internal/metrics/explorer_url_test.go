package metrics

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPromQL = `sum(rate(http_server_request_duration_seconds_count[5m]))`

func runInstantWithMockServer(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/prometheus/api/v1/query", testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       newTestInstantResponse(),
	})
	setTestEnv(t, server.URL)

	cmd := newInstantCmd()
	cmd.SetArgs(args)
	var err error
	stderr = testutil.CaptureStderr(t, func() {
		stdout = testutil.CaptureStdout(t, func() {
			err = cmd.Execute()
		})
	})
	require.NoError(t, err)
	return stdout, stderr
}

func explorerURLFromStderr(t *testing.T, stderr string) *url.URL {
	t.Helper()
	_, after, found := strings.Cut(stderr, "Open this query in Dash0:")
	require.True(t, found, "stderr must carry the deep link, got: %s", stderr)
	link, err := url.Parse(strings.TrimSpace(after))
	require.NoError(t, err)
	return link
}

func TestInstantCmd_ExplorerURLGoesToStderrInTableOutput(t *testing.T) {
	stdout, stderr := runInstantWithMockServer(t, "--promql", testPromQL)

	link := explorerURLFromStderr(t, stderr)
	assert.Equal(t, "/goto/metrics/query-builder", link.Path)
	assert.Equal(t, testPromQL, link.Query().Get("query"))
	assert.NotContains(t, link.Query(), "from", "an evaluation at now needs no time range")
	assert.NotContains(t, stdout, "Open this query in Dash0:")
}

func TestInstantCmd_ExplorerURLGoesToStderrInCSVOutput(t *testing.T) {
	stdout, stderr := runInstantWithMockServer(t, "--promql", testPromQL, "-o", "csv")

	explorerURLFromStderr(t, stderr)
	assert.NotContains(t, stdout, "Open this query in Dash0:")
}

func TestInstantCmd_ExplorerURLIsEmbeddedInJSONOutput(t *testing.T) {
	stdout, stderr := runInstantWithMockServer(t, "--promql", testPromQL, "-o", "json")

	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	assert.Contains(t, payload, "status", "the Prometheus payload stays at the top level")
	assert.Contains(t, payload, "data")

	var explorerURL string
	require.NoError(t, json.Unmarshal(payload["dash0ExplorerUrl"], &explorerURL))
	assert.Contains(t, explorerURL, "/goto/metrics/query-builder")
	assert.NotContains(t, stderr, "Open this query in Dash0:")
}

func TestInstantCmd_ExplorerURLEndsAtAnAbsoluteEvaluationTime(t *testing.T) {
	_, stderr := runInstantWithMockServer(t, "--promql", testPromQL, "--from", "2026-11-15T10:00:00Z")

	query := explorerURLFromStderr(t, stderr).Query()
	assert.Equal(t, "2026-11-15T09:00:00.000Z", query.Get("from"))
	assert.Equal(t, "2026-11-15T10:00:00.000Z", query.Get("to"))
}

func TestInstantCmd_ExplorerURLCarriesNamedDataset(t *testing.T) {
	_, stderr := runInstantWithMockServer(t, "--promql", testPromQL, "--dataset", "staging")

	assert.Equal(t, "staging", explorerURLFromStderr(t, stderr).Query().Get("dataset"))
}

// The web app opens the default dataset anyway, so the link leaves it out.
func TestInstantCmd_ExplorerURLOmitsDefaultDataset(t *testing.T) {
	_, stderr := runInstantWithMockServer(t, "--promql", testPromQL, "--dataset", "default")

	assert.NotContains(t, explorerURLFromStderr(t, stderr).Query(), "dataset")
}

func TestInstantCmd_ExplorerURLCarriesThePromQLOfFilters(t *testing.T) {
	_, stderr := runInstantWithMockServer(t, "--filter", "service.name is my-service")

	assert.Equal(t, `{service_name="my-service"}`, explorerURLFromStderr(t, stderr).Query().Get("query"))
}

func TestEvaluationTime(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		from   string
		want   time.Time
		wantOK bool
	}{
		{"", time.Time{}, false},
		{"now", time.Time{}, false},
		{"now-2h", time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC), true},
		{"now-1d", time.Time{}, false},
		{"2026-11-15T10:00:00.000Z", time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC), true},
	}
	for _, tt := range tests {
		t.Run(tt.from, func(t *testing.T) {
			got, ok := evaluationTime(tt.from, now)
			assert.Equal(t, tt.wantOK, ok)
			assert.True(t, tt.want.Equal(got), "got %s, want %s", got, tt.want)
		})
	}
}
