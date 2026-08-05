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
	apiPathSLOs   = "/api/slos"
	testAuthToken = "auth_test_token"
	testSLOID     = "00000000-0000-0000-0000-000000000001"
	testSLOID2    = "00000000-0000-0000-0000-000000000002"
	// testSLOOrigin is the dash0.com/origin label carried by the first SLO in
	// slos/list_success.json and by slos/get_success.json.
	testSLOOrigin        = "checkout-origin"
	fixtureListSuccess   = "slos/list_success.json"
	fixtureListEmpty     = "slos/list_empty.json"
	fixtureGetSuccess    = "slos/get_success.json"
	fixtureCreateSuccess = "slos/create_success.json"
	fixtureUpdateSuccess = "slos/update_success.json"
	// Same spec as get_success.json, with only dash0.com/version and
	// dash0.com/updated-at bumped — what the server really returns for a PUT
	// whose body did not change anything.
	fixtureUpdateUnchanged = "slos/update_unchanged.json"
	fixtureNotFound        = "slos/error_not_found.json"
	fixtureUnauthorized    = "dashboards/error_unauthorized.json"
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
	// The fixture carries two SLOs; --limit 1 must truncate to the first.
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json", "--limit", "1"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, `"kind": "SLO"`)
	assert.Contains(t, output, `"apiVersion": "openslo.com/v1"`)
	assert.Contains(t, output, `"metadata"`)
	assert.Contains(t, output, `"spec"`)
	// --limit 1 truncates: only the first SLO is emitted, not the second.
	assert.Contains(t, output, testSLOID)
	assert.NotContains(t, output, testSLOID2)
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
	// CSV/wide output adds DATASET, ORIGIN, and URL columns — the shared
	// contract documented in docs/commands.md. Origin matters most here: it is
	// the SLO upsert key, so it is the field scripts need.
	assert.Contains(t, output, "dataset")
	assert.Contains(t, output, "origin")
	assert.Contains(t, output, "url")
	assert.Contains(t, output, testSLOOrigin)
	assert.Contains(t, output, "slo_id="+testSLOID)
	assert.Contains(t, output, "Checkout availability")
}

// TestListSLOs_WideFormat pins the shared wide-format contract: `-o wide` adds
// DATASET, ORIGIN, and URL. ORIGIN was missing, which mattered because origin
// is the SLO upsert key and therefore the column users script against.
func TestListSLOs_WideFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathSLOs, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "wide"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "DATASET")
	assert.Contains(t, output, "ORIGIN")
	assert.Contains(t, output, "URL")
	assert.Contains(t, output, testSLOOrigin)
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
	// Origin is the SLO upsert key, so the table has to show it.
	assert.Contains(t, output, "Origin: "+testSLOOrigin)
}

