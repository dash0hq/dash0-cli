//go:build integration

package signaltometricsrules

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiPath       = "/api/signal-to-metrics"
	apiTestPath   = "/api/signal-to-metrics/test"
	testAuthToken = "auth_test_token"

	fixtureListSuccess    = "signaltometricsrules/list_success.json"
	fixtureListEmpty      = "signaltometricsrules/list_empty.json"
	fixtureGetSuccess     = "signaltometricsrules/get_success.json"
	fixtureGetDeleted     = "signaltometricsrules/get_deleted.json"
	fixtureCreateResponse = "signaltometricsrules/create_response.json"
	fixtureUpdateResponse = "signaltometricsrules/update_response.json"
	fixtureNotFound       = "signaltometricsrules/error_not_found.json"
	fixtureBadRequest     = "signaltometricsrules/error_bad_request.json"
	fixtureTestSuccess    = "signaltometricsrules/test_success.json"
	fixtureUnauthorized   = "dashboards/error_unauthorized.json"

	testOrigin = "dash0-cli_checkout-errors"
	testID     = "7c1e2a4b-3d5f-4a6b-9c8d-0e1f2a3b4c5d"
	testName   = "Checkout error logs"
)

var originOrIDPattern = regexp.MustCompile(`^/api/signal-to-metrics/[^/]+$`)

// newRootCmd mirrors the real command tree. This command is not gated behind
// --experimental.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewSignalToMetricsRulesCmd())
	return root
}

func writeDoc(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rule.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

const docSpec = `spec:
  display:
    name: Checkout error logs
  enabled: true
  match:
    signal: logs
    filters:
      - key: service.name
        operator: is
        value: checkout
      - key: otel.log.severity.range
        operator: is
        value: ERROR
  output:
    name: checkout.error_logs
    description: Error log records emitted by the checkout service
    interval: 1m
    keepResourceAttributes:
      - operator: is
        value: service.name
`

const docWithOrigin = `kind: Dash0SignalToMetrics
metadata:
  name: checkout-errors
  labels:
    dash0.com/origin: dash0-cli_checkout-errors
` + docSpec

const docWithIDOnly = `kind: Dash0SignalToMetrics
metadata:
  name: checkout-errors
  labels:
    dash0.com/id: 7c1e2a4b-3d5f-4a6b-9c8d-0e1f2a3b4c5d
` + docSpec

const docWithoutIdentifier = `kind: Dash0SignalToMetrics
metadata:
  name: checkout-errors
` + docSpec

func describe(requests []testutil.RecordedRequest) []string {
	seen := make([]string, 0, len(requests))
	for _, req := range requests {
		seen = append(seen, req.Method+" "+req.Path)
	}
	return seen
}

func findRequest(t *testing.T, requests []testutil.RecordedRequest, method, path string) testutil.RecordedRequest {
	t.Helper()
	for _, req := range requests {
		if req.Method == method && req.Path == path {
			return req
		}
	}
	t.Fatalf("want: %s %s\ngot requests: %v", method, path, describe(requests))
	return testutil.RecordedRequest{}
}

func countMethod(requests []testutil.RecordedRequest, method string) int {
	n := 0
	for _, req := range requests {
		if req.Method == method {
			n++
		}
	}
	return n
}

func decodeRule(t *testing.T, body []byte) dash0api.SignalToMetricsDefinition {
	t.Helper()
	var rule dash0api.SignalToMetricsDefinition
	require.NoError(t, json.Unmarshal(body, &rule))
	return rule
}

func execute(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetArgs(args)
	var err error
	out := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	return out, err
}

// --- list ---

func TestListSignalToMetricsRules_TableFormat(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListSuccess, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "table")

	require.NoError(t, err)
	for _, want := range []string{"NAME", "SIGNAL", "METRIC", "ENABLED", testName, testID, "logs", "checkout.error_logs", "Frontend request duration", "spans", "false"} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "ORIGIN", "origin is a wide-only column")
}

func TestListSignalToMetricsRules_WideFormat(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListSuccess, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "wide")

	require.NoError(t, err)
	assert.Contains(t, out, "DATASET")
	assert.Contains(t, out, "ORIGIN")
	assert.Contains(t, out, testOrigin)
}

