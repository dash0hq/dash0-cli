//go:build integration

package timeseriesaggregations

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiPath       = "/api/time-series-aggregations"
	testAuthToken = "auth_test_token"

	fixtureListSuccess     = "timeseriesaggregations/list_success.json"
	fixtureListEmpty       = "timeseriesaggregations/list_empty.json"
	fixtureListAllOptional = "timeseriesaggregations/list_all_optional_fields.json"
	fixtureGetSuccess      = "timeseriesaggregations/get_success.json"
	fixtureCreateResponse  = "timeseriesaggregations/create_response.json"
	fixtureUpdateResponse  = "timeseriesaggregations/update_response.json"
	fixtureNotFound        = "timeseriesaggregations/error_not_found.json"
	fixtureForbidden       = "timeseriesaggregations/error_forbidden.json"
	fixtureWrongDataset    = "timeseriesaggregations/error_wrong_dataset.json"
	fixtureUnauthorized    = "dashboards/error_unauthorized.json"
	testOrigin             = "http-server-request-duration"
	testID                 = "d54caa75-e94b-43c7-8470-23e4ab852ab6"
)

var originPattern = regexp.MustCompile(`^/api/time-series-aggregations/[^/]+$`)

// newRootCmd mirrors the real command tree. This command is not gated behind
// --experimental, unlike spam filters and teams.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewTimeSeriesAggregationsCmd())
	return root
}

// writeDoc writes a time series aggregation YAML document to a temp file and
// returns its path.
func writeDoc(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "aggregation.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

const docWithOrigin = `apiVersion: dash0.com/v1alpha1
kind: Dash0TimeSeriesAggregation
metadata:
  name: http-server-request-duration
  labels:
    dash0.com/origin: http-server-request-duration
spec:
  enabled: true
  display:
    name: HTTP server request duration
  match:
    metricNameMatcher:
      operator: is
      value: http.server.request.duration
  sample:
    interval: 5m
`

const docWithoutOrigin = `apiVersion: dash0.com/v1alpha1
kind: Dash0TimeSeriesAggregation
metadata:
  name: no-origin
spec:
  enabled: true
  display:
    name: No origin
  match:
    metricNameMatcher:
      operator: is
      value: http.server.request.duration
  sample:
    interval: 5m
`

// docWithOriginAndID carries both labels. The API ignores the body's id and
// targets the path's origin, so the CLI must still upsert by origin.
const docWithOriginAndID = `apiVersion: dash0.com/v1alpha1
kind: Dash0TimeSeriesAggregation
metadata:
  name: http-server-request-duration
  labels:
    dash0.com/origin: http-server-request-duration
    dash0.com/id: d54caa75-e94b-43c7-8470-23e4ab852ab6
spec:
  enabled: true
  display:
    name: HTTP server request duration
  match:
    metricNameMatcher:
      operator: is
      value: http.server.request.duration
  sample:
    interval: 5m
`

func assertPUTPath(t *testing.T, requests []testutil.RecordedRequest, wantPath, msg string) {
	t.Helper()
	for _, req := range requests {
		if req.Method == http.MethodPut && req.Path == wantPath {
			return
		}
	}
	t.Fatalf("%s\nwant: PUT %s\ngot requests: %v", msg, wantPath, describe(requests))
}

func assertNoPOST(t *testing.T, requests []testutil.RecordedRequest) {
	t.Helper()
	for _, req := range requests {
		if req.Method == http.MethodPost {
			t.Fatalf("expected no POST; the create path must always PUT by origin, got requests: %v", describe(requests))
		}
	}
}

func describe(requests []testutil.RecordedRequest) []string {
	seen := make([]string, 0, len(requests))
	for _, req := range requests {
		seen = append(seen, req.Method+" "+req.Path)
	}
	return seen
}

func TestListTimeSeriesAggregations_TableFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "table"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "HTTP server request duration")
	assert.Contains(t, output, testID)
	assert.Contains(t, output, "INTERVAL")
	assert.Contains(t, output, "5m")
	// No URL column: the API client ships no deeplink helper for this kind.
	assert.NotContains(t, output, "URL")
}

func TestListTimeSeriesAggregations_WideFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "wide"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "DATASET")
	assert.Contains(t, output, "ORIGIN")
	assert.Contains(t, output, testOrigin)
	assert.NotContains(t, output, "URL")
}

func TestListTimeSeriesAggregations_JSONFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, `"kind": "Dash0TimeSeriesAggregation"`)
	assert.Contains(t, output, `"dash0.com/origin": "http-server-request-duration"`)
}

func TestListTimeSeriesAggregations_YAMLFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "kind: Dash0TimeSeriesAggregation")
	// A multi-document stream, so the output pipes into `dash0 apply -f -`.
	assert.Contains(t, output, "---")
}

func TestListTimeSeriesAggregations_CSVFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "csv"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "name,id,interval,enabled,dataset,origin")
}

func TestListTimeSeriesAggregations_Empty(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListEmpty,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "table"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "No time series aggregations found.")
}

