package metrics

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dash0hq/dash0-api-client-go/profiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestInstantResponse() QueryInstantResponse {
	return QueryInstantResponse{
		Status: "success",
		Data: struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  []any     `json:"value"`
			} `json:"result"`
		}{
			ResultType: "vector",
			Result: []struct {
				Metric map[string]string `json:"metric"`
				Value  []any     `json:"value"`
			}{
				{
					Metric: map[string]string{
						"__name__": "dash0_logs",
						"job":      "api",
						"instance": "localhost:8080",
					},
					Value: []any{float64(1609459200), "42"},
				},
			},
		},
	}
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/prometheus/api/v1/query", r.URL.Path)
		assert.NotEmpty(t, r.URL.Query().Get("query"))
		assert.Equal(t, "Bearer test_token", r.Header.Get("Authorization"))
		assert.Contains(t, r.Header.Get("User-Agent"), "dash0-cli/")

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(newTestInstantResponse())
	}))
}

func setTestEnv(t *testing.T, serverURL string) {
	t.Helper()
	t.Setenv("DASH0_API_URL", serverURL)
	t.Setenv("DASH0_AUTH_TOKEN", "test_token")
}

func TestInstantCmdWithPromql(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setTestEnv(t, server.URL)

	cmd := newInstantCmd()
	cmd.SetArgs([]string{"--promql", `{otel_metric_name="dash0.logs"}`})
	// Redirect stdout to avoid test output noise.
	cmd.SetOut(os.Stderr)
	require.NoError(t, cmd.Execute())
}

func TestInstantCmdWithDeprecatedQueryFlag(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setTestEnv(t, server.URL)

	cmd := newInstantCmd()
	cmd.SetArgs([]string{"--query", `{otel_metric_name="dash0.logs"}`})
	cmd.SetOut(os.Stderr)
	require.NoError(t, cmd.Execute())
}

func TestInstantCmdWithFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify that the filter was translated to PromQL.
		queryParam := r.URL.Query().Get("query")
		assert.Contains(t, queryParam, `otel_metric_name="dash0.logs"`)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(newTestInstantResponse())
	}))
	defer server.Close()
	setTestEnv(t, server.URL)

	cmd := newInstantCmd()
	cmd.SetArgs([]string{"--filter", "otel_metric_name is dash0.logs"})
	cmd.SetOut(os.Stderr)
	require.NoError(t, cmd.Execute())
}

func TestInstantCmdMutuallyExclusiveFlags(t *testing.T) {
	cmd := newInstantCmd()
	cmd.SetArgs([]string{"--promql", "up", "--filter", "job is api"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestInstantCmdRequiresPromqlOrFilter(t *testing.T) {
	cmd := newInstantCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "either --promql or --filter must be specified")
}

func TestInstantCmdJSONOutput(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setTestEnv(t, server.URL)

	// Capture stdout.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := newInstantCmd()
	cmd.SetArgs([]string{"--promql", `{otel_metric_name="dash0.logs"}`, "-o", "json"})
	require.NoError(t, cmd.Execute())

	w.Close()
	os.Stdout = old

	out, _ := io.ReadAll(r)
	output := string(out)

	// Verify it's valid JSON with the expected structure.
	var response QueryInstantResponse
	require.NoError(t, json.Unmarshal([]byte(output), &response))
	assert.Equal(t, "success", response.Status)
	assert.Equal(t, "vector", response.Data.ResultType)
	assert.Len(t, response.Data.Result, 1)
}

func TestInstantCmdCSVOutput(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setTestEnv(t, server.URL)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := newInstantCmd()
	cmd.SetArgs([]string{"--promql", `{otel_metric_name="dash0.logs"}`, "-o", "csv"})
	require.NoError(t, cmd.Execute())

	w.Close()
	os.Stdout = old

	out, _ := io.ReadAll(r)
	output := string(out)

	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.GreaterOrEqual(t, len(lines), 2) // header + at least 1 row
	assert.Contains(t, lines[0], "timestamp") // CSV header uses canonical keys
	assert.Contains(t, lines[0], "value")
}

func TestInstantCmdCSVSkipHeader(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setTestEnv(t, server.URL)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := newInstantCmd()
	cmd.SetArgs([]string{"--promql", `{otel_metric_name="dash0.logs"}`, "-o", "csv", "--skip-header"})
	require.NoError(t, cmd.Execute())

	w.Close()
	os.Stdout = old

	out, _ := io.ReadAll(r)
	output := string(out)

	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Len(t, lines, 1)                    // only data, no header
	assert.NotContains(t, lines[0], "__name__") // no header keys
}

func TestInstantCmdRejectsAutoColumns(t *testing.T) {
	for _, col := range []string{"value", "timestamp", "time"} {
		t.Run(col, func(t *testing.T) {
			cmd := newInstantCmd()
			cmd.SetArgs([]string{"--promql", "up", "--column", col})

			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "always included")
		})
	}
}

func TestInstantCmdUnsupportedColumn(t *testing.T) {
	cmd := newInstantCmd()
	cmd.SetArgs([]string{"--promql", "up", "--column", "otel_metric_name"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not available")
	assert.Contains(t, err.Error(), "__name__")
}

func TestInstantCmdColumnWithJSON(t *testing.T) {
	cmd := newInstantCmd()
	cmd.SetArgs([]string{"--promql", "up", "-o", "json", "--column", "__name__"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "column")
}

func TestInstantCmdAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(QueryInstantResponse{
			Status:    "error",
			Error:     "execution error",
			ErrorType: "execution",
		})
	}))
	defer server.Close()
	setTestEnv(t, server.URL)

	cmd := newInstantCmd()
	cmd.SetArgs([]string{"--promql", "invalid_query"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "execution error")
}

func TestInstantCmdDeprecatedTimeFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "2024-01-25T10:00:00.000Z", r.URL.Query().Get("time"))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(newTestInstantResponse())
	}))
	defer server.Close()
	setTestEnv(t, server.URL)

	cmd := newInstantCmd()
	cmd.SetArgs([]string{"--promql", `{otel_metric_name="dash0.logs"}`, "--time", "2024-01-25T10:00:00Z"})
	cmd.SetOut(os.Stderr)
	require.NoError(t, cmd.Execute())
}

