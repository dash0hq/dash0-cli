//go:build integration

package views

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAuthToken = "auth_test_token"
	testViewID    = "test-view-id"
)

func TestUpdateView_IDMismatch(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "view.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: View
metadata:
  name: Error Logs
  labels:
    dash0.com/id: file-id-1111-2222-3333-444444444444
spec:
  query: "severity >= ERROR"
`), 0644)
	require.NoError(t, err)

	cmd := NewViewsCmd()
	cmd.SetArgs([]string{"update", "arg-id-aaaa-bbbb-cccc-dddddddddddd", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "does not match")
	assert.Contains(t, cmdErr.Error(), "arg-id-aaaa-bbbb-cccc-dddddddddddd")
	assert.Contains(t, cmdErr.Error(), "file-id-1111-2222-3333-444444444444")
}

func TestUpdateView_NoIDAnywhere(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "view.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: View
metadata:
  name: Error Logs
spec:
  query: "severity >= ERROR"
`), 0644)
	require.NoError(t, err)

	cmd := NewViewsCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "no view id given: pass one as an argument")
}

func TestCreateAndUpdateView_Deleted(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantRestored bool
	}{
		{name: "create", args: []string{"create"}, wantRestored: true},
		{name: "update", args: []string{"update"}, wantRestored: true},
		{name: "update dry run", args: []string{"update", "--dry-run"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetupTestEnv(t)

			server := testutil.NewMockServer(t, testutil.FixturesDir())
			server.On(http.MethodGet, apiPathViews+"/"+testViewID, testutil.MockResponse{
				StatusCode: http.StatusOK,
				BodyFile:   testutil.FixtureViewsGetDeleted,
				Validator:  testutil.RequireHeaders,
			})
			server.On(http.MethodPut, apiPathViews+"/"+testViewID, testutil.MockResponse{
				StatusCode: http.StatusOK,
				BodyFile:   testutil.FixtureViewsImportSuccess,
				Validator:  testutil.RequireHeaders,
			})

			yamlFile := filepath.Join(t.TempDir(), "asset.yaml")
			require.NoError(t, os.WriteFile(yamlFile, []byte(deletedViewYAML), 0644))

			cmd := NewViewsCmd()
			cmd.SetArgs(append(tt.args, "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken))

			var cmdErr error
			output := testutil.CaptureStdout(t, func() {
				cmdErr = cmd.Execute()
			})

			require.NoError(t, cmdErr)
			assert.Equal(t, tt.wantRestored, strings.Contains(output, `View "Test View" restored`), "restored in output:\n%s", output)
			puts := server.RequestBodies(t, http.MethodPut, apiPathViews+"/"+testViewID)
			assert.Equal(t, tt.wantRestored, len(puts) > 0, "PUT sent")
		})
	}
}

const deletedViewYAML = `kind: Dash0View
metadata:
  name: test-view
  labels:
    dash0.com/id: ` + testViewID + `
spec:
  display:
    name: Test View
  type: logs
  filter: []
  table:
    columns: []
    sort: []
`
