//go:build integration

package dashboards

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiPathDashboards    = "/api/dashboards"
	fixtureListSuccess   = "dashboards/list_success.json"
	fixtureListEmpty     = "dashboards/list_empty.json"
	fixtureGetSuccess    = "dashboards/get_success.json"
	fixtureNotFound      = "dashboards/error_not_found.json"
	fixtureUnauthorized  = "dashboards/error_unauthorized.json"
	testDashboardID      = "a1b2c3d4-5678-90ab-cdef-1234567890ab"
	testAuthToken        = "auth_test_token"
)

var dashboardIDPattern = regexp.MustCompile(`^/api/dashboards/[a-f0-9-]+$`)

func TestListDashboards_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDashboards, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	// Mock the GetDashboard calls for fetching display names
	server.OnPattern(http.MethodGet, dashboardIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// Should contain the table header
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "ID")
	// Should contain dashboard data - the display name comes from the get call
	assert.Contains(t, output, "New dashboard")
}

func TestListDashboards_Empty(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDashboards, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListEmpty,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "No dashboards found")
}

func TestListDashboards_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDashboards, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   fixtureUnauthorized,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", "auth_invalid_token"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestListDashboards_WideFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDashboards, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodGet, dashboardIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "wide"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// Wide format should include additional columns
	assert.Contains(t, output, "DATASET")
	assert.Contains(t, output, "ORIGIN")
	assert.Contains(t, output, "URL")
}

func TestListDashboards_CSVFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDashboards, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodGet, dashboardIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "csv"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	// Should have a header row plus data rows
	require.GreaterOrEqual(t, len(lines), 2)
	// Header should contain all wide columns in lowercase
	assert.Equal(t, "name,id,dataset,origin,url", lines[0])
	// Data rows should contain comma-separated values
	assert.Contains(t, lines[1], "New dashboard")
}

func TestListDashboards_CSVFormat_SkipHeader(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDashboards, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodGet, dashboardIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "csv", "--skip-header"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// Should not contain the header row
	assert.NotContains(t, output, "name,id,dataset,origin,url")
	// Should still contain data
	assert.Contains(t, output, "New dashboard")
}

func TestListDashboards_JSONFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDashboards, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// JSON format should be valid JSON with dashboard data
	assert.Contains(t, output, `"id"`)
	assert.Contains(t, output, `"0c3893ac-3d26-11ef-943e-eedf0419e619"`)
}

func TestGetDashboard_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDashboards+"/"+testDashboardID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"get", testDashboardID, "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// Should contain dashboard details
	assert.Contains(t, output, "Kind: Dashboard")
	assert.Contains(t, output, "Name: New dashboard")
	assert.Contains(t, output, "Dataset: default")
}

func TestGetDashboard_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	nonexistentID := "nonexistent-id"
	server.On(http.MethodGet, apiPathDashboards+"/"+nonexistentID, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   fixtureNotFound,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"get", nonexistentID, "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetDashboard_YAMLFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDashboards+"/"+testDashboardID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"get", testDashboardID, "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// Should be valid YAML
	assert.Contains(t, output, "kind: Dashboard")
	assert.Contains(t, output, "metadata:")
}

func TestGetDashboard_JSONFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDashboards+"/"+testDashboardID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"get", testDashboardID, "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// Should be valid JSON
	assert.Contains(t, output, `"kind": "Dashboard"`)
	assert.Contains(t, output, `"metadata"`)
}

func TestListDashboards_RequestRecording(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDashboards, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListEmpty,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "--dataset", "my-dataset"})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	// Verify the recorded request
	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Equal(t, http.MethodGet, req.Method)
	assert.Equal(t, apiPathDashboards, req.Path)
	assert.Contains(t, req.Query, "dataset=my-dataset")
	assert.True(t, strings.HasPrefix(req.Header.Get("Authorization"), "Bearer "))
}
