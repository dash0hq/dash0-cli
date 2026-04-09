//go:build integration

package checkrules

import (
	"net/http"
	"os"
	"path/filepath"
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
	assert.Contains(t, cmdErr.Error(), "no check rule ID provided as argument, and the file does not contain an ID")
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
	assert.Contains(t, cmdErr.Error(), "no check rule ID provided as argument, and the file does not contain an ID")
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
