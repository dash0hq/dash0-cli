//go:build integration

package checkrules

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

const testAuthToken = "auth_test_token"

func TestUpdateCheckRule_IDMismatch(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "rule.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: CheckRule
id: file-id-1111-2222-3333-444444444444
name: test-check-rule
expression: up == 0
`), 0644)
	require.NoError(t, err)

	cmd := NewCheckRulesCmd()
	cmd.SetArgs([]string{"update", "arg-id-aaaa-bbbb-cccc-dddddddddddd", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "does not match")
	assert.Contains(t, cmdErr.Error(), "arg-id-aaaa-bbbb-cccc-dddddddddddd")
	assert.Contains(t, cmdErr.Error(), "file-id-1111-2222-3333-444444444444")
}

func TestUpdateCheckRule_NoIDAnywhere(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "rule.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: CheckRule
name: test-check-rule
expression: up == 0
`), 0644)
	require.NoError(t, err)

	cmd := NewCheckRulesCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "no check rule id given: pass one as an argument")
}

func TestUpdateCheckRule_PrometheusRuleCRD_IDFromFile(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/alerting/check-rules/ca9af402-03aa-4c77-8a81-b4960b5126fd", testutil.MockResponse{
		Validator:  testutil.RequireHeaders,
		StatusCode: http.StatusOK,
		BodyFile:   "checkrules/get_success.json",
	})
	server.On(http.MethodPut, "/api/alerting/check-rules/ca9af402-03aa-4c77-8a81-b4960b5126fd", testutil.MockResponse{
		Validator:  testutil.RequireHeaders,
		StatusCode: http.StatusOK,
		BodyFile:   "checkrules/get_success.json",
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "rule.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: my-prom-rule
  labels:
    dash0.com/id: ca9af402-03aa-4c77-8a81-b4960b5126fd
spec:
  groups:
    - name: Alerting
      interval: 2m0s
      rules:
        - alert: Test Alert
          expr: up == 0
          for: 2m
`), 0644)
	require.NoError(t, err)

	cmd := NewCheckRulesCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.NoError(t, cmdErr)
}

func TestUpdateCheckRule_PrometheusRuleCRD_IDFromArg(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/alerting/check-rules/arg-id-1111-2222-3333-444444444444", testutil.MockResponse{
		Validator:  testutil.RequireHeaders,
		StatusCode: http.StatusOK,
		BodyFile:   "checkrules/get_success.json",
	})
	server.On(http.MethodPut, "/api/alerting/check-rules/arg-id-1111-2222-3333-444444444444", testutil.MockResponse{
		Validator:  testutil.RequireHeaders,
		StatusCode: http.StatusOK,
		BodyFile:   "checkrules/get_success.json",
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "rule.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: my-prom-rule
spec:
  groups:
    - name: Alerting
      rules:
        - alert: Test Alert
          expr: up == 0
`), 0644)
	require.NoError(t, err)

	cmd := NewCheckRulesCmd()
	cmd.SetArgs([]string{"update", "arg-id-1111-2222-3333-444444444444", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.NoError(t, cmdErr)
}

func TestUpdateCheckRule_PrometheusRuleCRD_NoID(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "rule.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: my-prom-rule
spec:
  groups:
    - name: Alerting
      rules:
        - alert: Test Alert
          expr: up == 0
`), 0644)
	require.NoError(t, err)

	cmd := NewCheckRulesCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "no check rule id given: pass one as an argument")
}

func TestUpdateCheckRule_PrometheusRuleCRD_MultipleRules(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "rule.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: my-prom-rule
  labels:
    dash0.com/id: some-id
spec:
  groups:
    - name: Alerting
      rules:
        - alert: Alert One
          expr: up == 0
        - alert: Alert Two
          expr: up == 1
`), 0644)
	require.NoError(t, err)

	cmd := NewCheckRulesCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "contains 2 check rules, but update requires exactly 1")
}

