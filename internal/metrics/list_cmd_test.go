package metrics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func labelValuesResponse(names []string) LabelValuesResponse {
	return LabelValuesResponse{
		Status: "success",
		Data:   names,
	}
}

func metadataResponse(data map[string][]MetadataEntry) MetadataResponse {
	return MetadataResponse{
		Status: "success",
		Data:   data,
	}
}

var sampleNames = []string{
	"container_cpu_usage_seconds_total",
	"http_server_active_requests",
	"http_server_request_duration_seconds_bucket",
	"http_server_request_duration_seconds_count",
	"process_cpu_seconds_total",
}

var sampleMetadata = map[string][]MetadataEntry{
	"container_cpu_usage_seconds_total": {{
		Type: "counter",
		Help: "Cumulative cpu time consumed in seconds",
		Unit: "",
	}},
	"http_server_active_requests": {{
		Type: "gauge",
		Help: "Number of active HTTP server requests",
		Unit: "{request}",
	}},
	"http_server_request_duration_seconds_bucket": {{
		Type: "histogram",
		Help: "Duration of HTTP server requests",
		Unit: "s",
	}},
	"http_server_request_duration_seconds_count": {{
		Type: "histogram",
		Help: "Duration of HTTP server requests",
		Unit: "s",
	}},
	"process_cpu_seconds_total": {{
		Type: "counter",
		Help: "Total user and system CPU time spent in seconds",
		Unit: "s",
	}},
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test_token", r.Header.Get("Authorization"))
		assert.NotEmpty(t, r.Header.Get("User-Agent"))

		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/label/__name__/values"):
			assert.NotEmpty(t, r.URL.Query().Get("start"), "start param is required")
			assert.NotEmpty(t, r.URL.Query().Get("end"), "end param is required")
			json.NewEncoder(w).Encode(labelValuesResponse(sampleNames))

		case strings.Contains(r.URL.Path, "/metadata"):
			json.NewEncoder(w).Encode(metadataResponse(sampleMetadata))

		default:
			t.Errorf("unexpected request path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func newErrorServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LabelValuesResponse{
			Status: "error",
			Error:  "something went wrong",
		})
	}))
}

func setupEnv(t *testing.T, serverURL string) {
	t.Helper()
	t.Setenv("DASH0_API_URL", serverURL)
	t.Setenv("DASH0_AUTH_TOKEN", "test_token")
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())
}

func execListCmd(t *testing.T, args ...string) (*cobra.Command, string, error) {
	t.Helper()

	// Wrap in a root command with the experimental flag so RequireExperimental works
	root := &cobra.Command{Use: "dash0"}
	root.PersistentFlags().BoolP("experimental", "X", false, "")

	metricsCmd := &cobra.Command{Use: "metrics"}
	metricsCmd.AddCommand(newListCmd())
	root.AddCommand(metricsCmd)

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		root.SetArgs(append([]string{"-X", "metrics", "list"}, args...))
		cmdErr = root.Execute()
	})
	return root, output, cmdErr
}

func TestListTable(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setupEnv(t, server.URL)

	_, output, err := execListCmd(t)
	require.NoError(t, err)

	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "http_server_active_requests")
	assert.Contains(t, output, "container_cpu_usage_seconds_total")
	// Table output should NOT contain type/unit metadata
	assert.NotContains(t, output, "gauge")
	assert.NotContains(t, output, "counter")
}

func TestListWide(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setupEnv(t, server.URL)

	_, output, err := execListCmd(t, "-o", "wide")
	require.NoError(t, err)

	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "TYPE")
	assert.Contains(t, output, "UNIT")
	assert.Contains(t, output, "DESCRIPTION")
	assert.Contains(t, output, "http_server_active_requests")
	assert.Contains(t, output, "gauge")
	assert.Contains(t, output, "{request}")
	assert.Contains(t, output, "Number of active HTTP server requests")
}

func TestListJSON(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setupEnv(t, server.URL)

	_, output, err := execListCmd(t, "-o", "json")
	require.NoError(t, err)

	var metrics []MetricInfo
	require.NoError(t, json.Unmarshal([]byte(output), &metrics))
	assert.Len(t, metrics, 5)

	// Verify sorted by name
	assert.Equal(t, "container_cpu_usage_seconds_total", metrics[0].Name)
	assert.Equal(t, "counter", metrics[0].Type)

	assert.Equal(t, "http_server_active_requests", metrics[1].Name)
	assert.Equal(t, "gauge", metrics[1].Type)
	assert.Equal(t, "{request}", metrics[1].Unit)
	assert.Equal(t, "Number of active HTTP server requests", metrics[1].Help)
}