func TestListSignalToMetricsRules_CSVFormat(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListSuccess, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "csv")

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, "name,id,signal,metric,enabled,dataset,origin", lines[0])
	assert.Equal(t, "Checkout error logs,"+testID+",logs,checkout.error_logs,true,default,"+testOrigin, lines[1])
}

func TestListSignalToMetricsRules_JSONFormat(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListSuccess, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json")

	require.NoError(t, err)
	var rules []dash0api.SignalToMetricsDefinition
	require.NoError(t, json.Unmarshal([]byte(out), &rules))
	require.Len(t, rules, 2)
	assert.Equal(t, testName, rules[0].Spec.Display.Name)
	assert.Equal(t, dash0api.SignalToMetricsSignalTypeSpans, rules[1].Spec.Match.Signal)
}

func TestListSignalToMetricsRules_YAMLFormatIsMultiDocument(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListSuccess, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml")

	require.NoError(t, err)
	assert.Contains(t, out, "kind: Dash0SignalToMetrics")
	assert.Contains(t, out, "\n---\n")
}

func TestListSignalToMetricsRules_Empty(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListEmpty, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "table")

	require.NoError(t, err)
	assert.Contains(t, out, "No signal-to-metrics rules found.")
}

func TestListSignalToMetricsRules_FiltersAreSentToTheAPI(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListEmpty, Validator: testutil.RequireHeaders})

	_, err := execute(t, "s2m", "list", "--signal", "spans", "--enabled=false", "--name", "frontend", "--origin-prefix", "dash0-cli",
		"--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json")

	require.NoError(t, err)
	query, err := url.ParseQuery(findRequest(t, server.Requests(), http.MethodGet, apiPath).Query)
	require.NoError(t, err)
	assert.Equal(t, "spans", query.Get("signal"))
	assert.Equal(t, "false", query.Get("enabled"))
	assert.Equal(t, "frontend", query.Get("name"))
	assert.Equal(t, "dash0-cli", query.Get("originPrefix"))
}

func TestListSignalToMetricsRules_NoFilterMeansNoEnabledParameter(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListEmpty, Validator: testutil.RequireHeaders})

	_, err := execute(t, "s2m", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json")

	require.NoError(t, err)
	query, err := url.ParseQuery(findRequest(t, server.Requests(), http.MethodGet, apiPath).Query)
	require.NoError(t, err)
	assert.False(t, query.Has("enabled"), "omitting --enabled must list enabled and disabled rules")
}

