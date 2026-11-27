//go:build integration

package dashboards

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

func TestUpdateDashboard_WithOriginFromFile(t *testing.T) {
	testutil.SetupTestEnv(t)

	yamlFile := filepath.Join(t.TempDir(), "dashboard.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`kind: Dashboard
metadata:
  name: My Dashboard
  labels:
    dash0.com/origin: dash0-cli_my-dashboard
spec:
  display:
    name: My Dashboard
`), 0644))

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		server.On(method, "/api/dashboards/dash0-cli_my-dashboard", testutil.MockResponse{
			StatusCode: http.StatusOK,
			BodyFile:   fixtureGetSuccess,
			Validator:  testutil.RequireHeaders,
		})
	}

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.True(t, slices.ContainsFunc(server.Requests(), func(req testutil.RecordedRequest) bool {
		return req.Method == http.MethodPut && req.Path == "/api/dashboards/dash0-cli_my-dashboard"
	}))
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
	assert.Contains(t, cmdErr.Error(), "no dashboard origin or id given: pass one as an argument")
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

func TestUpdateDashboard_PersesDashboardCRD_IDNotFound_FallsBackToOrigin(t *testing.T) {
	testutil.SetupTestEnv(t)

	missingID := "m1i2s3s4-5678-90ab-cdef-1234567890ab"
	origin := "my-perses-origin"
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/dashboards/"+missingID, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   fixtureNotFound,
		Validator:  testutil.RequireHeaders,
	})
	server.On(http.MethodGet, "/api/dashboards/"+origin, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.On(http.MethodPut, "/api/dashboards/"+origin, testutil.MockResponse{
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
    dash0.com/id: `+missingID+`
    dash0.com/origin: `+origin+`
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
	testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	var putPaths []string
	for _, req := range server.Requests() {
		if req.Method == http.MethodPut {
			putPaths = append(putPaths, req.Path)
		}
	}
	assert.Equal(t, []string{"/api/dashboards/" + origin}, putPaths)
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
	assert.Contains(t, cmdErr.Error(), "no dashboard origin or id given: pass one as an argument")
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

func TestCreateAndUpdateDashboard_Deleted(t *testing.T) {
	file := deletedDashboardYAML
	exportedFile := strings.Replace(file, "metadata:\n", "metadata:\n  annotations:\n    dash0.com/deleted-at: \"2026-10-13T12:50:57Z\"\n", 1)

	restored := `Dashboard "Test Dashboard" restored`
	wouldBeRestored := `Dashboard "Test Dashboard" would be restored`

	tests := []struct {
		name       string
		args       []string
		file       string
		putFixture string
		wantStatus string
		wantErr    string
	}{
		{name: "create", args: []string{"create"}, file: file, putFixture: testutil.FixtureDashboardsImportSuccess, wantStatus: restored},
		{name: "update", args: []string{"update"}, file: file, putFixture: testutil.FixtureDashboardsImportSuccess, wantStatus: restored},
		{name: "update of an exported file", args: []string{"update"}, file: exportedFile, putFixture: testutil.FixtureDashboardsImportSuccess, wantStatus: restored},
		{name: "update dry run", args: []string{"update", "--dry-run"}, file: file, putFixture: testutil.FixtureDashboardsImportSuccess, wantStatus: wouldBeRestored},
		{
			name:       "create that leaves the dashboard deleted",
			args:       []string{"create"},
			file:       file,
			putFixture: testutil.FixtureDashboardsGetDeleted,
			wantErr:    `Dashboard "Test Dashboard" was saved, but the API still shows it as deleted`,
		},
		{
			name:       "update that leaves the dashboard deleted",
			args:       []string{"update"},
			file:       file,
			putFixture: testutil.FixtureDashboardsGetDeleted,
			wantErr:    `Dashboard "Test Dashboard" was saved, but the API still shows it as deleted`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetupTestEnv(t)

			server := testutil.NewMockServer(t, testutil.FixturesDir())
			server.On(http.MethodGet, apiPathDashboards+"/"+testDashboardID, testutil.MockResponse{
				StatusCode: http.StatusOK,
				BodyFile:   testutil.FixtureDashboardsGetDeleted,
				Validator:  testutil.RequireHeaders,
			})
			server.On(http.MethodPut, apiPathDashboards+"/"+testDashboardID, testutil.MockResponse{
				StatusCode: http.StatusOK,
				BodyFile:   tt.putFixture,
				Validator:  testutil.RequireHeaders,
			})

			yamlFile := filepath.Join(t.TempDir(), "asset.yaml")
			require.NoError(t, os.WriteFile(yamlFile, []byte(tt.file), 0644))

			cmd := NewDashboardsCmd()
			cmd.SetArgs(append(tt.args, "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken))

			var cmdErr error
			output := testutil.CaptureStdout(t, func() {
				cmdErr = cmd.Execute()
			})

			if tt.wantErr != "" {
				require.EqualError(t, cmdErr, tt.wantErr)
			} else {
				require.NoError(t, cmdErr)
			}
			if tt.wantStatus == "" {
				assert.NotContains(t, output, "restored")
			} else {
				assert.Contains(t, output, tt.wantStatus)
			}
			puts := server.RequestBodies(t, http.MethodPut, apiPathDashboards+"/"+testDashboardID)
			assert.Equal(t, !slices.Contains(tt.args, "--dry-run"), len(puts) > 0, "PUT sent")
			for _, put := range puts {
				body, err := json.Marshal(put)
				require.NoError(t, err)
				assert.NotContains(t, string(body), "deleted-at", "PUT body")
			}
		})
	}
}

const deletedDashboardYAML = `kind: Dashboard
metadata:
  name: test-dashboard
  dash0Extensions:
    id: ` + testDashboardID + `
spec:
  display:
    name: Test Dashboard
`

func TestUpdateDashboard_EphemeralOverPermanentFailsBeforeTheWrite(t *testing.T) {
	testutil.SetupTestEnv(t)

	yamlFile := filepath.Join(t.TempDir(), "dashboard.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`kind: EphemeralDashboard
metadata:
  name: My Dashboard
spec:
  ttl: 1h
  display:
    name: My Dashboard
`), 0644))

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

	for _, extra := range [][]string{{"--dry-run"}, nil} {
		cmd := NewDashboardsCmd()
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetArgs(append([]string{"update", testDashboardID, "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken}, extra...))
		var err error
		testutil.CaptureStdout(t, func() {
			err = cmd.Execute()
		})
		require.Error(t, err, "args %v", extra)
		assert.Contains(t, err.Error(), "is stored as dashboard")
	}
	assert.Empty(t, server.RequestBodies(t, http.MethodPut, "/api/dashboards/"+testDashboardID))
}

func TestUpdateDashboard_EphemeralShowsItsKindAfterTheWrite(t *testing.T) {
	testutil.SetupTestEnv(t)

	yamlFile := filepath.Join(t.TempDir(), "dashboard.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`kind: EphemeralDashboard
metadata:
  name: My Dashboard
spec:
  ttl: 1h
  display:
    name: My Dashboard
`), 0644))
	stored := map[string]any{
		"kind":     "EphemeralDashboard",
		"metadata": map[string]any{"name": "My Dashboard"},
		"spec":     map[string]any{"ttl": "1h", "display": map[string]any{"name": "My Dashboard"}},
	}

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/dashboards/"+testDashboardID, testutil.MockResponse{StatusCode: http.StatusOK, Body: stored, Validator: testutil.RequireHeaders})
	server.On(http.MethodPut, "/api/dashboards/"+testDashboardID, testutil.MockResponse{StatusCode: http.StatusOK, Body: stored, Validator: testutil.RequireHeaders})

	cmd := NewDashboardsCmd()
	cmd.SetArgs([]string{"update", testDashboardID, "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})
	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, `Ephemeral dashboard "My Dashboard"`)
}