// TestGetSLO_ByOrigin_DeeplinkUsesResponseID pins that `slos get <origin>`
// builds the deep link from the id on the response, not from the argument.
// `GET /api/slos/{originOrId}` accepts either, so passing an origin used to
// produce a URL with the origin embedded — a link that does not resolve.
func TestGetSLO_ByOrigin_DeeplinkUsesResponseID(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, sloIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"get", testSLOOrigin, "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "slo_id="+testSLOID, "the deep link must use the id from the response")
	assert.NotContains(t, output, "slo_id="+testSLOOrigin, "a deep link built from an origin does not resolve")
	assert.Contains(t, output, "Origin: "+testSLOOrigin)
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

// TestDeleteSLO_ForceIdempotentOn404 asserts that `delete --force` treats an
// already-deleted SLO as success (exit 0) and prints an "already deleted" line
// to stderr. Regression coverage matching CHANGELOG 1.16.2 / issue #217.
func TestDeleteSLO_ForceIdempotentOn404(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, sloIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   fixtureNotFound,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"delete", testSLOID, "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	stderr := testutil.CaptureStderr(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err, "expected exit 0 with --force on 404")
	assert.Contains(t, stderr, "was already deleted")
	assert.Contains(t, stderr, testSLOID)
}

// TestUpdateSLO_StripsServerFields pins the strip contract: an input that
// still carries server-managed fields (as an exported `slos get -o yaml`
// would) must not send dash0.com/version, dash0.com/origin, or the
// created-at/updated-at timestamps back on the wire.
func TestUpdateSLO_StripsServerFields(t *testing.T) {
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
	require.NoError(t, os.WriteFile(yamlFile, []byte(sloUpdateWithServerFieldsYAML), 0644))

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Equal(t, http.MethodPut, req.Method)

	// Decode the wire body and assert the server-managed fields are absent.
	var sent dash0api.SloDefinition
	require.NoError(t, json.Unmarshal(req.Body, &sent))
	require.NotNil(t, sent.Metadata.Labels)
	assert.Nil(t, sent.Metadata.Labels.Dash0Comversion, "dash0.com/version must be stripped")
	assert.Nil(t, sent.Metadata.Labels.Dash0Comorigin, "dash0.com/origin must be stripped")
	if sent.Metadata.Annotations != nil {
		_, hasCreatedAt := sent.Metadata.Annotations.Get("dash0.com/created-at")
		assert.False(t, hasCreatedAt, "dash0.com/created-at must be stripped")
	}
	// Belt-and-suspenders: the raw body must not carry the stripped keys.
	body := string(req.Body)
	assert.NotContains(t, body, "dash0.com/version")
	assert.NotContains(t, body, "dash0.com/created-at")
	assert.NotContains(t, body, "dash0.com/origin")
}

// TestCreateSLO_DryRun asserts that `create --dry-run` validates without
// touching the API.
func TestCreateSLO_DryRun(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "slo.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(sloCreateYAML), 0644))

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken, "--dry-run"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "Dry run")
	// No API call must have been made.
	assert.Nil(t, server.LastRequest())
}

// TestUpdateSLO_DryRun asserts that `update --dry-run` fetches the current
// state for the diff but never issues the PUT.
func TestUpdateSLO_DryRun(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, sloIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "slo.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(sloUpdateYAML), 0644))

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken, "--dry-run"})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// The only request must be the GET used to build the diff — never a PUT.
	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Equal(t, http.MethodGet, req.Method)
}

// assertPUTPath finds a PUT request in the recorded stream that targets the
// given path. The upsert flow issues a preflight GET before the write, so
// LastRequest is not necessarily the PUT — scan server.Requests() instead.
func assertPUTPath(t *testing.T, requests []testutil.RecordedRequest, wantPath, msg string) {
	t.Helper()
	for _, req := range requests {
		if req.Method == http.MethodPut && req.Path == wantPath {
			return
		}
	}
	seen := make([]string, 0, len(requests))
	for _, req := range requests {
		seen = append(seen, req.Method+" "+req.Path)
	}
	t.Fatalf("%s\nwant: PUT %s\ngot requests: %v", msg, wantPath, seen)
}

// assertPOSTPath finds a POST request in the recorded stream that targets the
// given path. Mirrors assertPUTPath for the fall-through-to-create path.
func assertPOSTPath(t *testing.T, requests []testutil.RecordedRequest, wantPath, msg string) {
	t.Helper()
	for _, req := range requests {
		if req.Method == http.MethodPost && req.Path == wantPath {
			return
		}
	}
	seen := make([]string, 0, len(requests))
	for _, req := range requests {
		seen = append(seen, req.Method+" "+req.Path)
	}
	t.Fatalf("%s\nwant: POST %s\ngot requests: %v", msg, wantPath, seen)
}

// assertNoMethod asserts that no request in the recorded stream used the given
// method — e.g. that an upsert-by-PUT never fell through to POST.
func assertNoMethod(t *testing.T, requests []testutil.RecordedRequest, method, msg string) {
	t.Helper()
	for _, req := range requests {
		assert.NotEqual(t, method, req.Method, msg)
	}
}

