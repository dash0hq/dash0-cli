//go:build integration

package pods

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiPathQueryRange     = "/api/prometheus/api/v1/query_range"
	fixtureUnauthorized   = "dashboards/error_unauthorized.json"
	testPodsAuthToken     = "auth_test_token"
	testPodsNamespace     = "shop"
	testPodsPod           = "checkout-757b495d7d-cqhb2"
	testPodsDataset       = "staging"
	expectedPanelRequests = 15
	// The sample the handler leaves out of every series, so a gap must parse as null
	gapAt = 3
)

// A query_range handler that answers on the window the command asked for: sample i holds i+0.5, sample gapAt is
// missing, and the logs panel's one series is ERROR. Queries containing failQuery answer 500.
func queryRangeHandler(t *testing.T, failQuery string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := testutil.RequireHeaders(r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		require.NoError(t, r.ParseForm())
		query := r.PostForm.Get("query")
		if failQuery != "" && strings.Contains(query, failQuery) {
			http.Error(w, `{"status":"error","error":"internal"}`, http.StatusInternalServerError)
			return
		}
		start, err := strconv.ParseInt(r.PostForm.Get("start"), 10, 64)
		require.NoError(t, err)
		end, err := strconv.ParseInt(r.PostForm.Get("end"), 10, 64)
		require.NoError(t, err)
		step, err := strconv.ParseInt(r.PostForm.Get("step"), 10, 64)
		require.NoError(t, err)
		var values [][2]any
		for i, ts := 0, start; ts <= end; i, ts = i+1, ts+step {
			if i != gapAt {
				values = append(values, [2]any{ts, strconv.FormatFloat(float64(i)+0.5, 'f', -1, 64)})
			}
		}
		metric := map[string]string{}
		if strings.Contains(query, "otel_log_severity_range") {
			metric["otel_log_severity_range"] = "ERROR"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]any{"resultType": "matrix", "result": []any{map[string]any{"metric": metric, "values": values}}},
		})
	}
}

func newExperimentalPodsCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	k8sCmd := &cobra.Command{Use: "k8s"}
	k8sCmd.AddCommand(NewPodsCmd())
	root.AddCommand(k8sCmd)
	return root
}

type watchJSON struct {
	Namespace  string                `json:"namespace"`
	Pod        string                `json:"pod"`
	Step       string                `json:"step"`
	Timestamps []string              `json:"timestamps"`
	Series     map[string][]*float64 `json:"series"`
	Failed     []string              `json:"failed"`
	Links      map[string]string     `json:"links"`
}

