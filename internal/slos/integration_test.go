//go:build integration

package slos

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiPathSLOs          = "/api/slos"
	testAuthToken        = "auth_test_token"
	testSLOID            = "00000000-0000-0000-0000-000000000001"
	fixtureListSuccess   = "slos/list_success.json"
	fixtureListEmpty     = "slos/list_empty.json"
	fixtureGetSuccess    = "slos/get_success.json"
	fixtureCreateSuccess = "slos/create_success.json"
	fixtureUpdateSuccess = "slos/update_success.json"
	fixtureNotFound      = "slos/error_not_found.json"
	fixtureUnauthorized  = "dashboards/error_unauthorized.json"
)

var sloIDPattern = regexp.MustCompile(`^/api/slos/[^/]+$`)

func TestListSLOs_JSONFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathSLOs, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json", "--limit", "2"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, `"kind": "SLO"`)
	assert.Contains(t, output, `"apiVersion": "openslo.com/v1"`)
	assert.Contains(t, output, `"metadata"`)
	assert.Contains(t, output, `"spec"`)
}

func TestListSLOs_YAMLFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathSLOs, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml", "--limit", "2"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "kind: SLO")
	assert.Contains(t, output, "apiVersion: openslo.com/v1")
	assert.Contains(t, output, "metadata:")
	assert.Contains(t, output, "spec:")
}

func TestListSLOs_TableFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathSLOs, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "ID")
	assert.Contains(t, output, "Checkout availability")
	assert.Contains(t, output, testSLOID)
}

func TestListSLOs_CSVFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathSLOs, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "csv"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// CSV/wide output adds a URL deep-link column.
	assert.Contains(t, output, "url")
	assert.Contains(t, output, "slo_id="+testSLOID)
	assert.Contains(t, output, "Checkout availability")
}

func TestListSLOs_Empty(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathSLOs, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListEmpty,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "No SLOs found.")
}

func TestListSLOs_AuthError(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathSLOs, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   fixtureUnauthorized,
	})

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.Error(t, err)
}

func TestGetSLO_JSONFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, sloIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"get", testSLOID, "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, `"kind": "SLO"`)
	assert.Contains(t, output, `"checkout"`)
}

func TestGetSLO_YAMLFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, sloIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"get", testSLOID, "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "kind: SLO")
	assert.Contains(t, output, "apiVersion: openslo.com/v1")
}

func TestGetSLO_DefaultFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, sloIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"get", testSLOID, "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "Kind: SLO")
	assert.Contains(t, output, "Name: Checkout availability")
	assert.Contains(t, output, "Service: checkout")
}

func TestGetSLO_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, sloIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   fixtureNotFound,
	})

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"get", testSLOID, "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.Error(t, err)
}

func TestCreateSLO_DatasetQueryParam(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathSLOs, testutil.MockResponse{
		StatusCode: http.StatusCreated,
		BodyFile:   fixtureCreateSuccess,
		Validator:  testutil.RequireHeaders,
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "slo.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(sloCreateYAML), 0644))

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken, "--dataset", "my-dataset"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, output, `SLO "Checkout availability" created`)

	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Equal(t, http.MethodPost, req.Method)
	assert.Contains(t, req.Query, "dataset=my-dataset")
	assert.True(t, strings.HasPrefix(req.Header.Get("Authorization"), "Bearer "))
}

func TestUpdateSLO_DatasetQueryParam(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, sloIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPut, sloIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureUpdateSuccess,
		Validator:  testutil.RequireHeaders,
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "slo.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(sloUpdateYAML), 0644))

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken, "--dataset", "my-dataset"})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Equal(t, http.MethodPut, req.Method)
	assert.Contains(t, req.Query, "dataset=my-dataset")

	// The outbound body must be a valid OpenSLO v1 document.
	var sent dash0api.SloDefinition
	require.NoError(t, json.Unmarshal(req.Body, &sent))
	assert.Equal(t, "openslo.com/v1", string(sent.ApiVersion))
	assert.Equal(t, dash0api.SLO, sent.Kind)
}

func TestDeleteSLO_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, sloIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"delete", testSLOID, "--api-url", server.URL, "--auth-token", testAuthToken, "--force"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "deleted")
}

const sloCreateYAML = `apiVersion: openslo.com/v1
kind: SLO
metadata:
  name: checkout-availability
  annotations:
    dash0.com/display-name: Checkout availability
    dash0.com/enabled: "true"
spec:
  description: 99 percent of checkout HTTP requests succeed over a rolling 28-day window.
  service: checkout
  budgetingMethod: Occurrences
  timeWindow:
    - duration: 28d
      isRolling: true
  indicator:
    metadata:
      name: checkout-success-ratio
    spec:
      ratioMetric:
        counter: true
        good:
          metricSource:
            type: Prometheus
            spec:
              query: 'http_server_request_duration_seconds_count{service_name="checkout",http_response_status_code!~"5.."}'
        total:
          metricSource:
            type: Prometheus
            spec:
              query: 'http_server_request_duration_seconds_count{service_name="checkout"}'
  objectives:
    - displayName: 99% availability
      target: 0.99
`

const sloUpdateYAML = `apiVersion: openslo.com/v1
kind: SLO
metadata:
  name: checkout-availability
  labels:
    dash0.com/id: 00000000-0000-0000-0000-000000000001
  annotations:
    dash0.com/display-name: Checkout availability
    dash0.com/enabled: "true"
spec:
  description: 99.5 percent of checkout HTTP requests succeed over a rolling 28-day window.
  service: checkout
  budgetingMethod: Occurrences
  timeWindow:
    - duration: 28d
      isRolling: true
  indicator:
    metadata:
      name: checkout-success-ratio
    spec:
      ratioMetric:
        counter: true
        good:
          metricSource:
            type: Prometheus
            spec:
              query: 'http_server_request_duration_seconds_count{service_name="checkout",http_response_status_code!~"5.."}'
        total:
          metricSource:
            type: Prometheus
            spec:
              query: 'http_server_request_duration_seconds_count{service_name="checkout"}'
  objectives:
    - displayName: 99.5% availability
      target: 0.995
`
