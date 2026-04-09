//go:build integration

package dashboards

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateDashboard_WithIDArg(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "dashboard.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: Dashboard
metadata:
  name: My Dashboard
spec:
  display:
    name: My Dashboard
`), 0644)
	require.NoError(t, err)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/dashboards/"+testDashboardID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.On(http.MethodPut, "/api/dashboards/"+testDashboardID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"update", testDashboardID, "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	// Output is a diff; since GET and PUT return the same fixture, expect "no changes"
	assert.Contains(t, output, "no changes")
}

func TestUpdateDashboard_WithIDFromFile(t *testing.T) {
	testutil.SetupTestEnv(t)

	dashboardID := "d1e2f3a4-5678-90ab-cdef-1234567890ab"
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "dashboard.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: Dashboard
metadata:
  name: My Dashboard
  dash0Extensions:
    id: `+dashboardID+`
spec:
  display:
    name: My Dashboard
`), 0644)
	require.NoError(t, err)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/dashboards/"+dashboardID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.On(http.MethodPut, "/api/dashboards/"+dashboardID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	// Output is a diff; since GET and PUT return the same fixture, expect "no changes"
	assert.Contains(t, output, "no changes")
}

func TestUpdateDashboard_IDMismatch(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "dashboard.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: Dashboard
metadata:
  name: My Dashboard
  dash0Extensions:
    id: file-id-1111-2222-3333-444444444444
spec:
  display:
    name: My Dashboard
`), 0644)
	require.NoError(t, err)

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"update", "arg-id-aaaa-bbbb-cccc-dddddddddddd", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "does not match")
	assert.Contains(t, cmdErr.Error(), "arg-id-aaaa-bbbb-cccc-dddddddddddd")
	assert.Contains(t, cmdErr.Error(), "file-id-1111-2222-3333-444444444444")
}

func TestUpdateDashboard_NoIDAnywhere(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "dashboard.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: Dashboard
metadata:
  name: My Dashboard
spec:
  display:
    name: My Dashboard
`), 0644)
	require.NoError(t, err)

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "no dashboard ID provided as argument, and the file does not contain an ID")
}

func TestUpdateDashboard_PersesDashboardCRD_IDFromFile(t *testing.T) {
	testutil.SetupTestEnv(t)

	persesID := "p1e2r3s4-5678-90ab-cdef-1234567890ab"
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/dashboards/"+persesID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.On(http.MethodPut, "/api/dashboards/"+persesID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "dashboard.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: perses.dev/v1alpha1
kind: PersesDashboard
metadata:
  name: my-perses-dashboard
  labels:
    dash0.com/id: `+persesID+`
spec:
  display:
    name: My Perses Dashboard
  duration: 5m
  panels: {}
`), 0644)
	require.NoError(t, err)

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "no changes")
}

func TestUpdateDashboard_PersesDashboardCRD_IDFromArg(t *testing.T) {
	testutil.SetupTestEnv(t)

	argID := "a1r2g3i4-5678-90ab-cdef-1234567890ab"
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/dashboards/"+argID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.On(http.MethodPut, "/api/dashboards/"+argID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "dashboard.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: perses.dev/v1alpha1
kind: PersesDashboard
metadata:
  name: my-perses-dashboard
spec:
  display:
    name: My Perses Dashboard
  duration: 5m
  panels: {}
`), 0644)
	require.NoError(t, err)

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"update", argID, "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "no changes")
}

func TestUpdateDashboard_PersesDashboardCRD_NoID(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "dashboard.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: perses.dev/v1alpha1
kind: PersesDashboard
metadata:
  name: my-perses-dashboard
spec:
  display:
    name: My Perses Dashboard
`), 0644)
	require.NoError(t, err)

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "no dashboard ID provided as argument, and the file does not contain an ID")
}

func TestUpdateDashboard_PersesDashboardCRD_IDMismatch(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "dashboard.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: perses.dev/v1alpha1
kind: PersesDashboard
metadata:
  name: my-perses-dashboard
  labels:
    dash0.com/id: file-id-1111-2222-3333-444444444444
spec:
  display:
    name: My Perses Dashboard
`), 0644)
	require.NoError(t, err)

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"update", "arg-id-aaaa-bbbb-cccc-dddddddddddd", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "does not match")
}