func TestParseQueryFormat(t *testing.T) {
	tests := []struct {
		input string
		want  queryFormat
	}{
		{"table", queryFormatTable},
		{"TABLE", queryFormatTable},
		{"json", queryFormatJSON},
		{"JSON", queryFormatJSON},
		{"csv", queryFormatCSV},
		{"CSV", queryFormatCSV},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseQueryFormat(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("invalid format", func(t *testing.T) {
		_, err := parseQueryFormat("xml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported output format")
	})

	t.Run("empty defaults to table", func(t *testing.T) {
		got, err := parseQueryFormat("")
		require.NoError(t, err)
		assert.Equal(t, queryFormatTable, got)
	})
}

// TestInstantCmdHonorsProfileFromContext is a regression test for
// https://github.com/dash0hq/dash0-cli/issues/221: runInstant used to call
// profiles.ResolveConfiguration directly, ignoring the per-invocation profile
// (--profile / DASH0_PROFILE) that cmd/dash0/main.go stores on the context.
// It must instead route through client.NewRawHTTPConfig so the context-carried
// profile wins over environment variables and the on-disk active profile.
func TestInstantCmdHonorsProfileFromContext(t *testing.T) {
	// Env-var sink: if runInstant regressed to profiles.ResolveConfiguration,
	// it would resolve against these vars and hit this server. The context
	// must trump this.
	var envHits atomic.Int32
	envServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(newTestInstantResponse())
	}))
	t.Cleanup(envServer.Close)

	// Context-carried profile: this is where the request must go.
	var (
		contextHits    atomic.Int32
		contextAuth    string
		contextDataset string
	)
	contextServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextHits.Add(1)
		contextAuth = r.Header.Get("Authorization")
		contextDataset = r.URL.Query().Get("dataset")

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(newTestInstantResponse())
	}))
	t.Cleanup(contextServer.Close)

	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())
	t.Setenv("DASH0_API_URL", envServer.URL)
	t.Setenv("DASH0_AUTH_TOKEN", "auth_env_token")
	t.Setenv("DASH0_DATASET", "env-dataset")

	ctx := profiles.WithConfiguration(context.Background(), &profiles.Configuration{
		ApiUrl:    contextServer.URL,
		AuthToken: "auth_context_token",
		Dataset:   "context-dataset",
	})

	cmd := newInstantCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"--promql", "up"})
	cmd.SetOut(os.Stderr)
	require.NoError(t, cmd.Execute())

	assert.Equal(t, int32(0), envHits.Load(), "env-var/active-profile server must not be hit when a profile is carried on the context")
	assert.Equal(t, int32(1), contextHits.Load(), "context-carried profile server must be hit exactly once")
	assert.Equal(t, "Bearer auth_context_token", contextAuth, "auth token must come from the context profile")
	assert.Equal(t, "context-dataset", contextDataset, "dataset must come from the context profile")
}

// TestInstantCmdDatasetFlagOverridesContextProfile complements the regression
// test above: when both --dataset and a context-carried profile are present,
// the explicit flag wins. This guards against a future refactor that starts
// letting the context clobber flag overrides.
func TestInstantCmdDatasetFlagOverridesContextProfile(t *testing.T) {
	var dataset string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dataset = r.URL.Query().Get("dataset")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(newTestInstantResponse())
	}))
	t.Cleanup(server.Close)

	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	ctx := profiles.WithConfiguration(context.Background(), &profiles.Configuration{
		ApiUrl:    server.URL,
		AuthToken: "auth_context_token",
		Dataset:   "context-dataset",
	})

	cmd := newInstantCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"--promql", "up", "--dataset", "flag-dataset"})
	cmd.SetOut(os.Stderr)
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "flag-dataset", dataset)
}