func TestUpdateCheckRule_PrometheusRuleCRD_IDMismatch(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "rule.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: my-prom-rule
  labels:
    dash0.com/id: file-id-1111-2222-3333-444444444444
spec:
  groups:
    - name: Alerting
      rules:
        - alert: Test Alert
          expr: up == 0
`), 0644)
	require.NoError(t, err)

	cmd := NewCheckRulesCmd()
	cmd.SetArgs([]string{"update", "arg-id-aaaa-bbbb-cccc-dddddddddddd", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "does not match")
}

func TestUpdateCheckRule_Deleted(t *testing.T) {
	const ruleID = "47b6ccbe-82ab-47c6-a613-ce0d7f34353e"
	const plainFile = `kind: CheckRule
id: ` + ruleID + `
name: test-check-rule
expression: up == 0
`
	const exportedFile = plainFile + `metadata:
  annotations:
    dash0.com/deleted-at: "2026-10-13T12:50:57Z"
  labels:
    dash0.com/source: dash0-cli
`
	tests := []struct {
		name        string
		file        string
		extraArgs   []string
		putResponse string
		wantOutput  []string
		wantErr     string
		wantDiff    bool
		wantPut     bool
	}{
		{
			name:        "update",
			file:        plainFile,
			putResponse: testutil.FixtureCheckRulesRestoreSuccess,
			wantOutput:  []string{`Check rule "test-check-rule" restored`},
			wantPut:     true,
		},
		{
			name:        "update with a changed field",
			file:        strings.Replace(plainFile, "up == 0", "up == 1", 1),
			putResponse: testutil.FixtureCheckRulesRestoreChanged,
			wantOutput:  []string{`Check rule "test-check-rule" restored`, "-expression: up == 0", "+expression: up == 1"},
			wantDiff:    true,
			wantPut:     true,
		},
		{
			name:        "update that leaves the rule deleted",
			file:        plainFile,
			putResponse: testutil.FixtureCheckRulesGetDeleted,
			wantErr:     `check rule "test-check-rule" is still deleted after the update`,
			wantPut:     true,
		},
		{
			name:       "dry run",
			file:       plainFile,
			extraArgs:  []string{"--dry-run"},
			wantOutput: []string{`-    dash0.com/deleted-at: "2026-10-13T12:50:57Z"`},
			wantDiff:   true,
		},
		{
			name:       "dry run of an exported file",
			file:       exportedFile,
			extraArgs:  []string{"--dry-run"},
			wantOutput: []string{`-    dash0.com/deleted-at: "2026-10-13T12:50:57Z"`},
			wantDiff:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetupTestEnv(t)

			server := testutil.NewMockServer(t, testutil.FixturesDir())
			server.On(http.MethodGet, apiPathCheckRules+"/"+ruleID, testutil.MockResponse{
				Validator:  testutil.RequireHeaders,
				StatusCode: http.StatusOK,
				BodyFile:   testutil.FixtureCheckRulesGetDeleted,
			})
			server.On(http.MethodPut, apiPathCheckRules+"/"+ruleID, testutil.MockResponse{
				Validator:  testutil.RequireHeaders,
				StatusCode: http.StatusOK,
				BodyFile:   tt.putResponse,
			})

			yamlFile := filepath.Join(t.TempDir(), "rule.yaml")
			require.NoError(t, os.WriteFile(yamlFile, []byte(tt.file), 0644))

			cmd := NewCheckRulesCmd()
			cmd.SetArgs(append([]string{"update", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken}, tt.extraArgs...))

			var cmdErr error
			output := testutil.CaptureStdout(t, func() {
				cmdErr = cmd.Execute()
			})

			if tt.wantErr != "" {
				require.EqualError(t, cmdErr, tt.wantErr)
			} else {
				require.NoError(t, cmdErr)
			}
			for _, want := range tt.wantOutput {
				assert.Contains(t, output, want)
			}
			assert.Equal(t, tt.wantDiff, strings.Contains(output, "--- Check rule (before)"), "diff in output:\n%s", output)
			assert.Equal(t, tt.wantPut, countRequests(server.Requests(), http.MethodPut) > 0, "PUT sent")
		})
	}
}

func countRequests(requests []testutil.RecordedRequest, method string) int {
	count := 0
	for _, request := range requests {
		if request.Method == method {
			count++
		}
	}
	return count
}
