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
	assert.Contains(t, output, "Check rule")
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
	assert.Contains(t, output, "Check rule")
	assert.Contains(t, output, "updated")

	// Verify the import request body
	importReq := findImportRequest(server.Requests(), apiPathImportCheckRule)
	require.NotNil(t, importReq, "expected an import request for check rule")
	body := string(importReq.Body)
	assert.NotContains(t, body, "dash0.com/origin")
	assert.Contains(t, body, "47b6ccbe-82ab-47c6-a613-ce0d7f34353e")
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
  name: Test Dashboard
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
	// Dashboard exists — GET by dash0Extensions.id returns 200
	server.On(http.MethodGet, "/api/dashboards/existing-dashboard-id", testutil.MockResponse{
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

	// Verify the import request body has server-generated fields stripped
	importReq := findImportRequest(server.Requests(), apiPathImportDashboard)
	require.NotNil(t, importReq, "expected an import request for dashboard")
	body := string(importReq.Body)
	assert.NotContains(t, body, `"createdAt"`)
	assert.NotContains(t, body, `"updatedAt"`)
	assert.NotContains(t, body, `"version"`)
	assert.NotContains(t, body, `"annotations"`)
	assert.Contains(t, body, "existing-dashboard-id")
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
	assert.Contains(t, output, "Dry run: 3 documents validated successfully")
	assert.Contains(t, output, "Dashboard")
	assert.Contains(t, output, "Check rule")
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
	assert.Contains(t, output, "Synthetic check")
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
	assert.Contains(t, output, "Check rule")
	assert.Contains(t, output, "created")
}

func TestApply_Directory_MultipleFiles(t *testing.T) {
	testutil.SetupTestEnv(t)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "checkrule.yaml"), []byte(`kind: CheckRule
name: test-check-rule
expression: up == 0
`), 0644)
	os.WriteFile(filepath.Join(dir, "view.yaml"), []byte(`kind: View
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

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureCheckRulesNotFound,
	})
	server.WithCheckRuleImport(testutil.FixtureCheckRulesImportSuccess)
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureViewsNotFound,
	})
	server.WithViewImport(testutil.FixtureViewsImportSuccess)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", dir, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "checkrule.yaml: Check rule")
	assert.Contains(t, output, "view.yaml: View")
	assert.Contains(t, output, "created")
}

func TestApply_Directory_NestedSubdirectories(t *testing.T) {
	testutil.SetupTestEnv(t)

	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "root.yaml"), []byte(`kind: CheckRule
name: root-rule
expression: up == 0
`), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "nested.yaml"), []byte(`kind: CheckRule
name: nested-rule
expression: down == 1
`), 0644)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureCheckRulesNotFound,
	})
	server.WithCheckRuleImport(testutil.FixtureCheckRulesImportSuccess)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", dir, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "root.yaml: Check rule")
	assert.Contains(t, output, filepath.Join("sub", "nested.yaml")+": Check rule")
	assert.Equal(t, 2, countOccurrences(output, "created"))
}

func TestApply_Directory_DryRun(t *testing.T) {
	testutil.SetupTestEnv(t)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "dashboard.yaml"), []byte(`kind: Dashboard
metadata:
  name: test-dashboard
`), 0644)
	os.WriteFile(filepath.Join(dir, "rules.yaml"), []byte(`kind: CheckRule
name: rule-1
expression: up == 0
---
kind: CheckRule
name: rule-2
expression: down == 1
`), 0644)
	os.WriteFile(filepath.Join(dir, "view.yaml"), []byte(`kind: View
metadata:
  name: test-view
`), 0644)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", dir, "--dry-run"})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "Dry run: 4 documents from 3 files validated successfully")
	assert.Contains(t, output, "dashboard.yaml")
	assert.Contains(t, output, "rules.yaml")
	assert.Contains(t, output, "view.yaml")
	assert.Contains(t, output, "Dashboard")
	assert.Contains(t, output, "Check rule")
	assert.Contains(t, output, "View")
}

func TestApply_Directory_ValidationError_BlocksAll(t *testing.T) {
	testutil.SetupTestEnv(t)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "good.yaml"), []byte(`kind: Dashboard
metadata:
  name: good-dashboard
`), 0644)
	os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(`metadata:
  name: missing-kind
`), 0644)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", dir, "--dry-run"})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "validation failed")
	assert.Contains(t, cmdErr.Error(), "bad.yaml")
	assert.Contains(t, cmdErr.Error(), "missing 'kind' field")
}

func TestApply_Directory_MultipleValidationErrors(t *testing.T) {
	testutil.SetupTestEnv(t)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a_broken.yaml"), []byte(`metadata:
  name: no-kind
`), 0644)
	os.WriteFile(filepath.Join(dir, "b_invalid.yaml"), []byte(`kind: Pod
metadata:
  name: wrong-kind
`), 0644)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", dir, "--dry-run"})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "validation failed with 2 errors")
	assert.Contains(t, cmdErr.Error(), "missing 'kind' field")
	assert.Contains(t, cmdErr.Error(), "unsupported kind")
}

func TestApply_Directory_EmptyDirectory(t *testing.T) {
	testutil.SetupTestEnv(t)

	dir := t.TempDir()

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", dir, "--dry-run"})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "no .yaml or .yml files found")
}

func TestApply_Directory_NonExistent(t *testing.T) {
	testutil.SetupTestEnv(t)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", "/nonexistent/directory"})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "failed to read input")
}

func TestApply_Dashboard_Created_StripsId(t *testing.T) {
	testutil.SetupTestEnv(t)

	// Dashboard has a dash0Extensions.id but the asset doesn't exist (404)
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "dashboard.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: Dashboard
metadata:
  dash0extensions:
    id: deleted-dashboard-uuid
  name: Re-imported Dashboard
spec:
  display:
    name: Re-imported Dashboard
  layouts:
    - kind: Grid
      spec:
        items: []
  panels: {}
`), 0644)
	require.NoError(t, err)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	// GET returns 404 — the dashboard was deleted
	server.On(http.MethodGet, "/api/dashboards/deleted-dashboard-uuid", testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureDashboardsNotFound,
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
	assert.Contains(t, output, "created")

	// Verify the import request body has dash0Extensions.id stripped (replaced with a new UUID)
	for _, req := range server.Requests() {
		if req.Method == http.MethodPost && req.Path == apiPathImportDashboard {
			body := string(req.Body)
			assert.NotContains(t, body, "deleted-dashboard-uuid")
		}
	}
}

