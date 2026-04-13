package metrics

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

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
