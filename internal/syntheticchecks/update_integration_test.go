//go:build integration

package syntheticchecks

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
	file := deletedSyntheticCheckYAML
	exportedFile := strings.Replace(file, "metadata:\n", "metadata:\n  annotations:\n    dash0.com/deleted-at: \"2026-10-13T12:50:57Z\"\n", 1)

	restored := `Synthetic check "Test Synthetic Check" restored`
	wouldBeRestored := `Synthetic check "Test Synthetic Check" would be restored`

	tests := []struct {
		name       string
		args       []string
		file       string
		putFixture string
		wantStatus string
		wantErr    string
	}{
		{name: "create", args: []string{"create"}, file: file, putFixture: testutil.FixtureSyntheticChecksImportSuccess, wantStatus: restored},
		{name: "update", args: []string{"update"}, file: file, putFixture: testutil.FixtureSyntheticChecksImportSuccess, wantStatus: restored},
		{name: "update of an exported file", args: []string{"update"}, file: exportedFile, putFixture: testutil.FixtureSyntheticChecksImportSuccess, wantStatus: restored},
		{name: "update dry run", args: []string{"update", "--dry-run"}, file: file, putFixture: testutil.FixtureSyntheticChecksImportSuccess, wantStatus: wouldBeRestored},
		{
			name:       "create that leaves the synthetic check deleted",
			args:       []string{"create"},
			file:       file,
			putFixture: testutil.FixtureSyntheticChecksGetDeleted,
			wantErr:    `Synthetic check "Test Synthetic Check" was saved, but the API still shows it as deleted`,
		},
		{
			name:       "update that leaves the synthetic check deleted",
			args:       []string{"update"},
			file:       file,
			putFixture: testutil.FixtureSyntheticChecksGetDeleted,
			wantErr:    `Synthetic check "Test Synthetic Check" was saved, but the API still shows it as deleted`,
		},
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
				BodyFile:   tt.putFixture,
				Validator:  testutil.RequireHeaders,
			})

			yamlFile := filepath.Join(t.TempDir(), "asset.yaml")
			require.NoError(t, os.WriteFile(yamlFile, []byte(tt.file), 0644))

			cmd := NewSyntheticChecksCmd()
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
			puts := server.RequestBodies(t, http.MethodPut, apiPathSyntheticChecks+"/"+testSyntheticCheckID)
			assert.Equal(t, !slices.Contains(tt.args, "--dry-run"), len(puts) > 0, "PUT sent")
			for _, put := range puts {
				body, err := json.Marshal(put)
				require.NoError(t, err)
				assert.NotContains(t, string(body), "deleted-at", "PUT body")
			}
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