// No wire-captured fixture populates the optional spec fields, so a decode
// regression in one would only surface against a real environment that uses it.
func TestListTimeSeriesAggregations_AllOptionalFields(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListAllOptional,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "priority: 10")
	assert.Contains(t, output, "delay: 30s")
	assert.Contains(t, output, "staleAfter: 10m")
	assert.Contains(t, output, "otherFilters:")
	assert.Contains(t, output, "attributeModifications:")
	assert.Contains(t, output, "drop_attributes")
}

func TestGetTimeSeriesAggregation_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "get", testOrigin, "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "table"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "Kind: Time series aggregation")
	assert.Contains(t, output, "Origin: "+testOrigin)
	assert.Contains(t, output, "Interval: 5m")
	assert.Contains(t, output, "Enabled: true")
}

// An undecodable 200 leaves the aggregation nil with a nil error, and the
// summary path reads aggregation.Spec directly, so this used to panic.
func TestGetTimeSeriesAggregation_UndecodableSuccessBodyIsAnError(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originPattern, testutil.MockResponse{
		StatusCode:  http.StatusOK,
		Body:        "not json",
		ContentType: "text/html",
		Validator:   testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "get", testOrigin, "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "table"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "returned no time series aggregation")
	assert.Contains(t, err.Error(), testOrigin)
}

func TestGetTimeSeriesAggregation_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   fixtureNotFound,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "get", "no-such-origin", "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "time series aggregation")
	assert.Contains(t, err.Error(), "no-such-origin")
	assert.Contains(t, err.Error(), "not found")
}

// A token without the admin role is the most likely first-run failure for this
// asset type, and every endpoint answers it with a 403.
func TestTimeSeriesAggregations_Forbidden(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{
		StatusCode: http.StatusForbidden,
		BodyFile:   fixtureForbidden,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "list", "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "admin role")
	// The credential must never be echoed back, in any error path.
	assert.NotContains(t, err.Error(), testAuthToken)
}

func TestTimeSeriesAggregations_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   fixtureUnauthorized,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "list", "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
	assert.NotContains(t, err.Error(), testAuthToken)
}

func TestListTimeSeriesAggregations_MalformedResponse(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       json.RawMessage(`{"timeSeriesAggregations": [{"kind": "Dash0TimeSeriesAggregation", "spec": {"sample": {"interval": 42}}}]}`),
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "list", "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "interval")
}

// Create always PUTs by origin and never POSTs, because the API rejects a POST
// without an origin, and one whose origin already exists.
func TestCreateTimeSeriesAggregationFromFile_UpsertByOrigin_Creates(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	// No existing aggregation, so the preflight GET 404s and the action is
	// reported as "created".
	server.OnPattern(http.MethodGet, originPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   fixtureNotFound,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPut, originPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureCreateResponse,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "create", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, `Time series aggregation "HTTP server request duration" created`)
	assertPUTPath(t, server.Requests(), apiPath+"/"+testOrigin, "create must upsert by origin")
	assertNoPOST(t, server.Requests())
}

func TestCreateTimeSeriesAggregationFromFile_UpsertByOrigin_Updates(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPut, originPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureUpdateResponse,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "create", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, `updated`)
	assertPUTPath(t, server.Requests(), apiPath+"/"+testOrigin, "create must upsert by origin on an existing aggregation")
	assertNoPOST(t, server.Requests())
}

// The server ignores the body's id and targets the path's origin, so a document
// carrying both labels must still be upserted by origin.
func TestCreateTimeSeriesAggregationFromFile_IDInBodyIgnored(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPut, originPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureUpdateResponse,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "create", "-f", writeDoc(t, docWithOriginAndID), "--api-url", server.URL, "--auth-token", testAuthToken})

	require.NoError(t, cmd.Execute())
	assertPUTPath(t, server.Requests(), apiPath+"/"+testOrigin, "origin wins over the id in the body")
	assertNoPOST(t, server.Requests())
}

// The origin travels in the URL path. Leaving version, source, or dataset in
// the body would send server state back as if the user had authored it.
func TestCreateTimeSeriesAggregationFromFile_StripsOriginFromBody(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   fixtureNotFound,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPut, originPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureCreateResponse,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "create", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken})
	require.NoError(t, cmd.Execute())

	var body *dash0api.TimeSeriesAggregationDefinition
	for _, req := range server.Requests() {
		if req.Method == http.MethodPut {
			var decoded dash0api.TimeSeriesAggregationDefinition
			require.NoError(t, json.Unmarshal(req.Body, &decoded))
			body = &decoded
		}
	}
	require.NotNil(t, body, "expected a PUT request")
	if body.Metadata.Labels != nil {
		assert.Nil(t, body.Metadata.Labels.Dash0Comorigin, "origin travels in the URL path, not the body")
		assert.Nil(t, body.Metadata.Labels.Dash0Comversion)
		assert.Nil(t, body.Metadata.Labels.Dash0Comsource)
	}
}

// Origin is mandatory and there is no create path to fall back to, so this must
// fail locally, with no HTTP request at all.
func TestCreateTimeSeriesAggregationFromFile_MissingOriginFails(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "create", "-f", writeDoc(t, docWithoutOrigin), "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), `dash0.com/origin`)
	assert.Empty(t, server.Requests(), "the document must be rejected before any API call")
}

