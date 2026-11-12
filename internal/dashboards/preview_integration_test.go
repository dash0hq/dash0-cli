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

// UUIDv5 of each dashboard id in previewIDNamespace, computed outside the CLI.
const (
	ordersPreviewID         = "dd67c185-957d-5d5b-afa5-f1be006171e3"
	checkoutPreviewID       = "a2b44bf1-36e5-5b71-a673-3296084b8222"
	ordersPrefixedPreviewID = "711a89a5-0d38-52bd-9707-31779be82673" // "pr-376-orders-overview"
)

// The id the API assigned to the permanent dashboard whose own id,
// "orders-overview", is not a UUID and is therefore kept as its origin.
const ordersServerID = "5f0c8a52-1b9e-4c3d-8f7a-2e6d9b4c1a03"

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

func TestPreviewDashboards_UpsertsEveryDashboardAsEphemeralUnderADerivedID(t *testing.T) {
	testutil.SetupTestEnv(t)
	dir := writePreviewFiles(t, map[string]string{
		"orders.yaml":   previewDashboardYAML,
		"checkout.yaml": previewPersesDashboardYAML,
		"view.yaml":     previewViewYAML,
	})

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDashboards+"/"+ordersPreviewID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       ephemeralDashboardResponse("Orders"),
	})
	server.On(http.MethodGet, apiPathDashboards+"/"+checkoutPreviewID, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   fixtureNotFound,
	})
	server.On(http.MethodPut, apiPathDashboards+"/"+ordersPreviewID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       ephemeralDashboardResponse("Orders"),
		Validator:  testutil.RequireHeaders,
	})
	server.On(http.MethodPut, apiPathDashboards+"/"+checkoutPreviewID, testutil.MockResponse{
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
	for _, path := range []string{apiPathDashboards + "/" + ordersPreviewID, apiPathDashboards + "/" + checkoutPreviewID} {
		bodies := server.RequestBodies(t, http.MethodPut, path)
		require.Len(t, bodies, 1, path)
		assert.Equal(t, "EphemeralDashboard", bodies[0]["kind"], path)
		assert.Equal(t, "12h", bodies[0]["spec"].(map[string]any)["ttl"], path)
	}
	for _, req := range server.Requests() {
		assert.NotContains(t, req.Path, "/api/views", "a View is not previewed")
	}
	assert.Contains(t, output, "https://app.dash0.test/goto/dashboards?dashboard_id="+ordersPreviewID)
	assert.Contains(t, output, "https://app.dash0.test/goto/dashboards?dashboard_id="+checkoutPreviewID)
	assert.Regexp(t, `Orders\s+`+ordersPreviewID+`\s+updated`, output)
	assert.Regexp(t, `Checkout\s+`+checkoutPreviewID+`\s+created`, output)
}

func TestPreviewDashboards_LinksEachPreviewToItsDashboardByServerID(t *testing.T) {
	testutil.SetupTestEnv(t)
	dir := writePreviewFiles(t, map[string]string{
		"orders.yaml":   previewDashboardYAML,
		"checkout.yaml": previewPersesDashboardYAML,
	})

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	for _, id := range []string{ordersPreviewID, checkoutPreviewID} {
		server.On(http.MethodGet, apiPathDashboards+"/"+id, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: fixtureNotFound})
		server.On(http.MethodPut, apiPathDashboards+"/"+id, testutil.MockResponse{StatusCode: http.StatusOK, Body: ephemeralDashboardResponse(id)})
	}
	server.On(http.MethodGet, apiPathDashboards, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body: []map[string]any{
			{"id": ordersServerID, "origin": "orders-overview", "kind": "Dashboard", "dataset": "default", "tags": []string{}},
			{"id": ordersPreviewID, "kind": "EphemeralDashboard", "dataset": "default", "tags": []string{}},
			{"id": checkoutPreviewID, "kind": "EphemeralDashboard", "dataset": "default", "tags": []string{}},
		},
		Validator: testutil.RequireHeaders,
	})

	cmd := newExperimentalDashboardsCmd()
	cmd.SetArgs([]string{"-X", "dashboards", "preview", "-f", dir, "-o", "json", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	var results []map[string]string
	require.NoError(t, json.Unmarshal([]byte(output), &results))
	currentByID := map[string]string{}
	for _, result := range results {
		currentByID[result["id"]] = result["current"]
	}
	assert.Equal(t, map[string]string{
		ordersPreviewID:   "https://app.dash0.test/goto/dashboards?dashboard_id=" + ordersServerID,
		checkoutPreviewID: "",
	}, currentByID, "only Orders exists as a permanent dashboard, and its link uses the server id, not its origin")
}

func TestPreviewDashboards_PrefixSeparatesPreviewsAndPrefixesTheirNames(t *testing.T) {
	testutil.SetupTestEnv(t)
	dir := writePreviewFiles(t, map[string]string{"orders.yaml": previewDashboardYAML})

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDashboards+"/"+ordersPrefixedPreviewID, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: fixtureNotFound})
	server.On(http.MethodPut, apiPathDashboards+"/"+ordersPrefixedPreviewID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       ephemeralDashboardResponse("PR 376-Orders"),
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalDashboardsCmd()
	cmd.SetArgs([]string{"-X", "dashboards", "preview", "-f", dir, "--prefix", "PR 376", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	bodies := server.RequestBodies(t, http.MethodPut, apiPathDashboards+"/"+ordersPrefixedPreviewID)
	require.Len(t, bodies, 1, "the prefix is part of the preview id, so it does not replace the unprefixed preview "+ordersPreviewID)
	assert.Equal(t, "pr-376-orders", bodies[0]["metadata"].(map[string]any)["name"])
	assert.Equal(t, "PR 376-Orders", bodies[0]["spec"].(map[string]any)["display"].(map[string]any)["name"])
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

func TestPreviewDashboards_LeavesThePermanentDashboardWithTheSameIDAlone(t *testing.T) {
	testutil.SetupTestEnv(t)
	dir := writePreviewFiles(t, map[string]string{"orders.yaml": previewDashboardYAML})

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDashboards+"/orders-overview", testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
	})
	server.On(http.MethodGet, apiPathDashboards+"/"+ordersPreviewID, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   fixtureNotFound,
	})
	server.On(http.MethodPut, apiPathDashboards+"/"+ordersPreviewID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       ephemeralDashboardResponse("Orders"),
	})

	cmd := newExperimentalDashboardsCmd()
	cmd.SetArgs([]string{"-X", "dashboards", "preview", "-f", dir, "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	require.Len(t, server.RequestBodies(t, http.MethodPut, apiPathDashboards+"/"+ordersPreviewID), 1)
	for _, req := range server.Requests() {
		assert.NotEqual(t, apiPathDashboards+"/orders-overview", req.Path, "the permanent dashboard is not read or written")
	}
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
