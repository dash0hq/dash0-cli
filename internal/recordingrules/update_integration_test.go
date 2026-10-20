//go:build integration

package recordingrules

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
	testAuthToken       = "auth_test_token"
	testRecordingRuleID = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
)

func TestUpdateRecordingRule_IDMismatch(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "rule.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: CPU Usage Average
  labels:
    dash0.com/id: file-id-1111-2222-3333-444444444444
spec:
  groups:
    - name: cpu-averages
      rules:
        - record: instance:cpu_usage:avg5m
          expr: avg without(cpu) (rate(node_cpu_seconds_total{mode!="idle"}[5m]))
`), 0644)
	require.NoError(t, err)

	cmd := NewRecordingRulesCmd()
	cmd.SetArgs([]string{"update", "arg-id-aaaa-bbbb-cccc-dddddddddddd", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "does not match")
	assert.Contains(t, cmdErr.Error(), "arg-id-aaaa-bbbb-cccc-dddddddddddd")
	assert.Contains(t, cmdErr.Error(), "file-id-1111-2222-3333-444444444444")
}

func TestUpdateRecordingRule_NoIDAnywhere(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "rule.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: CPU Usage Average
spec:
  groups:
    - name: cpu-averages
      rules:
        - record: instance:cpu_usage:avg5m
          expr: avg without(cpu) (rate(node_cpu_seconds_total{mode!="idle"}[5m]))
`), 0644)
	require.NoError(t, err)

	cmd := NewRecordingRulesCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "no recording rule id given: pass one as an argument")
}

func TestCreateAndUpdateRecordingRule_Deleted(t *testing.T) {
	file := deletedRecordingRuleYAML
	exportedFile := strings.Replace(file, "metadata:\n", "metadata:\n  annotations:\n    dash0.com/deleted-at: \"2026-10-13T12:50:57Z\"\n", 1)

	tests := []struct {
		name         string
		args         []string
		file         string
		putFixture   string
		wantRestored bool
		wantErr      string
	}{
		{name: "create", args: []string{"create"}, file: file, putFixture: testutil.FixtureRecordingRulesCreateSuccess, wantRestored: true},
		{name: "update", args: []string{"update"}, file: file, putFixture: testutil.FixtureRecordingRulesCreateSuccess, wantRestored: true},
		{name: "update of an exported file", args: []string{"update"}, file: exportedFile, putFixture: testutil.FixtureRecordingRulesCreateSuccess, wantRestored: true},
		{name: "update dry run", args: []string{"update", "--dry-run"}, file: file, putFixture: testutil.FixtureRecordingRulesCreateSuccess},
		{
			name:       "create that leaves the recording rule deleted",
			args:       []string{"create"},
			file:       file,
			putFixture: testutil.FixtureRecordingRulesGetDeleted,
			wantErr:    `recording rule "CPU Usage Average" is still deleted after the update`,
		},
		{
			name:       "update that leaves the recording rule deleted",
			args:       []string{"update"},
			file:       file,
			putFixture: testutil.FixtureRecordingRulesGetDeleted,
			wantErr:    `recording rule "CPU Usage Average" is still deleted after the update`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetupTestEnv(t)

			server := testutil.NewMockServer(t, testutil.FixturesDir())
			server.On(http.MethodGet, apiPathRecordingRules+"/"+testRecordingRuleID, testutil.MockResponse{
				StatusCode: http.StatusOK,
				BodyFile:   testutil.FixtureRecordingRulesGetDeleted,
				Validator:  testutil.RequireHeaders,
			})
			server.On(http.MethodPut, apiPathRecordingRules+"/"+testRecordingRuleID, testutil.MockResponse{
				StatusCode: http.StatusOK,
				BodyFile:   tt.putFixture,
				Validator:  testutil.RequireHeaders,
			})

			yamlFile := filepath.Join(t.TempDir(), "asset.yaml")
			require.NoError(t, os.WriteFile(yamlFile, []byte(tt.file), 0644))

			cmd := NewRecordingRulesCmd()
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
			assert.Equal(t, tt.wantRestored, strings.Contains(output, `Recording rule "CPU Usage Average" restored`), "restored in output:\n%s", output)
			puts := server.RequestBodies(t, http.MethodPut, apiPathRecordingRules+"/"+testRecordingRuleID)
			assert.Equal(t, !slices.Contains(tt.args, "--dry-run"), len(puts) > 0, "PUT sent")
			for _, put := range puts {
				body, err := json.Marshal(put)
				require.NoError(t, err)
				assert.NotContains(t, string(body), "deleted-at", "PUT body")
			}
			for _, put := range puts {
				labels := put["metadata"].(map[string]any)["labels"].(map[string]any)
				assert.Equal(t, "3", labels["dash0.com/version"], "the PUT must carry the version of the deleted rule")
			}
		})
	}
}

const deletedRecordingRuleYAML = `apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: CPU Usage Average
  labels:
    dash0.com/id: ` + testRecordingRuleID + `
spec:
  groups:
    - name: cpu-averages
      interval: 1m
      rules:
        - record: instance:cpu_usage:avg5m
          expr: avg without(cpu) (rate(node_cpu_seconds_total{mode!="idle"}[5m]))
`