func TestListCSV(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setupEnv(t, server.URL)

	_, output, err := execListCmd(t, "-o", "csv")
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Equal(t, "name,type,unit,description", lines[0])
	assert.GreaterOrEqual(t, len(lines), 2)
	// Check a data row
	assert.Contains(t, output, "http_server_active_requests,gauge,{request},Number of active HTTP server requests")
}

func TestListCSVSkipHeader(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setupEnv(t, server.URL)

	_, output, err := execListCmd(t, "-o", "csv", "--skip-header")
	require.NoError(t, err)

	assert.NotContains(t, output, "name,type,unit,description")
	assert.Contains(t, output, "http_server_active_requests")
}

func TestListTableSkipHeader(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setupEnv(t, server.URL)

	_, output, err := execListCmd(t, "--skip-header")
	require.NoError(t, err)

	assert.NotContains(t, output, "NAME")
	assert.Contains(t, output, "http_server_active_requests")
}

func TestListFilterSubstring(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setupEnv(t, server.URL)

	_, output, err := execListCmd(t, "--filter", "http_server")
	require.NoError(t, err)

	assert.Contains(t, output, "http_server_active_requests")
	assert.Contains(t, output, "http_server_request_duration_seconds_bucket")
	assert.NotContains(t, output, "container_cpu_usage_seconds_total")
	assert.NotContains(t, output, "process_cpu_seconds_total")
}

func TestListFilterRegex(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setupEnv(t, server.URL)

	_, output, err := execListCmd(t, "--filter", "^http_server.*count$")
	require.NoError(t, err)

	assert.Contains(t, output, "http_server_request_duration_seconds_count")
	assert.NotContains(t, output, "http_server_active_requests")
	assert.NotContains(t, output, "http_server_request_duration_seconds_bucket")
}

func TestListLimit(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setupEnv(t, server.URL)

	_, output, err := execListCmd(t, "--limit", "2")
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(output), "\n")
	// Header + 2 data rows
	assert.Equal(t, 3, len(lines))
}

func TestListFilterAndLimit(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setupEnv(t, server.URL)

	_, output, err := execListCmd(t, "--filter", "http_server", "--limit", "1")
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(output), "\n")
	// Header + 1 data row
	assert.Equal(t, 2, len(lines))
}

func TestListEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(labelValuesResponse([]string{}))
	}))
	defer server.Close()
	setupEnv(t, server.URL)

	_, _, err := execListCmd(t)
	// "No metrics found." is printed to stderr, command still succeeds
	require.NoError(t, err)
}

func TestListFilterNoMatch(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setupEnv(t, server.URL)

	_, _, err := execListCmd(t, "--filter", "nonexistent_metric_xyz")
	require.NoError(t, err)
}

func TestListAPIError(t *testing.T) {
	server := newErrorServer(t)
	defer server.Close()
	setupEnv(t, server.URL)

	_, _, err := execListCmd(t)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "something went wrong")
}

func TestListRequiresExperimental(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setupEnv(t, server.URL)

	root := &cobra.Command{Use: "dash0"}
	root.PersistentFlags().BoolP("experimental", "X", false, "")

	metricsCmd := &cobra.Command{Use: "metrics"}
	metricsCmd.AddCommand(newListCmd())
	root.AddCommand(metricsCmd)

	root.SetArgs([]string{"metrics", "list"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental")
}

func TestListTimeRangeParams(t *testing.T) {
	var capturedStart, capturedEnd string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/label/__name__/values") {
			capturedStart = r.URL.Query().Get("start")
			capturedEnd = r.URL.Query().Get("end")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(labelValuesResponse(sampleNames))
	}))
	defer server.Close()
	setupEnv(t, server.URL)

	_, _, err := execListCmd(t, "--from", "now-6h", "--to", "now")
	require.NoError(t, err)

	assert.NotEmpty(t, capturedStart, "start param should be set")
	assert.NotEmpty(t, capturedEnd, "end param should be set")
}

func TestListDatasetParam(t *testing.T) {
	var capturedDataset string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedDataset = r.URL.Query().Get("dataset")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(labelValuesResponse(sampleNames))
	}))
	defer server.Close()

	t.Setenv("DASH0_API_URL", server.URL)
	t.Setenv("DASH0_AUTH_TOKEN", "test_token")
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	_, _, err := execListCmd(t, "--dataset", "my-dataset")
	require.NoError(t, err)
	assert.Equal(t, "my-dataset", capturedDataset)
}