func TestWatch_JSON(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnDefault(queryRangeHandler(t, ""))

	cmd := newExperimentalPodsCmd()
	cmd.SetArgs([]string{"-X", "k8s", "pods", "watch", testPodsPod, "-n", testPodsNamespace, "-o", "json", "--range", "15m",
		"--dataset", testPodsDataset, "--api-url", server.URL, "--auth-token", testPodsAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	var got watchJSON
	require.NoError(t, json.Unmarshal([]byte(output), &got))
	assert.Equal(t, testPodsNamespace, got.Namespace)
	assert.Equal(t, testPodsPod, got.Pod)
	assert.Equal(t, "1m0s", got.Step)
	assert.Len(t, got.Timestamps, 16)
	for _, key := range []string{"cpu", "cpuRequest", "cpuLimit", "mem", "memRequest", "memLimit", "cpuUtil", "memUtil", "requests", "errors", "errorRate", "p50", "p95", "p99", "logs:ERROR"} {
		require.Len(t, got.Series[key], 16, key)
		for i, v := range got.Series[key] {
			if i == gapAt {
				assert.Nil(t, v, "%s[%d]", key, i)
				continue
			}
			require.NotNil(t, v, "%s[%d]", key, i)
			assert.Equal(t, float64(i)+0.5, *v, "%s[%d]", key, i)
		}
	}
	assert.Empty(t, got.Failed)
	assert.Contains(t, got.Links["metrics"], "/goto/metrics/query-builder?")

	requests := server.Requests()
	require.Len(t, requests, expectedPanelRequests)
	form, err := url.ParseQuery(string(requests[0].Body))
	require.NoError(t, err)
	assert.Equal(t, testPodsDataset, form.Get("dataset"))
	assert.Equal(t, "60", form.Get("step"))
	assert.Contains(t, form.Get("query"), `k8s_pod_name="`+testPodsPod+`"`)
}

func TestWatch_Once(t *testing.T) {
	testutil.SetupTestEnv(t)
	t.Setenv("COLUMNS", "150")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnDefault(queryRangeHandler(t, ""))

	cmd := newExperimentalPodsCmd()
	cmd.SetArgs([]string{"-X", "k8s", "pods", "watch", testPodsPod, "-n", testPodsNamespace, "--once",
		"--api-url", server.URL, "--auth-token", testPodsAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, output, testPodsNamespace+"/"+testPodsPod)
	assert.Contains(t, output, "Memory")
	assert.Contains(t, output, "Open metrics")
}

func TestWatch_JSONPartialFailure(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnDefault(queryRangeHandler(t, "histogram_quantile(0.99"))

	cmd := newExperimentalPodsCmd()
	cmd.SetArgs([]string{"-X", "k8s", "pods", "watch", testPodsPod, "-n", testPodsNamespace, "-o", "json", "--range", "15m",
		"--api-url", server.URL, "--auth-token", testPodsAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	var got watchJSON
	require.NoError(t, json.Unmarshal([]byte(output), &got))
	assert.Equal(t, []string{"p99"}, got.Failed)
	assert.NotContains(t, got.Series, "p99")
	assert.Len(t, got.Series["cpu"], 16)
}

func TestWatch_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathQueryRange, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   fixtureUnauthorized,
	})

	cmd := newExperimentalPodsCmd()
	cmd.SetArgs([]string{"-X", "k8s", "pods", "watch", testPodsPod, "-n", testPodsNamespace, "-o", "json",
		"--api-url", server.URL, "--auth-token", testPodsAuthToken})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401 Unauthorized")
	assert.Contains(t, err.Error(), "The provided auth token is not known.")
}

func TestWatch_RequiresExperimentalFlag(t *testing.T) {
	testutil.SetupTestEnv(t)

	cmd := newExperimentalPodsCmd()
	cmd.SetArgs([]string{"k8s", "pods", "watch", testPodsPod, "-n", testPodsNamespace})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental command")
}

func TestWatch_InvalidRange(t *testing.T) {
	testutil.SetupTestEnv(t)

	cmd := newExperimentalPodsCmd()
	cmd.SetArgs([]string{"-X", "k8s", "pods", "watch", testPodsPod, "-n", testPodsNamespace, "--range", "2h"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown range "2h"`)
}

func TestWatch_WorkloadJSON(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnDefault(queryRangeHandler(t, ""))

	cmd := newExperimentalPodsCmd()
	cmd.SetArgs([]string{"-X", "k8s", "pods", "watch", "checkout", "-n", testPodsNamespace, "--kind", "deploy", "--aggregate", "MAX", "--per-pod", "-o", "json",
		"--range", "15m", "--api-url", server.URL, "--auth-token", testPodsAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	var got struct {
		watchJSON
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Aggregate string `json:"aggregate"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &got))
	assert.Equal(t, "deployment", got.Kind)
	assert.Equal(t, "checkout", got.Name)
	assert.Empty(t, got.Pod)
	assert.Equal(t, "max", got.Aggregate)

	requests := server.Requests()
	require.Len(t, requests, expectedPanelRequests+1+len(podPanels))
	var maxed int
	for _, r := range requests {
		form, err := url.ParseQuery(string(r.Body))
		require.NoError(t, err)
		assert.Contains(t, form.Get("query"), `k8s_deployment_name="checkout"`)
		assert.NotContains(t, form.Get("query"), `k8s_pod_name="`)
		if strings.HasPrefix(form.Get("query"), "max(") {
			maxed++
		}
	}
	// Dash0 combines the pods: CPU and memory with their requests, limits and shares of the limits, requests and errors
	assert.Equal(t, 10, maxed)
}

func TestWatch_InvalidKind(t *testing.T) {
	testutil.SetupTestEnv(t)

	cmd := newExperimentalPodsCmd()
	cmd.SetArgs([]string{"-X", "k8s", "pods", "watch", "checkout", "-n", testPodsNamespace, "--kind", "services"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown kind "services"`)
}

func TestWatch_AggNeedsAWorkload(t *testing.T) {
	testutil.SetupTestEnv(t)

	cmd := newExperimentalPodsCmd()
	cmd.SetArgs([]string{"-X", "k8s", "pods", "watch", testPodsPod, "-n", testPodsNamespace, "--aggregate", "max"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--aggregate and --per-pod need a workload")
}
