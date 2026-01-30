//go:build integration

package apply

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAuthToken = "auth_test_token"

	// Import API paths
	apiPathImportDashboard      = "/api/import/dashboard"
	apiPathImportCheckRule      = "/api/import/check-rule"
	apiPathImportView           = "/api/import/view"
	apiPathImportSyntheticCheck = "/api/import/synthetic-check"
)

var (
	dashboardIDPattern      = regexp.MustCompile(`^/api/dashboards/[^/]+$`)
	checkRuleIDPattern      = regexp.MustCompile(`^/api/alerting/check-rules/[^/]+$`)
	viewIDPattern           = regexp.MustCompile(`^/api/views/[^/]+$`)
	syntheticCheckIDPattern = regexp.MustCompile(`^/api/synthetic-checks/[^/]+$`)
)

func TestApply_CheckRule_Created(t *testing.T) {
	testutil.SetupTestEnv(t)

	// Create a temp file with a check rule
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "checkrule.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: CheckRule
name: test-check-rule
expression: up == 0
`), 0644)
	require.NoError(t, err)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	// No existing check rule (return 404 on get)
	server.OnPattern(http.MethodGet, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureCheckRulesNotFound,
	})
	// Import succeeds
	server.WithCheckRuleImport(testutil.FixtureCheckRulesImportSuccess)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "CheckRule")
	assert.Contains(t, output, "created")
}

func TestApply_CheckRule_Updated(t *testing.T) {
	testutil.SetupTestEnv(t)

	// Create a temp file with a check rule that has an ID
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "checkrule.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: CheckRule
id: 47b6ccbe-82ab-47c6-a613-ce0d7f34353e
name: test-check-rule
expression: up == 0
`), 0644)
	require.NoError(t, err)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	// Check rule exists (return 200 on get)
	server.On(http.MethodGet, "/api/alerting/check-rules/47b6ccbe-82ab-47c6-a613-ce0d7f34353e", testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureCheckRulesImportSuccess,
		Validator:  testutil.RequireAuthHeader,
	})
	// Import succeeds
	server.WithCheckRuleImport(testutil.FixtureCheckRulesImportSuccess)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "CheckRule")
	assert.Contains(t, output, "updated")
}

func TestApply_Dashboard_Created(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "dashboard.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: Dashboard
metadata:
  name: test-dashboard
spec:
  display:
    name: Test Dashboard
  layouts:
    - kind: Grid
      spec:
        items: []
  panels: {}
`), 0644)
	require.NoError(t, err)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	// No existing dashboard (return 404 on get)
	server.OnPattern(http.MethodGet, dashboardIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureDashboardsNotFound,
	})
	// Import succeeds
	server.WithDashboardImport(testutil.FixtureDashboardsImportSuccess)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "Dashboard")
	assert.Contains(t, output, "created")
}

func TestApply_Dashboard_Updated(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "dashboard.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: Dashboard
metadata:
  dash0extensions:
    id: existing-dashboard-id
  name: test-dashboard
spec:
  display:
    name: Test Dashboard
  layouts:
    - kind: Grid
      spec:
        items: []
  panels: {}
`), 0644)
	require.NoError(t, err)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	// Dashboard exists (return 200 on get)
	server.On(http.MethodGet, "/api/dashboards/dash0-cli", testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDashboardsImportSuccess,
		Validator:  testutil.RequireAuthHeader,
	})
	// Import succeeds
	server.WithDashboardImport(testutil.FixtureDashboardsImportSuccess)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "Dashboard")
	assert.Contains(t, output, "updated")
}

