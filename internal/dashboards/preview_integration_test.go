//go:build integration

package dashboards

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const previewDashboardYAML = `kind: Dashboard
metadata:
  name: orders
  dash0Extensions:
    id: orders-overview
spec:
  display:
    name: Orders
  panels: {}
  layouts: []
`

const previewPersesDashboardYAML = `apiVersion: perses.dev/v1alpha1
kind: PersesDashboard
metadata:
  name: checkout
  labels:
    dash0.com/id: checkout-overview
spec:
  display:
    name: Checkout
  panels: {}
  layouts: []
`

const previewViewYAML = `kind: View
metadata:
  name: errors
spec:
  type: logs
  display:
    name: Errors
`

func newExperimentalDashboardsCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewDashboardsCmd())
	return root
}

func writePreviewFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	return dir
}

func ephemeralDashboardResponse(name string) map[string]any {
	return map[string]any{
		"kind":     "EphemeralDashboard",
		"metadata": map[string]any{"name": name},
		"spec":     map[string]any{"display": map[string]any{"name": name}, "ttl": "12h"},
	}
}

func TestPreviewDashboards_UpsertsEveryDashboardAsEphemeralUnderItsOwnID(t *testing.T) {
	testutil.SetupTestEnv(t)
	dir := writePreviewFiles(t, map[string]string{
		"orders.yaml":   previewDashboardYAML,
		"checkout.yaml": previewPersesDashboardYAML,
		"view.yaml":     previewViewYAML,
	})

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDashboards+"/orders-overview", testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       ephemeralDashboardResponse("Orders"),
	})
	server.On(http.MethodGet, apiPathDashboards+"/checkout-overview", testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   fixtureNotFound,
	})
	server.On(http.MethodPut, apiPathDashboards+"/orders-overview", testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       ephemeralDashboardResponse("Orders"),
		Validator:  testutil.RequireHeaders,
	})
	server.On(http.MethodPut, apiPathDashboards+"/checkout-overview", testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       ephemeralDashboardResponse("Checkout"),
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalDashboardsCmd()
	cmd.SetArgs([]string{"-X", "dashboards", "preview", "-f", dir, "--ttl", "12h", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	for _, path := range []string{apiPathDashboards + "/orders-overview", apiPathDashboards + "/checkout-overview"} {
		bodies := server.RequestBodies(t, http.MethodPut, path)
		require.Len(t, bodies, 1, path)
		assert.Equal(t, "EphemeralDashboard", bodies[0]["kind"], path)
		assert.Equal(t, "12h", bodies[0]["spec"].(map[string]any)["ttl"], path)
	}
	for _, req := range server.Requests() {
		assert.NotContains(t, req.Path, "/api/views", "a View is not previewed")
	}
	assert.Contains(t, output, "https://app.dash0.test/goto/dashboards?dashboard_id=orders-overview")
	assert.Contains(t, output, "https://app.dash0.test/goto/dashboards?dashboard_id=checkout-overview")
	assert.Regexp(t, `Orders\s+orders-overview\s+updated`, output)
	assert.Regexp(t, `Checkout\s+checkout-overview\s+created`, output)
}

func TestPreviewDashboards_WithoutIDCreatesWithDefaultTTL(t *testing.T) {
	testutil.SetupTestEnv(t)
	dir := writePreviewFiles(t, map[string]string{"orders.yaml": `kind: Dashboard
metadata:
  name: orders
spec:
  display:
    name: Orders
`})

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	created := ephemeralDashboardResponse("Orders")
	created["metadata"].(map[string]any)["dash0Extensions"] = map[string]any{"id": testDashboardID}
	server.On(http.MethodPost, apiPathDashboards, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       created,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalDashboardsCmd()
	cmd.SetArgs([]string{"-X", "dashboards", "preview", "-f", filepath.Join(dir, "orders.yaml"), "-o", "json", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	bodies := server.RequestBodies(t, http.MethodPost, apiPathDashboards)
	require.Len(t, bodies, 1)
	assert.Equal(t, "EphemeralDashboard", bodies[0]["kind"])
	assert.Equal(t, "1d", bodies[0]["spec"].(map[string]any)["ttl"])

	var results []map[string]string
	require.NoError(t, json.Unmarshal([]byte(output), &results))
	assert.Equal(t, []map[string]string{{
		"name":   "Orders",
		"id":     testDashboardID,
		"action": "created",
		"url":    "https://app.dash0.test/goto/dashboards?dashboard_id=" + testDashboardID,
	}}, results)
}

func TestPreviewDashboards_StopsOnAPermanentDashboardWithTheSameID(t *testing.T) {
	testutil.SetupTestEnv(t)
	dir := writePreviewFiles(t, map[string]string{"orders.yaml": previewDashboardYAML})

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDashboards+"/orders-overview", testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
	})
	server.On(http.MethodPut, apiPathDashboards+"/orders-overview", testutil.MockResponse{
		StatusCode: http.StatusBadRequest,
		Body: map[string]any{"error": map[string]any{
			"code":    400,
			"message": "Bad Request: The dashboard is a Dashboard and cannot change to a EphemeralDashboard. Create a new dashboard instead.",
		}},
	})

	cmd := newExperimentalDashboardsCmd()
	cmd.SetArgs([]string{"-X", "dashboards", "preview", "-f", dir, "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot change to a EphemeralDashboard")
	assert.NotContains(t, output, "https://app.dash0.test")
}

func TestPreviewDashboards_FailsWithoutDashboardsBeforeAnyRequest(t *testing.T) {
	testutil.SetupTestEnv(t)
	dir := writePreviewFiles(t, map[string]string{"view.yaml": previewViewYAML})
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	cmd := newExperimentalDashboardsCmd()
	cmd.SetArgs([]string{"-X", "dashboards", "preview", "-f", dir, "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Dashboard or PersesDashboard found")
	assert.Empty(t, server.Requests())
}

func TestPreviewDashboards_RequiresExperimental(t *testing.T) {
	testutil.SetupTestEnv(t)
	dir := writePreviewFiles(t, map[string]string{"orders.yaml": previewDashboardYAML})
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	cmd := newExperimentalDashboardsCmd()
	cmd.SetArgs([]string{"dashboards", "preview", "-f", dir, "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental")
	assert.Empty(t, server.Requests())
}
