//go:build integration

package pods

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiPathQueryRange     = "/api/prometheus/api/v1/query_range"
	fixtureQueryRange     = "pods/query_range_success.json"
	fixtureUnauthorized   = "dashboards/error_unauthorized.json"
	testPodsAuthToken     = "auth_test_token"
	testPodsNamespace     = "dash0-shared"
	testPodsPod           = "api-757b495d7d-cqhb2"
	testPodsDataset       = "staging"
	expectedPanelRequests = 12
)

func newExperimentalPodsCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewPodsCmd())
	return root
}

type watchJSON struct {
	Namespace  string                `json:"namespace"`
	Pod        string                `json:"pod"`
	Step       string                `json:"step"`
	Timestamps []string              `json:"timestamps"`
	Series     map[string][]*float64 `json:"series"`
	Links      map[string]string     `json:"links"`
}

func TestWatch_JSON(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathQueryRange, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQueryRange,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalPodsCmd()
	cmd.SetArgs([]string{"-X", "pods", "watch", testPodsPod, "-n", testPodsNamespace, "-o", "json", "--range", "15m",
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
	for _, key := range []string{"cpu", "cpuRequest", "cpuLimit", "mem", "memRequest", "memLimit", "requests", "errors", "p50", "p95", "p99"} {
		assert.Contains(t, got.Series, key)
	}
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
	server.On(http.MethodPost, apiPathQueryRange, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQueryRange,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalPodsCmd()
	cmd.SetArgs([]string{"-X", "pods", "watch", testPodsPod, "-n", testPodsNamespace, "--once",
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

func TestWatch_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathQueryRange, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   fixtureUnauthorized,
	})

	cmd := newExperimentalPodsCmd()
	cmd.SetArgs([]string{"-X", "pods", "watch", testPodsPod, "-n", testPodsNamespace, "-o", "json",
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
	cmd.SetArgs([]string{"pods", "watch", testPodsPod, "-n", testPodsNamespace})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental command")
}

func TestWatch_InvalidRange(t *testing.T) {
	testutil.SetupTestEnv(t)

	cmd := newExperimentalPodsCmd()
	cmd.SetArgs([]string{"-X", "pods", "watch", testPodsPod, "-n", testPodsNamespace, "--range", "2h"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown range "2h"`)
}