// --dry-run must catch what a real create would, not report a valid document.
func TestCreateTimeSeriesAggregationFromFile_MissingOriginFailsDryRun(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "create", "-f", writeDoc(t, docWithoutOrigin), "--dry-run", "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), `dash0.com/origin`)
}

// Origins are unique per organization while each aggregation belongs to one
// dataset, so this is the error a user hits the first time they apply one asset
// directory to a second dataset. The bare 400 needs translating.
func TestCreateTimeSeriesAggregationFromFile_WrongDatasetHint(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originPattern, testutil.MockResponse{
		StatusCode: http.StatusBadRequest,
		BodyFile:   fixtureWrongDataset,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "create", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists in another dataset")
	assert.Contains(t, err.Error(), "unique per organization")
	assert.Contains(t, err.Error(), "distinct origin per dataset")
	// The advice is reachable as a hint rather than buried in the message.
	assert.Contains(t, err.Error(), "\nHint:")
	// It names the constraint and the fix without inventing a replacement
	// origin the user never chose.
	assert.NotContains(t, err.Error(), testOrigin+"-staging")
	assertNoPOST(t, server.Requests())
}

// TestUpdateTimeSeriesAggregation_UnchangedDocumentReportsNoChanges proves
// marshalForDiff strips the server-managed labels. dash0.com/version is
// incremented on every PUT, so without the strip a reapply of unchanged
// content would always render a diff and the idempotency roundtrip could
// never pass.
func TestUpdateTimeSeriesAggregation_UnchangedDocumentReportsNoChanges(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	// The response carries version "2" while the "before" state carries "1",
	// mirroring the real server's behavior on every PUT.
	server.OnPattern(http.MethodPut, originPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "update", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "no changes")
}

// TestUpdateTimeSeriesAggregation_ChangedFieldShowsDiff keeps the assertion
// above from passing vacuously: if the diff never rendered anything, this
// test would fail too.
func TestUpdateTimeSeriesAggregation_ChangedFieldShowsDiff(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPut, originPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureUpdateResponse,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "update", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "-    interval: 5m")
	assert.Contains(t, output, "+    interval: 1m")
	// The version bump the server applies on every PUT must not show up.
	assert.NotContains(t, output, "dash0.com/version")
}

func TestUpdateTimeSeriesAggregation_ArgumentMustMatchFile(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "update", "some-other-origin", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	// The document carries an origin and no id, so the message names only the
	// origin rather than reporting an empty id the file never had.
	assert.Contains(t, err.Error(), `does not match the file's origin ("http-server-request-duration")`)
	assert.Empty(t, server.Requests(), "a mismatched argument must fail before any API call")
}

func TestUpdateTimeSeriesAggregation_DryRunDoesNotWrite(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "update", "-f", writeDoc(t, docWithOrigin), "--dry-run", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	for _, req := range server.Requests() {
		assert.Equal(t, http.MethodGet, req.Method, "--dry-run must not write")
	}
}

func TestDeleteTimeSeriesAggregation_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	// The live API answers 204 with an empty body. The mock always sets a JSON
	// content type, which the generated response parser then tries to
	// unmarshal, so the fixture uses 200 with an empty object — the same shape
	// every other delete test in this repo uses.
	server.OnPattern(http.MethodDelete, originPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "delete", testOrigin, "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, `Time series aggregation "`+testOrigin+`" deleted`)
}

// TestDeleteTimeSeriesAggregation_WrongDataset asserts the cross-dataset 400
// fails even under --force. The aggregation exists and belongs to another
// dataset, so reporting success would claim a deletion that never happened.
func TestDeleteTimeSeriesAggregation_WrongDataset(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, originPattern, testutil.MockResponse{
		StatusCode: http.StatusBadRequest,
		BodyFile:   fixtureWrongDataset,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "delete", testOrigin, "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists in another dataset")
}

// TestDeleteTimeSeriesAggregation_AlreadyDeletedWithForce covers the path that
// fires only if the API ever starts returning 404 for an absent aggregation.
// Today it returns 204, matching every other asset type in this CLI.
func TestDeleteTimeSeriesAggregation_AlreadyDeletedWithForce(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, originPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   fixtureNotFound,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "delete", "no-such-origin", "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

	require.NoError(t, cmd.Execute())
}

func TestDeleteTimeSeriesAggregation_NotFoundWithoutForce(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, originPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   fixtureNotFound,
		Validator:  testutil.RequireHeaders,
	})

	// Agent mode skips the confirmation prompt, so the test exercises the
	// error path rather than blocking on stdin. Set directly rather than via
	// the flag: agentmode.Enabled is initialized in main(), which tests do not
	// run.
	prev := agentmode.Enabled
	agentmode.Enabled = true
	t.Cleanup(func() { agentmode.Enabled = prev })

	cmd := newRootCmd()
	cmd.SetArgs([]string{"tsa", "delete", "no-such-origin", "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
