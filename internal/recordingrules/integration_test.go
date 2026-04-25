//go:build integration

package recordingrules

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiPathRecordingRules = "/api/recording-rules"
	fixtureListSuccess    = "recordingrules/list_success.json"
	fixtureListEmpty      = "recordingrules/list_empty.json"
	fixtureGetSuccess     = "recordingrules/get_success.json"
	fixtureCreateSuccess  = "recordingrules/create_success.json"
	fixtureUnauthorized   = "dashboards/error_unauthorized.json"
)

var recordingRuleIDPattern = regexp.MustCompile(`^/api/recording-rules/[a-f0-9-]+$`)

func TestListRecordingRules_JSONFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathRecordingRules, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewRecordingRulesCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json", "--limit", "2"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// JSON output should contain full recording rule definitions
	assert.Contains(t, output, `"kind": "PrometheusRule"`)
	assert.Contains(t, output, `"metadata"`)
	assert.Contains(t, output, `"spec"`)
	assert.Contains(t, output, `"CPU Usage Average"`)
}

func TestListRecordingRules_YAMLFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathRecordingRules, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewRecordingRulesCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml", "--limit", "2"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// YAML output should contain full recording rule definitions as multi-document YAML
	assert.Contains(t, output, "kind: PrometheusRule")
	assert.Contains(t, output, "metadata:")
	assert.Contains(t, output, "spec:")
	// Multiple documents should be separated by ---
	assert.Contains(t, output, "---")
}

func TestListRecordingRules_TableFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathRecordingRules, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewRecordingRulesCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "ID")
	assert.Contains(t, output, "CPU Usage Average")
	assert.Contains(t, output, "Request Rate Summary")
}

func TestListRecordingRules_Empty(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathRecordingRules, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListEmpty,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewRecordingRulesCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "No recording rules found.")
}

func TestGetRecordingRule_JSONFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, recordingRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewRecordingRulesCmd()
	cmd.SetArgs([]string{"get", "f47ac10b-58cc-4372-a567-0e02b2c3d479", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, `"kind": "PrometheusRule"`)
	assert.Contains(t, output, `"CPU Usage Average"`)
}

func TestGetRecordingRule_YAMLFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, recordingRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewRecordingRulesCmd()
	cmd.SetArgs([]string{"get", "f47ac10b-58cc-4372-a567-0e02b2c3d479", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "kind: PrometheusRule")
	assert.Contains(t, output, "name: CPU Usage Average")
}

func TestGetRecordingRule_DefaultFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, recordingRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewRecordingRulesCmd()
	cmd.SetArgs([]string{"get", "f47ac10b-58cc-4372-a567-0e02b2c3d479", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "Kind: PrometheusRule")
	assert.Contains(t, output, "Name: CPU Usage Average")
	assert.Contains(t, output, "Groups: 1")
	assert.Contains(t, output, "Rules: 1")
}

func TestDeleteRecordingRule_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, recordingRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewRecordingRulesCmd()
	cmd.SetArgs([]string{"delete", "f47ac10b-58cc-4372-a567-0e02b2c3d479", "--api-url", server.URL, "--auth-token", testAuthToken, "--force"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "deleted")
}

func TestListRecordingRules_DatasetQueryParam(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathRecordingRules, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListEmpty,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewRecordingRulesCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "--dataset", "my-dataset"})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Equal(t, http.MethodGet, req.Method)
	assert.Equal(t, apiPathRecordingRules, req.Path)
	assert.Contains(t, req.Query, "dataset=my-dataset")
	assert.True(t, strings.HasPrefix(req.Header.Get("Authorization"), "Bearer "))
}

func TestCreateRecordingRule_DatasetQueryParam(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathRecordingRules, testutil.MockResponse{
		StatusCode: http.StatusCreated,
		BodyFile:   fixtureCreateSuccess,
		Validator:  testutil.RequireHeaders,
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "rule.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: Test Rule
spec:
  groups:
    - name: test-group
      rules:
        - record: instance:cpu_usage:avg5m
          expr: avg without(cpu) (rate(node_cpu_seconds_total{mode!="idle"}[5m]))
`), 0644)
	require.NoError(t, err)

	cmd := NewRecordingRulesCmd()
	cmd.SetArgs([]string{"create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken, "--dataset", "my-dataset"})

	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Equal(t, http.MethodPost, req.Method)
	assert.Contains(t, req.Query, "dataset=my-dataset")
}

func TestUpdateRecordingRule_DatasetQueryParam(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, recordingRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPut, recordingRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "rule.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: CPU Usage Average
  labels:
    dash0.com/id: f47ac10b-58cc-4372-a567-0e02b2c3d479
spec:
  groups:
    - name: cpu-averages
      interval: 1m
      rules:
        - record: instance:cpu_usage:avg5m
          expr: avg without(cpu) (rate(node_cpu_seconds_total{mode!="idle"}[5m]))
`), 0644)
	require.NoError(t, err)

	cmd := NewRecordingRulesCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken, "--dataset", "my-dataset"})

	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Equal(t, http.MethodPut, req.Method)
	assert.Contains(t, req.Query, "dataset=my-dataset")
}
