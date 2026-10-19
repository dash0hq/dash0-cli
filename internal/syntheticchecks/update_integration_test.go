//go:build integration

package syntheticchecks

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
	testAuthToken        = "auth_test_token"
	testSyntheticCheckID = "test-synthetic-check-id"
)

func TestUpdateSyntheticCheck_IDMismatch(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "check.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: SyntheticCheck
metadata:
  name: API Health Check
  labels:
    dash0.com/id: file-id-1111-2222-3333-444444444444
spec:
  url: https://api.example.com/health
  interval: 60s
`), 0644)
	require.NoError(t, err)

	cmd := NewSyntheticChecksCmd()
	cmd.SetArgs([]string{"update", "arg-id-aaaa-bbbb-cccc-dddddddddddd", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "does not match")
	assert.Contains(t, cmdErr.Error(), "arg-id-aaaa-bbbb-cccc-dddddddddddd")
	assert.Contains(t, cmdErr.Error(), "file-id-1111-2222-3333-444444444444")
}

func TestUpdateSyntheticCheck_NoIDAnywhere(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "check.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: SyntheticCheck
metadata:
  name: API Health Check
spec:
  url: https://api.example.com/health
  interval: 60s
`), 0644)
	require.NoError(t, err)

	cmd := NewSyntheticChecksCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "no synthetic check id given: pass one as an argument")
}

func TestCreateAndUpdateSyntheticCheck_Deleted(t *testing.T) {
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
			server.On(http.MethodGet, apiPathSyntheticChecks+"/"+testSyntheticCheckID, testutil.MockResponse{
				StatusCode: http.StatusOK,
				BodyFile:   testutil.FixtureSyntheticChecksGetDeleted,
				Validator:  testutil.RequireHeaders,
			})
			server.On(http.MethodPut, apiPathSyntheticChecks+"/"+testSyntheticCheckID, testutil.MockResponse{
				StatusCode: http.StatusOK,
				BodyFile:   testutil.FixtureSyntheticChecksImportSuccess,
				Validator:  testutil.RequireHeaders,
			})

			yamlFile := filepath.Join(t.TempDir(), "asset.yaml")
			require.NoError(t, os.WriteFile(yamlFile, []byte(deletedSyntheticCheckYAML), 0644))

			cmd := NewSyntheticChecksCmd()
			cmd.SetArgs(append(tt.args, "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken))

			var cmdErr error
			output := testutil.CaptureStdout(t, func() {
				cmdErr = cmd.Execute()
			})

			require.NoError(t, cmdErr)
			assert.Equal(t, tt.wantRestored, strings.Contains(output, `Synthetic check "Test Synthetic Check" restored`), "restored in output:\n%s", output)
			puts := server.RequestBodies(t, http.MethodPut, apiPathSyntheticChecks+"/"+testSyntheticCheckID)
			assert.Equal(t, tt.wantRestored, len(puts) > 0, "PUT sent")
		})
	}
}

const deletedSyntheticCheckYAML = `kind: Dash0SyntheticCheck
metadata:
  name: test-synthetic-check
  labels:
    dash0.com/id: ` + testSyntheticCheckID + `
spec:
  display:
    name: Test Synthetic Check
  plugin:
    kind: http
    spec:
      request:
        method: get
        url: https://example.com/health
`