// sloWithLabels renders an OpenSLO v1 SLO document carrying the given
// dash0.com labels block, used to drive the upsert-routing tests through the
// real `dash0 slos create -f <file>` command path.
func sloWithLabels(labels string) string {
	return `apiVersion: openslo.com/v1
kind: SLO
metadata:
  name: checkout-availability
  labels:
` + labels + `  annotations:
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
}

// TestCreateSLOFromFile_UpsertByOrigin asserts that an SLO YAML carrying only a
// dash0.com/origin label (no id) routes to PUT /api/slos/{origin} rather than
// POST. Regression coverage for the "download an origin-only CR, reapply via
// CLI" idempotency loop — origin must upsert in place, never POST a duplicate.
func TestCreateSLOFromFile_UpsertByOrigin(t *testing.T) {
	testutil.SetupTestEnv(t)

	const origin = "my-slo-origin"

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
	require.NoError(t, os.WriteFile(yamlFile, []byte(sloWithLabels("    dash0.com/origin: "+origin+"\n")), 0644))

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	assertPUTPath(t, server.Requests(), apiPathSLOs+"/"+origin, "expected PUT to /api/slos/{origin}")
	assertNoMethod(t, server.Requests(), http.MethodPost, "origin-only input must upsert via PUT, never POST a duplicate")
}

// TestCreateSLOFromFile_UpsertByID asserts that an SLO YAML carrying only a
// dash0.com/id label (a server-style slo_… id, no origin) routes to PUT
// /api/slos/{id} when the preflight GET finds the SLO in the target env.
func TestCreateSLOFromFile_UpsertByID(t *testing.T) {
	testutil.SetupTestEnv(t)

	const sloID = "slo_01k5vpx97efdnrkqan15b41k84"

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
	require.NoError(t, os.WriteFile(yamlFile, []byte(sloWithLabels("    dash0.com/id: "+sloID+"\n")), 0644))

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	assertPUTPath(t, server.Requests(), apiPathSLOs+"/"+sloID, "expected PUT-by-id, got POST — the id label should route to upsert")
	assertNoMethod(t, server.Requests(), http.MethodPost, "id present with a 200 preflight must upsert via PUT, never POST")
}

// TestCreateSLOFromFile_UpsertByID_FallsBackToPOSTWhenNotFound asserts that
// when the id in the input YAML does not exist in the target environment
// (cross-environment apply — download from org A, apply to org B), the CLI
// falls back to POST instead of PUT-ing to an unknown id and getting a 404.
// Without this, `dash0 apply` is not idempotent across environments.
func TestCreateSLOFromFile_UpsertByID_FallsBackToPOSTWhenNotFound(t *testing.T) {
	testutil.SetupTestEnv(t)

	const sloID = "slo_01k5vpx97efdnrkqan15b41k84"

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	// Preflight GET returns 404 — the id belongs to a different org.
	server.OnPattern(http.MethodGet, sloIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   fixtureNotFound,
		Validator:  testutil.RequireHeaders,
	})
	// POST fallback returns the freshly-created SLO.
	server.On(http.MethodPost, apiPathSLOs, testutil.MockResponse{
		StatusCode: http.StatusCreated,
		BodyFile:   fixtureCreateSuccess,
		Validator:  testutil.RequireHeaders,
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "slo.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(sloWithLabels("    dash0.com/id: "+sloID+"\n")), 0644))

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err, "cross-env apply must not fail — an id from a different org should trigger POST fallback")

	assertNoMethod(t, server.Requests(), http.MethodPut, "PUT to an unknown id would 404; expected POST fallback instead")
	assertPOSTPath(t, server.Requests(), apiPathSLOs, "expected POST fallback after GET 404")

	// The POST body must not carry the source organization's id. SLO ids are
	// assigned by the server on create, and StripSLOServerFields clears
	// dash0.com/id along with version, origin, dataset, source, and the
	// timestamps — so the cross-environment fallback cannot leak a foreign
	// identifier. This assertion pins that guarantee at the wire level.
	post := findRecordedRequest(server.Requests(), http.MethodPost, apiPathSLOs)
	require.NotNil(t, post, "expected a recorded POST to %s", apiPathSLOs)
	var body struct {
		Metadata struct {
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(post.Body, &body))
	assert.NotContains(t, body.Metadata.Labels, "dash0.com/id",
		"the POST fallback must not send the source organization's dash0.com/id — the server assigns it")
}

// findRecordedRequest returns the first recorded request matching method and
// path, or nil when there is none.
func findRecordedRequest(requests []testutil.RecordedRequest, method, path string) *testutil.RecordedRequest {
	for i := range requests {
		if requests[i].Method == method && requests[i].Path == path {
			return &requests[i]
		}
	}
	return nil
}

// TestCreateSLOFromFile_OriginWinsOverID asserts that when both origin and id
// labels are present, origin is the upsert key.
func TestCreateSLOFromFile_OriginWinsOverID(t *testing.T) {
	testutil.SetupTestEnv(t)

	const origin = "my-slo-origin"
	const sloID = "slo_01k5vpx97efdnrkqan15b41k84"

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
	require.NoError(t, os.WriteFile(yamlFile, []byte(sloWithLabels(
		"    dash0.com/origin: "+origin+"\n    dash0.com/id: "+sloID+"\n")), 0644))

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	assertPUTPath(t, server.Requests(), apiPathSLOs+"/"+origin, "origin must win over id when both are present")
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

// sloUpdateWithServerFieldsYAML mirrors the shape of an exported
// `slos get -o yaml`: it carries the server-managed dash0.com/version,
// dash0.com/dataset, and dash0.com/origin labels plus the created-at/updated-at
// annotations. The update path must strip all of these before the PUT.
const sloUpdateWithServerFieldsYAML = `apiVersion: openslo.com/v1
kind: SLO
metadata:
  name: checkout-availability
  labels:
    dash0.com/id: 00000000-0000-0000-0000-000000000001
    dash0.com/version: "1"
    dash0.com/dataset: default
    dash0.com/origin: tf_6f2a1c8e-3b47-4d90-a1e5-9c73b0d84f21
  annotations:
    dash0.com/display-name: Checkout availability
    dash0.com/enabled: "true"
    dash0.com/created-at: "2026-01-15T10:00:00Z"
    dash0.com/updated-at: "2026-01-15T10:00:00Z"
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

// TestUpdateSLO_UnchangedDocumentReportsNoChanges pins actual idempotency: a
// re-apply of an unchanged SLO document must be a no-op from the user's point
// of view, reported as "no changes" — not merely "no duplicate was created".
//
// The mock reproduces the server behavior that made this fail: the PUT response
// (update_unchanged.json) is byte-for-byte the GET response (get_success.json)
// except that the server bumped dash0.com/version 1 -> 2 and dash0.com/updated-at
// 10:00 -> 12:00. Because asset.marshalForDiff had no *dash0api.SloDefinition
// case, neither side of the diff was normalized and the CLI printed a spurious
// two-line version/updated-at diff on every re-apply. This test fails without
// the strip and needs no live API, which matters because the roundtrip scripts
// are gated on org entitlement in some environments.
func TestUpdateSLO_UnchangedDocumentReportsNoChanges(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, sloIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPut, sloIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureUpdateUnchanged,
		Validator:  testutil.RequireHeaders,
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "slo.yaml")
	// The same document an export produces — identical spec, server-managed
	// metadata still attached.
	require.NoError(t, os.WriteFile(yamlFile, []byte(sloUpdateWithServerFieldsYAML), 0644))

	cmd := NewSlosCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	assert.Contains(t, output, `SLO "Checkout availability": no changes`,
		"re-applying an unchanged SLO must report no changes")
	// The whole point: no server-managed field may leak into the diff.
	assert.NotContains(t, output, "dash0.com/updated-at",
		"a server-bumped updated-at must not surface as a change")
	assert.NotContains(t, output, "dash0.com/version",
		"a server-bumped version must not surface as a change")
	assert.NotContains(t, output, "dash0.com/created-at")
	assert.NotContains(t, output, "(before)", "no unified diff header should be printed at all")
}