func TestListSignalToMetricsRules_InvalidSignalFailsBeforeAnyRequest(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	_, err := execute(t, "s2m", "list", "--signal", "metrics", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid --signal "metrics"`)
	assert.Empty(t, server.Requests())
}

func TestListSignalToMetricsRules_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusUnauthorized, BodyFile: fixtureUnauthorized, Validator: testutil.RequireHeaders})

	_, err := execute(t, "s2m", "list", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

// --- get ---

func TestGetSignalToMetricsRule_Success(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "get", testOrigin, "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "table")

	require.NoError(t, err)
	for _, want := range []string{"Kind: Signal-to-metrics rule", "Name: " + testName, "ID: " + testID, "Origin: " + testOrigin, "Signal: logs", "Metric: checkout.error_logs", "Interval: 1m", "Enabled: true"} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "Deleted:")
}

func TestGetSignalToMetricsRule_Deleted(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetDeleted, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "get", testOrigin, "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "table")

	require.NoError(t, err)
	assert.Contains(t, out, "Deleted: 2026-10-22 11:30:00")
}

func TestGetSignalToMetricsRule_YAMLRoundTripsIdentifiers(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "get", testID, "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml")

	require.NoError(t, err)
	assert.Contains(t, out, "kind: Dash0SignalToMetrics")
	assert.Contains(t, out, "dash0.com/origin: "+testOrigin)
	assert.Contains(t, out, "dash0.com/id: "+testID)
}

func TestGetSignalToMetricsRule_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: fixtureNotFound, Validator: testutil.RequireHeaders})

	_, err := execute(t, "s2m", "get", "missing", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- create ---

func TestCreateSignalToMetricsRule_ByOrigin_Creates(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: fixtureNotFound, Validator: testutil.RequireHeaders})
	server.OnPattern(http.MethodPut, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "create", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, `Signal-to-metrics rule "Checkout error logs" created (id: `+testID+`)`)
	put := findRequest(t, server.Requests(), http.MethodPut, apiPath+"/"+testOrigin)
	assert.Zero(t, countMethod(server.Requests(), http.MethodPost), "an origin document must never POST")

	// The server takes the origin from the path and the dataset from the query,
	// so neither belongs in the body.
	body := decodeRule(t, put.Body)
	require.NotNil(t, body.Metadata.Labels)
	assert.Nil(t, body.Metadata.Labels.Dash0Comorigin)
	assert.Nil(t, body.Metadata.Labels.Dash0Comdataset)
	assert.Equal(t, testName, body.Spec.Display.Name)
}

func TestCreateSignalToMetricsRule_ByOrigin_Updates(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders})
	server.OnPattern(http.MethodPut, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureUpdateResponse, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "create", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "updated")
	findRequest(t, server.Requests(), http.MethodPut, apiPath+"/"+testOrigin)
}

func TestCreateSignalToMetricsRule_ByOrigin_RestoresDeletedRule(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetDeleted, Validator: testutil.RequireHeaders})
	server.OnPattern(http.MethodPut, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureUpdateResponse, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "create", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "restored")
}

func TestCreateSignalToMetricsRule_ByIDOnly_ExistingRuleIsUpdatedInPlace(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders})
	server.OnPattern(http.MethodPut, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureUpdateResponse, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "create", "-f", writeDoc(t, docWithIDOnly), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "updated")
	findRequest(t, server.Requests(), http.MethodPut, apiPath+"/"+testID)
	assert.Zero(t, countMethod(server.Requests(), http.MethodPost))
}

// A PUT to an unknown id would create a rule whose origin is that id, so a
// missing id must fall back to POST without the id, which the server rejects
// in a POST body.
func TestCreateSignalToMetricsRule_ByIDOnly_UnknownIDFallsBackToPOSTWithoutID(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: fixtureNotFound, Validator: testutil.RequireHeaders})
	server.On(http.MethodPost, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureCreateResponse, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "create", "-f", writeDoc(t, docWithIDOnly), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "created (id: 9f8e7d6c-5b4a-4392-8170-6f5e4d3c2b1a)")
	assert.Zero(t, countMethod(server.Requests(), http.MethodPut), "an unknown id must never be PUT")
	body := decodeRule(t, findRequest(t, server.Requests(), http.MethodPost, apiPath).Body)
	if body.Metadata.Labels != nil {
		assert.Nil(t, body.Metadata.Labels.Dash0Comid)
	}
}

func TestCreateSignalToMetricsRule_ByIDOnly_SurfacesNon404PreflightError(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusUnauthorized, BodyFile: fixtureUnauthorized, Validator: testutil.RequireHeaders})

	_, err := execute(t, "s2m", "create", "-f", writeDoc(t, docWithIDOnly), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Zero(t, countMethod(server.Requests(), http.MethodPost), "a failed preflight must not create a duplicate")
	assert.Zero(t, countMethod(server.Requests(), http.MethodPut))
}

func TestCreateSignalToMetricsRule_WithoutIdentifierPOSTs(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureCreateResponse, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "create", "-f", writeDoc(t, docWithoutIdentifier), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "created")
	assert.Equal(t, []string{"POST " + apiPath}, describe(server.Requests()))
}

func TestCreateSignalToMetricsRule_DryRunMakesNoRequests(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	out, err := execute(t, "s2m", "create", "-f", writeDoc(t, docWithOrigin), "--dry-run", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "Dry run: signal-to-metrics rule definition is valid")
	assert.Empty(t, server.Requests())
}

// --- update ---

func TestUpdateSignalToMetricsRule_UnchangedDocumentReportsNoChanges(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders})
	// The response bumps the version and updated-at, as the real server does.
	server.OnPattern(http.MethodPut, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureUpdateResponse, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "update", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, `Signal-to-metrics rule "Checkout error logs": no changes`)
	findRequest(t, server.Requests(), http.MethodPut, apiPath+"/"+testOrigin)
}

func TestUpdateSignalToMetricsRule_ChangedFieldShowsDiff(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders})

	changed := strings.Replace(docWithOrigin, "interval: 1m", "interval: 5m", 1)
	out, err := execute(t, "s2m", "update", "-f", writeDoc(t, changed), "--dry-run", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "-    interval: 1m")
	assert.Contains(t, out, "+    interval: 5m")
	assert.Zero(t, countMethod(server.Requests(), http.MethodPut), "--dry-run must not write")
}

func TestUpdateSignalToMetricsRule_MissingRuleFailsInsteadOfCreating(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: fixtureNotFound, Validator: testutil.RequireHeaders})

	_, err := execute(t, "s2m", "update", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.Zero(t, countMethod(server.Requests(), http.MethodPut), "the PUT endpoint upserts, so update must stop before it")
}

func TestUpdateSignalToMetricsRule_ArgumentMustMatchFile(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	_, err := execute(t, "s2m", "update", "some-other-origin", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match the file's origin")
	assert.Empty(t, server.Requests())
}

func TestUpdateSignalToMetricsRule_NoIdentifierFails(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	_, err := execute(t, "s2m", "update", "-f", writeDoc(t, docWithoutIdentifier), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no signal-to-metrics rule origin or id given")
}

// --- delete ---

func TestDeleteSignalToMetricsRule_Success(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, Body: map[string]any{}, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "delete", testOrigin, "--force", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, `Signal-to-metrics rule "`+testOrigin+`" deleted`)
	findRequest(t, server.Requests(), http.MethodDelete, apiPath+"/"+testOrigin)
}

func TestDeleteSignalToMetricsRule_AlreadyDeletedWithForce(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: fixtureNotFound, Validator: testutil.RequireHeaders})

	_, err := execute(t, "s2m", "delete", "gone", "--force", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
}

func TestDeleteSignalToMetricsRule_NotFoundWithoutForce(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: fixtureNotFound, Validator: testutil.RequireHeaders})

	// Agent mode skips the confirmation prompt without implying --force.
	prev := agentmode.Enabled
	agentmode.Enabled = true
	t.Cleanup(func() { agentmode.Enabled = prev })

	_, err := execute(t, "s2m", "delete", "gone", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- test ---

func TestTestSignalToMetricsRule_TableFormat(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiTestPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureTestSuccess, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "test", "-f", writeDoc(t, docWithOrigin), "--from", "now-1h", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "table")

	require.NoError(t, err)
	assert.Contains(t, out, `Signal-to-metrics rule "Checkout error logs" matched 1284 logs`)
	assert.Contains(t, out, "From: 2026-10-21T07:15:00Z")
	assert.Contains(t, out, "To: 2026-10-21T08:15:00Z")

	var request dash0api.SignalToMetricsTestRequest
	require.NoError(t, json.Unmarshal(findRequest(t, server.Requests(), http.MethodPost, apiTestPath).Body, &request))
	assert.Equal(t, testName, request.Definition.Spec.Display.Name)
	require.NotNil(t, request.TimeRange)
	from, err := json.Marshal(request.TimeRange.From)
	require.NoError(t, err)
	assert.JSONEq(t, `"now-1h"`, string(from))
	if request.Definition.Metadata.Labels != nil {
		assert.Nil(t, request.Definition.Metadata.Labels.Dash0Comdataset, "a dataset label would conflict with --dataset")
	}
}

func TestTestSignalToMetricsRule_JSONFormat(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiTestPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureTestSuccess, Validator: testutil.RequireHeaders})

	out, err := execute(t, "s2m", "test", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json")

	require.NoError(t, err)
	var result dash0api.SignalToMetricsTestResponse
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.EqualValues(t, 1284, result.MatchedCount)
}

func TestTestSignalToMetricsRule_InvalidDefinitionListsProblems(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiTestPath, testutil.MockResponse{StatusCode: http.StatusBadRequest, BodyFile: fixtureBadRequest, Validator: testutil.RequireHeaders})

	_, err := execute(t, "s2m", "test", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `signal-to-metrics rule "Checkout error logs" is not valid`)
	assert.Contains(t, err.Error(), "must be a valid metric name")
	assert.Contains(t, err.Error(), "Hint:")
}

func TestTestSignalToMetricsRule_RejectsUnknownOutputFormat(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	_, err := execute(t, "s2m", "test", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "valid formats: table, json")
	assert.Empty(t, server.Requests())
}

func TestSignalToMetricsRules_LongNameWorks(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListEmpty, Validator: testutil.RequireHeaders})

	_, err := execute(t, "signal-to-metrics-rules", "list", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
}