func TestApply_CheckRule_Created_StripsId(t *testing.T) {
	testutil.SetupTestEnv(t)

	// Check rule has an ID but the asset doesn't exist (404)
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "checkrule.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: CheckRule
id: deleted-rule-uuid
name: test-check-rule
expression: up == 0
`), 0644)
	require.NoError(t, err)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	// GET returns 404 — the check rule was deleted
	server.On(http.MethodGet, "/api/alerting/check-rules/deleted-rule-uuid", testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureCheckRulesNotFound,
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
	assert.Contains(t, output, "Check rule")
	assert.Contains(t, output, "created")

	// Verify the import request body has the ID stripped
	for _, req := range server.Requests() {
		if req.Method == http.MethodPost && req.Path == apiPathImportCheckRule {
			body := string(req.Body)
			assert.NotContains(t, body, "deleted-rule-uuid")
		}
	}
}

func TestApply_View_Updated(t *testing.T) {
	testutil.SetupTestEnv(t)

	viewID := "existing-view-uuid"
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "view.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: View
metadata:
  name: test-view
  labels:
    dash0.com/id: existing-view-uuid
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
	// View exists — GET by Dash0Comid returns 200
	server.On(http.MethodGet, "/api/views/"+viewID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureViewsImportSuccess,
		Validator:  testutil.RequireAuthHeader,
	})
	// Import succeeds
	server.WithViewImport(testutil.FixtureViewsImportSuccess)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "View")
	assert.Contains(t, output, "updated")

	// Verify the import request body has server-generated fields stripped
	importReq := findImportRequest(server.Requests(), apiPathImportView)
	require.NotNil(t, importReq, "expected an import request for view")
	body := string(importReq.Body)
	assert.NotContains(t, body, `"dash0.com/origin"`)
	assert.NotContains(t, body, `"dash0.com/version"`)
	assert.NotContains(t, body, `"dash0.com/source"`)
	assert.NotContains(t, body, `"dash0.com/dataset"`)
	assert.NotContains(t, body, `"permissions"`)
	assert.Contains(t, body, "existing-view-uuid")
}

func TestApply_SyntheticCheck_Updated(t *testing.T) {
	testutil.SetupTestEnv(t)

	checkID := "existing-check-uuid"
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "syntheticcheck.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: SyntheticCheck
metadata:
  name: test-synthetic-check
  labels:
    dash0.com/id: existing-check-uuid
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
	// Synthetic check exists — GET by Dash0Comid returns 200
	server.On(http.MethodGet, "/api/synthetic-checks/"+checkID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureSyntheticChecksImportSuccess,
		Validator:  testutil.RequireAuthHeader,
	})
	// Import succeeds
	server.WithSyntheticCheckImport(testutil.FixtureSyntheticChecksImportSuccess)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "Synthetic check")
	assert.Contains(t, output, "updated")

	// Verify the import request body has server-generated fields stripped
	importReq := findImportRequest(server.Requests(), apiPathImportSyntheticCheck)
	require.NotNil(t, importReq, "expected an import request for synthetic check")
	body := string(importReq.Body)
	assert.NotContains(t, body, `"dash0.com/origin"`)
	assert.NotContains(t, body, `"dash0.com/version"`)
	assert.NotContains(t, body, `"dash0.com/dataset"`)
	assert.NotContains(t, body, `"permissions"`)
	assert.Contains(t, body, "existing-check-uuid")
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

// findImportRequest finds the first POST request to the given import API path.
func findImportRequest(requests []testutil.RecordedRequest, path string) *testutil.RecordedRequest {
	for _, req := range requests {
		if req.Method == http.MethodPost && req.Path == path {
			return &req
		}
	}
	return nil
}

func TestApply_View_Created_PreservesFilterValues(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "view.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: View
metadata:
  name: test-view-with-filter
spec:
  display:
    name: Test View With Filter
  type: logs
  filter:
    - key: otel.log.severity.range
      operator: is_one_of
      values:
        - "ERROR"
        - "FATAL"
    - key: service.name
      operator: is
      value: "my-service"
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

	// Verify filter values survive the YAML parse → JSON serialize round-trip
	importReq := findImportRequest(server.Requests(), apiPathImportView)
	require.NotNil(t, importReq, "expected an import request for view")
	body := string(importReq.Body)
	assert.Contains(t, body, "ERROR")
	assert.Contains(t, body, "FATAL")
	assert.Contains(t, body, "my-service")
	assert.Contains(t, body, "otel.log.severity.range")
	assert.Contains(t, body, "is_one_of")
}

func TestApply_View_Created_StripsId(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "view.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: View
metadata:
  name: test-view
  labels:
    dash0.com/id: deleted-view-uuid
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
	// GET returns 404 — the view was deleted
	server.On(http.MethodGet, "/api/views/deleted-view-uuid", testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureViewsNotFound,
		Validator:  testutil.RequireAuthHeader,
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

	// Verify the import request body has the ID stripped
	importReq := findImportRequest(server.Requests(), apiPathImportView)
	require.NotNil(t, importReq, "expected an import request for view")
	body := string(importReq.Body)
	assert.NotContains(t, body, "deleted-view-uuid")
}

func TestApply_SyntheticCheck_Created_StripsId(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "syntheticcheck.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: SyntheticCheck
metadata:
  name: test-synthetic-check
  labels:
    dash0.com/id: deleted-check-uuid
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
	// GET returns 404 — the synthetic check was deleted
	server.On(http.MethodGet, "/api/synthetic-checks/deleted-check-uuid", testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureSyntheticChecksNotFound,
		Validator:  testutil.RequireAuthHeader,
	})
	server.WithSyntheticCheckImport(testutil.FixtureSyntheticChecksImportSuccess)

	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "Synthetic check")
	assert.Contains(t, output, "created")

	// Verify the import request body has the ID stripped
	importReq := findImportRequest(server.Requests(), apiPathImportSyntheticCheck)
	require.NotNil(t, importReq, "expected an import request for synthetic check")
	body := string(importReq.Body)
	assert.NotContains(t, body, "deleted-check-uuid")
}