func TestListDefaultDatasetNotSent(t *testing.T) {
	var capturedDataset string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedDataset = r.URL.Query().Get("dataset")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(labelValuesResponse(sampleNames))
	}))
	defer server.Close()

	t.Setenv("DASH0_API_URL", server.URL)
	t.Setenv("DASH0_AUTH_TOKEN", "test_token")
	t.Setenv("DASH0_DATASET", "default")
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	_, _, err := execListCmd(t)
	require.NoError(t, err)
	assert.Empty(t, capturedDataset, "default dataset should not be sent as param")
}

func TestListInvalidFormat(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setupEnv(t, server.URL)

	_, _, err := execListCmd(t, "-o", "xml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown output format")
}

func TestListSkipHeaderWithJSON(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setupEnv(t, server.URL)

	_, _, err := execListCmd(t, "-o", "json", "--skip-header")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--skip-header is not supported")
}

func TestListWideMetadataEndpoint(t *testing.T) {
	var labelValuesHit, metadataHit bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/label/__name__/values"):
			labelValuesHit = true
			json.NewEncoder(w).Encode(labelValuesResponse(sampleNames))
		case strings.Contains(r.URL.Path, "/metadata"):
			metadataHit = true
			json.NewEncoder(w).Encode(metadataResponse(sampleMetadata))
		}
	}))
	defer server.Close()
	setupEnv(t, server.URL)

	_, _, err := execListCmd(t, "-o", "wide")
	require.NoError(t, err)
	assert.False(t, labelValuesHit, "wide format should not hit label values API")
	assert.True(t, metadataHit, "wide format should hit metadata API")
}

func TestListTableLabelValuesEndpoint(t *testing.T) {
	var labelValuesHit, metadataHit bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/label/__name__/values"):
			labelValuesHit = true
			json.NewEncoder(w).Encode(labelValuesResponse(sampleNames))
		case strings.Contains(r.URL.Path, "/metadata"):
			metadataHit = true
			json.NewEncoder(w).Encode(metadataResponse(sampleMetadata))
		}
	}))
	defer server.Close()
	setupEnv(t, server.URL)

	_, _, err := execListCmd(t)
	require.NoError(t, err)
	assert.True(t, labelValuesHit, "table format should hit label values API")
	assert.False(t, metadataHit, "table format should not hit metadata API")
}

// Test that "No metrics found." is a clean exit (no error)
func TestListEmptyFilterResult(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setupEnv(t, server.URL)

	_, stdout, err := execListCmd(t, "--filter", "zzz_nonexistent_zzz")
	require.NoError(t, err)
	// stdout should be empty since "No metrics found." goes to stderr
	assert.Empty(t, strings.TrimSpace(stdout))
}

func TestListAlias(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()
	setupEnv(t, server.URL)

	root := &cobra.Command{Use: "dash0"}
	root.PersistentFlags().BoolP("experimental", "X", false, "")
	metricsCmd := &cobra.Command{Use: "metrics"}
	metricsCmd.AddCommand(newListCmd())
	root.AddCommand(metricsCmd)

	// Use "ls" alias instead of "list"
	var cmdErr error
	_ = testutil.CaptureStdout(t, func() {
		root.SetArgs([]string{"-X", "metrics", "ls"})
		cmdErr = root.Execute()
	})
	require.NoError(t, cmdErr)
}

func TestListAuthorizationHeader(t *testing.T) {
	var capturedAuthHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuthHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(labelValuesResponse(sampleNames))
	}))
	defer server.Close()

	t.Setenv("DASH0_API_URL", server.URL)
	t.Setenv("DASH0_AUTH_TOKEN", "my_secret_token")
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())

	_, _, err := execListCmd(t)
	require.NoError(t, err)
	assert.Equal(t, "Bearer my_secret_token", capturedAuthHeader)
}

func TestListHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()
	setupEnv(t, server.URL)

	_, _, err := execListCmd(t)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestListMissingConfig(t *testing.T) {
	t.Setenv("DASH0_CONFIG_DIR", t.TempDir())
	// Don't set DASH0_API_URL or DASH0_AUTH_TOKEN
	os.Unsetenv("DASH0_API_URL")
	os.Unsetenv("DASH0_AUTH_TOKEN")

	_, _, err := execListCmd(t)
	require.Error(t, err)
}