func TestApply_MultipleDocuments(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "assets.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: CheckRule
name: check-rule-1
expression: up == 0
---
kind: CheckRule
name: check-rule-2
expression: down == 1
`), 0644)
	require.NoError(t, err)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	// No existing check rules
	server.OnPattern(http.MethodGet, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureCheckRulesNotFound,
	})
	// Import succeeds
	server.WithCheckRuleImport(testutil.FixtureCheckRulesImportSuccess)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	// Should have two "created" outputs
	assert.Equal(t, 2, countOccurrences(output, "created"))
}

func TestApply_DryRun(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "assets.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: Dashboard
metadata:
  name: test-dashboard
---
kind: CheckRule
name: test-rule
expression: up == 0
---
kind: View
metadata:
  name: test-view
`), 0644)
	require.NoError(t, err)

	// No mock server needed for dry run
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", yamlFile, "--dry-run"})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "Dry run: 3 document(s) validated successfully")
	assert.Contains(t, output, "Dashboard")
	assert.Contains(t, output, "CheckRule")
	assert.Contains(t, output, "View")
}

func TestApply_InvalidKind(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "invalid.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: Pod
metadata:
  name: test-pod
`), 0644)
	require.NoError(t, err)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", yamlFile, "--dry-run"})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "unsupported kind")
}

func TestApply_MissingKind(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "missing-kind.yaml")
	err := os.WriteFile(yamlFile, []byte(`metadata:
  name: test-asset
`), 0644)
	require.NoError(t, err)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", yamlFile, "--dry-run"})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "missing 'kind' field")
}

func TestApply_FromStdin(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureCheckRulesNotFound,
	})
	server.WithCheckRuleImport(testutil.FixtureCheckRulesImportSuccess)

	// Create a temp file to simulate stdin
	tmpDir := t.TempDir()
	stdinFile := filepath.Join(tmpDir, "stdin.yaml")
	err := os.WriteFile(stdinFile, []byte(`kind: CheckRule
name: stdin-check-rule
expression: up == 0
`), 0644)
	require.NoError(t, err)

	// Read the file to simulate stdin
	stdinData, err := os.ReadFile(stdinFile)
	require.NoError(t, err)

	// Redirect stdin
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	go func() {
		w.Write(stdinData)
		w.Close()
	}()

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", "-", "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "created")
}

func TestApply_View_Created(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "view.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: View
metadata:
  name: test-view
spec:
  display:
    name: Test View
  type: logs
  filter: []
  table:
    columns: []
    sort: []
`), 0644)
	require.NoError(t, err)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureViewsNotFound,
	})
	server.WithViewImport(testutil.FixtureViewsImportSuccess)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "View")
	assert.Contains(t, output, "created")
}

func TestApply_SyntheticCheck_Created(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "syntheticcheck.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: SyntheticCheck
metadata:
  name: test-synthetic-check
spec:
  display:
    name: Test Synthetic Check
  http:
    url: https://example.com/health
    method: GET
  scheduling:
    interval: 1m
    timeout: 30s
  locations:
    - eu-west-1
`), 0644)
	require.NoError(t, err)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, syntheticCheckIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureSyntheticChecksNotFound,
	})
	server.WithSyntheticCheckImport(testutil.FixtureSyntheticChecksImportSuccess)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "SyntheticCheck")
	assert.Contains(t, output, "created")
}

func TestApply_MissingFile(t *testing.T) {
	testutil.SetupTestEnv(t)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", "/nonexistent/file.yaml"})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "failed to read input")
}

func TestApply_MissingFileFlag(t *testing.T) {
	testutil.SetupTestEnv(t)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "file is required")
}

func TestApply_PrometheusRule(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "prometheusrule.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: test-rules
spec:
  groups:
    - name: test-group
      interval: 1m
      rules:
        - alert: HighErrorRate
          expr: sum(rate(errors[5m])) > 0.1
          for: 5m
          labels:
            severity: critical
          annotations:
            summary: High error rate detected
`), 0644)
	require.NoError(t, err)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureCheckRulesNotFound,
	})
	server.WithCheckRuleImport(testutil.FixtureCheckRulesImportSuccess)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "PrometheusRule")
	assert.Contains(t, output, "created")
}

// countOccurrences counts the number of times substr appears in s
func countOccurrences(s, substr string) int {
	count := 0
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			count++
		}
	}
	return count
}
