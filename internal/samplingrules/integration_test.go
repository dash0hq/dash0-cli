//go:build integration

package samplingrules

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
	apiPath       = "/api/sampling-rules"
	testAuthToken = "auth_test_token"

	fixtureListSuccess    = "samplingrules/list_success.json"
	fixtureListEmpty      = "samplingrules/list_empty.json"
	fixtureGetSuccess     = "samplingrules/get_success.json"
	fixtureCreateResponse = "samplingrules/create_response.json"
	fixtureUpdateResponse = "samplingrules/update_response.json"
	fixtureNotFound       = "samplingrules/error_not_found.json"
	fixtureWrongDataset   = "samplingrules/error_wrong_dataset.json"
	fixtureUnauthorized   = "dashboards/error_unauthorized.json"

	testOrigin    = "dash0-cli_checkout-errors"
	testID        = "4b7e2d91-3c5a-4f08-9e61-2a8d7c4f1b35"
	testCreatedID = "c2f81a47-9d3e-4b6c-8a05-7e1d4b9f2c60"
	testName      = "Keep checkout errors"
)

var originOrIDPattern = regexp.MustCompile(`^/api/sampling-rules/[^/]+$`)

// newRootCmd mirrors the real command tree. This command is not gated behind
// --experimental.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewSamplingRulesCmd())
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
    name: Keep checkout errors
  enabled: true
  conditions:
    kind: and
    spec:
      conditions:
        - kind: ottl
          spec:
            ottl: resource.attributes["service.name"] == "checkout"
        - kind: error
          spec: {}
  rateLimit:
    rate: 100
`

const docWithOrigin = `kind: Dash0Sampling
metadata:
  name: checkout-errors
  labels:
    dash0.com/origin: dash0-cli_checkout-errors
    dash0.com/dataset: default
` + docSpec

const docWithIDOnly = `kind: Dash0Sampling
metadata:
  name: checkout-errors
  labels:
    dash0.com/id: 4b7e2d91-3c5a-4f08-9e61-2a8d7c4f1b35
` + docSpec

const docWithOriginAndForeignID = `kind: Dash0Sampling
metadata:
  name: checkout-errors
  labels:
    dash0.com/origin: dash0-cli_checkout-errors
    dash0.com/id: 0e5d3b71-8a2c-4f9e-b6d4-1c7a9e3f5b08
` + docSpec

const docWithoutIdentifier = `kind: Dash0Sampling
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

func decodeRule(t *testing.T, body []byte) dash0api.SamplingDefinition {
	t.Helper()
	var rule dash0api.SamplingDefinition
	require.NoError(t, json.Unmarshal(body, &rule))
	return rule
}

func execute(t *testing.T, args ...string) (string, error) {
	t.Helper()
	out, _, err := executeWithStderr(t, args...)
	return out, err
}

func executeWithStderr(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetArgs(args)
	stderr = testutil.CaptureStderr(t, func() {
		stdout = testutil.CaptureStdout(t, func() {
			err = cmd.Execute()
		})
	})
	return stdout, stderr, err
}

// listWithEnabledRule registers a list whose first rule is enabled, so the
// turn-on check finds sampling already on and stays quiet.
func listWithEnabledRule(server *testutil.MockServer) {
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListSuccess, Validator: testutil.RequireHeaders})
}

// --- list ---

func TestListSamplingRules_TableFormat(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	listWithEnabledRule(server)

	out, err := execute(t, "sampling-rules", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "table")

	require.NoError(t, err)
	for _, want := range []string{"NAME", "ID", "ENABLED", "CONDITIONS", testName, testID, "and(ottl, error)", "Baseline", "probabilistic 0.05", "false"} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "ORIGIN", "origin is a wide-only column")
	assert.NotContains(t, out, "RATE LIMIT", "the rate limit is a wide-only column")
}

func TestListSamplingRules_WideFormat(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	listWithEnabledRule(server)

	out, err := execute(t, "sampling-rules", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "wide")

	require.NoError(t, err)
	for _, want := range []string{"RATE LIMIT", "DATASET", "ORIGIN", testOrigin, "100"} {
		assert.Contains(t, out, want)
	}
}

func TestListSamplingRules_CSVFormat(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	listWithEnabledRule(server)

	out, err := execute(t, "sampling-rules", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "csv")

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, "name,id,enabled,conditions,rate limit,dataset,origin", lines[0])
	assert.Equal(t, `Keep checkout errors,`+testID+`,true,"and(ottl, error)",100,default,`+testOrigin, lines[1])
	assert.Equal(t, "Baseline,9a3c5e7f-1b2d-4c6e-8f0a-3b5d7f9e1c24,false,probabilistic 0.05,,default,api-9a3c5e7f-1b2d-4c6e-8f0a-3b5d7f9e1c24", lines[2])
}

func TestListSamplingRules_JSONFormat(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	listWithEnabledRule(server)

	out, err := execute(t, "sampling-rules", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json")

	require.NoError(t, err)
	var rules []dash0api.SamplingDefinition
	require.NoError(t, json.Unmarshal([]byte(out), &rules))
	require.Len(t, rules, 2)
	assert.Equal(t, testName, rules[0].Spec.Display.Name)
	assert.False(t, rules[1].Spec.Enabled)
}

func TestListSamplingRules_YAMLFormatIsMultiDocument(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	listWithEnabledRule(server)

	out, err := execute(t, "sampling-rules", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml")

	require.NoError(t, err)
	assert.Contains(t, out, "kind: Dash0Sampling")
	assert.Contains(t, out, "\n---\n")
}

func TestListSamplingRules_AgentModeDefaultsToJSON(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	listWithEnabledRule(server)
	prev := agentmode.Enabled
	agentmode.Enabled = true
	t.Cleanup(func() { agentmode.Enabled = prev })

	out, err := execute(t, "sampling-rules", "list", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	var rules []dash0api.SamplingDefinition
	require.NoError(t, json.Unmarshal([]byte(out), &rules))
}

func TestListSamplingRules_Limit(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	listWithEnabledRule(server)

	out, err := execute(t, "sampling-rules", "list", "--limit", "1", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "csv", "--skip-header")

	require.NoError(t, err)
	assert.Len(t, strings.Split(strings.TrimSpace(out), "\n"), 1)
}

func TestListSamplingRules_DatasetIsSentAsQueryParameter(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListEmpty, Validator: testutil.RequireHeaders})

	_, err := execute(t, "sampling-rules", "list", "--dataset", "staging", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json")

	require.NoError(t, err)
	query, err := url.ParseQuery(findRequest(t, server.Requests(), http.MethodGet, apiPath).Query)
	require.NoError(t, err)
	assert.Equal(t, "staging", query.Get("dataset"))
}

// Without a dataset parameter the list endpoint returns the rules of every
// dataset, so the default dataset must be named explicitly.
func TestListSamplingRules_DefaultDatasetIsSentExplicitly(t *testing.T) {
	for _, args := range [][]string{nil, {"--dataset", "default"}} {
		testutil.SetupTestEnv(t)
		server := testutil.NewMockServer(t, testutil.FixturesDir())
		server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListEmpty, Validator: testutil.RequireHeaders})

		_, err := execute(t, append([]string{"sampling-rules", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json"}, args...)...)

		require.NoError(t, err)
		query, err := url.ParseQuery(findRequest(t, server.Requests(), http.MethodGet, apiPath).Query)
		require.NoError(t, err)
		assert.Equal(t, "default", query.Get("dataset"), "args: %v", args)
	}
}

func TestListSamplingRules_Empty(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListEmpty, Validator: testutil.RequireHeaders})

	out, err := execute(t, "sampling-rules", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "table")

	require.NoError(t, err)
	assert.Contains(t, out, "No sampling rules found.")
}

func TestListSamplingRules_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusUnauthorized, BodyFile: fixtureUnauthorized, Validator: testutil.RequireHeaders})

	_, err := execute(t, "sampling-rules", "list", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

// --- get ---

func TestGetSamplingRule_Success(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders})

	out, err := execute(t, "sampling-rules", "get", testOrigin, "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "table")

	require.NoError(t, err)
	for _, want := range []string{
		"Kind: Sampling rule", "Name: " + testName, "ID: " + testID, "Dataset: default", "Origin: " + testOrigin,
		"Enabled: true", "Conditions: and(ottl, error)", "Rate limit: 100 traces per minute",
	} {
		assert.Contains(t, out, want)
	}
	findRequest(t, server.Requests(), http.MethodGet, apiPath+"/"+testOrigin)
}

func TestGetSamplingRule_YAMLRoundTripsIdentifiers(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders})

	out, err := execute(t, "sampling-rules", "get", testID, "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml")

	require.NoError(t, err)
	assert.Contains(t, out, "kind: Dash0Sampling")
	assert.Contains(t, out, "dash0.com/origin: "+testOrigin)
	assert.Contains(t, out, "dash0.com/id: "+testID)
	assert.Contains(t, out, `ottl: resource.attributes["service.name"] == "checkout"`)
}

func TestGetSamplingRule_JSONFormat(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders})

	out, err := execute(t, "sampling-rules", "get", testID, "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json")

	require.NoError(t, err)
	rule := decodeRule(t, []byte(out))
	assert.Equal(t, testID, dash0api.GetSamplingRuleID(&rule))
}

func TestGetSamplingRule_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: fixtureNotFound, Validator: testutil.RequireHeaders})

	_, err := execute(t, "sampling-rules", "get", "missing", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetSamplingRule_InAnotherDataset(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusBadRequest, BodyFile: fixtureWrongDataset, Validator: testutil.RequireHeaders})

	_, err := execute(t, "sampling-rules", "get", testOrigin, "--dataset", "staging", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `sampling rule "`+testOrigin+`" already exists in another dataset`)
	assert.Contains(t, err.Error(), "Hint: origins and IDs are unique per organization")
}

// --- create ---

func TestCreateSamplingRule_ByOrigin_Creates(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	listWithEnabledRule(server)
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: fixtureNotFound, Validator: testutil.RequireHeaders})
	server.OnPattern(http.MethodPut, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders})

	out, err := execute(t, "sampling-rules", "create", "-f", writeDoc(t, docWithOrigin), "--dataset", "staging", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, `Sampling rule "Keep checkout errors" created (id: `+testID+`)`)
	put := findRequest(t, server.Requests(), http.MethodPut, apiPath+"/"+testOrigin)
	assert.Zero(t, countMethod(server.Requests(), http.MethodPost), "an origin document must never POST")

	query, err := url.ParseQuery(put.Query)
	require.NoError(t, err)
	assert.Equal(t, "staging", query.Get("dataset"))

	// The server takes the origin from the path and the dataset from the query.
	// A dataset label in the body would win over --dataset.
	body := decodeRule(t, put.Body)
	require.NotNil(t, body.Metadata.Labels)
	assert.Nil(t, body.Metadata.Labels.Dash0Comorigin)
	assert.Nil(t, body.Metadata.Labels.Dash0Comdataset)
	assert.Nil(t, body.Metadata.Labels.Dash0Comid)
	assert.True(t, body.Spec.Enabled)
	require.NotNil(t, body.Spec.RateLimit)
	assert.Equal(t, 100, body.Spec.RateLimit.Rate)
}

// A document exported from another organization carries that organization's
// id next to the origin. The origin in the path selects the rule, so the id
// must not be sent.
func TestCreateSamplingRule_ByOrigin_DoesNotSendID(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders})
	server.OnPattern(http.MethodPut, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureUpdateResponse, Validator: testutil.RequireHeaders})

	out, err := execute(t, "sampling-rules", "create", "-f", writeDoc(t, docWithOriginAndForeignID), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "updated")
	body := decodeRule(t, findRequest(t, server.Requests(), http.MethodPut, apiPath+"/"+testOrigin).Body)
	if body.Metadata.Labels != nil {
		assert.Nil(t, body.Metadata.Labels.Dash0Comid)
	}
}

// The GET honors --dataset, so a rule stored in another dataset fails the
// preflight with the cross-dataset 400, and nothing is written.
func TestCreateSamplingRule_OriginInAnotherDatasetFailsBeforeTheWrite(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusBadRequest, BodyFile: fixtureWrongDataset, Validator: testutil.RequireHeaders})

	_, err := execute(t, "sampling-rules", "create", "-f", writeDoc(t, docWithOrigin), "--dataset", "staging", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `sampling rule "`+testOrigin+`" already exists in another dataset`)
	assert.Contains(t, err.Error(), "Hint: origins and IDs are unique per organization")
	assert.Equal(t, []string{"GET " + apiPath + "/" + testOrigin}, describe(server.Requests()), "nothing is written")
}

func TestCreateSamplingRule_IDInAnotherDatasetFailsBeforeTheWrite(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusBadRequest, BodyFile: fixtureWrongDataset, Validator: testutil.RequireHeaders})

	_, err := execute(t, "sampling-rules", "create", "-f", writeDoc(t, docWithIDOnly), "--dataset", "staging", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `sampling rule "`+testID+`" already exists in another dataset`)
	assert.Equal(t, []string{"GET " + apiPath + "/" + testID}, describe(server.Requests()), "nothing is written")
}

// A concurrent write can still put the rule in another dataset between the
// preflight and the PUT, so the PUT's 400 is explained too.
func TestCreateSamplingRule_WrongDatasetOnTheWriteIsExplained(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	listWithEnabledRule(server)
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: fixtureNotFound, Validator: testutil.RequireHeaders})
	server.OnPattern(http.MethodPut, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusBadRequest, BodyFile: fixtureWrongDataset, Validator: testutil.RequireHeaders})

	_, err := execute(t, "sampling-rules", "create", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists in another dataset")
}

func TestCreateSamplingRule_ByIDOnly_ExistingRuleIsUpdatedInPlace(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders})
	server.OnPattern(http.MethodPut, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureUpdateResponse, Validator: testutil.RequireHeaders})

	out, err := execute(t, "sampling-rules", "create", "-f", writeDoc(t, docWithIDOnly), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "updated")
	body := decodeRule(t, findRequest(t, server.Requests(), http.MethodPut, apiPath+"/"+testID).Body)
	require.NotNil(t, body.Metadata.Labels)
	require.NotNil(t, body.Metadata.Labels.Dash0Comid)
	assert.Equal(t, testID, *body.Metadata.Labels.Dash0Comid, "a PUT to the id may carry the same id")
	assert.Zero(t, countMethod(server.Requests(), http.MethodPost))
}

// IDs are unique across organizations, so a PUT to an id another
// organization owns fails. A missing id falls back to POST without the id,
// which the server rejects in a POST body.
func TestCreateSamplingRule_ByIDOnly_UnknownIDFallsBackToPOSTWithoutID(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	listWithEnabledRule(server)
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: fixtureNotFound, Validator: testutil.RequireHeaders})
	server.On(http.MethodPost, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureCreateResponse, Validator: testutil.RequireHeaders})

	out, err := execute(t, "sampling-rules", "create", "-f", writeDoc(t, docWithIDOnly), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "created (id: "+testCreatedID+")")
	assert.Zero(t, countMethod(server.Requests(), http.MethodPut), "an unknown id must never be PUT")
	body := decodeRule(t, findRequest(t, server.Requests(), http.MethodPost, apiPath).Body)
	if body.Metadata.Labels != nil {
		assert.Nil(t, body.Metadata.Labels.Dash0Comid)
	}
}

func TestCreateSamplingRule_ByIDOnly_SurfacesNon404PreflightError(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusUnauthorized, BodyFile: fixtureUnauthorized, Validator: testutil.RequireHeaders})

	_, err := execute(t, "sampling-rules", "create", "-f", writeDoc(t, docWithIDOnly), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Zero(t, countMethod(server.Requests(), http.MethodPost), "a failed preflight must not create a duplicate")
	assert.Zero(t, countMethod(server.Requests(), http.MethodPut))
}

func TestCreateSamplingRule_WithoutIdentifierPOSTs(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	listWithEnabledRule(server)
	server.On(http.MethodPost, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureCreateResponse, Validator: testutil.RequireHeaders})

	out, err := execute(t, "sampling-rules", "create", "-f", writeDoc(t, docWithoutIdentifier), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "created")
	assert.Equal(t, []string{"GET " + apiPath, "POST " + apiPath}, describe(server.Requests()), "only the turn-on check reads")
}

// --- create: turning sampling on ---

func TestCreateSamplingRule_FirstEnabledRuleWarns(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListEmpty, Validator: testutil.RequireHeaders})
	server.On(http.MethodPost, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureCreateResponse, Validator: testutil.RequireHeaders})

	out, stderr, err := executeWithStderr(t, "sampling-rules", "create", "-f", writeDoc(t, docWithoutIdentifier), "--dataset", "staging", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "created")
	assert.Contains(t, stderr, `warning: sampling rule "Keep checkout errors" is the first enabled sampling rule in dataset "staging", so it turns sampling on`)
	assert.NotContains(t, out, "warning", "warnings go to stderr")
}

// The turn-on check must list only the default dataset. A list without the
// dataset parameter would return the enabled rules of other datasets and
// silence the warning.
func TestCreateSamplingRule_FirstEnabledRuleInDefaultDatasetListsOnlyThatDataset(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListEmpty, Validator: testutil.RequireHeaders})
	server.On(http.MethodPost, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureCreateResponse, Validator: testutil.RequireHeaders})

	_, stderr, err := executeWithStderr(t, "sampling-rules", "create", "-f", writeDoc(t, docWithoutIdentifier), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	query, err := url.ParseQuery(findRequest(t, server.Requests(), http.MethodGet, apiPath).Query)
	require.NoError(t, err)
	assert.Equal(t, "default", query.Get("dataset"))
	assert.Contains(t, stderr, `is the first enabled sampling rule in dataset "default"`)
}

func TestCreateSamplingRule_DisabledRuleDoesNotWarnOrList(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureCreateResponse, Validator: testutil.RequireHeaders})

	disabled := strings.Replace(docWithoutIdentifier, "enabled: true", "enabled: false", 1)
	_, stderr, err := executeWithStderr(t, "sampling-rules", "create", "-f", writeDoc(t, disabled), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.NotContains(t, stderr, "warning")
	assert.Equal(t, []string{"POST " + apiPath}, describe(server.Requests()))
	body := decodeRule(t, findRequest(t, server.Requests(), http.MethodPost, apiPath).Body)
	assert.False(t, body.Spec.Enabled)
}

// --- create: local validation ---

func TestCreateSamplingRule_MissingEnabledFailsBeforeAnyRequest(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	doc := strings.Replace(docWithOrigin, "  enabled: true\n", "", 1)
	_, err := execute(t, "sampling-rules", "create", "-f", writeDoc(t, doc), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires spec.enabled")
	assert.Empty(t, server.Requests())
}

func TestCreateSamplingRule_TooManyConditionsFailBeforeAnyRequest(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	var b strings.Builder
	b.WriteString("kind: Dash0Sampling\nmetadata:\n  name: noisy\nspec:\n  enabled: false\n  conditions:\n    kind: and\n    spec:\n      conditions:\n")
	for i := 0; i < 33; i++ {
		b.WriteString("        - kind: error\n          spec: {}\n")
	}
	_, err := execute(t, "sampling-rules", "create", "-f", writeDoc(t, b.String()), "--dry-run", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than the limit of 32")
	assert.Empty(t, server.Requests())
}

func TestCreateSamplingRule_DryRunMakesNoRequests(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	out, err := execute(t, "sampling-rules", "create", "-f", writeDoc(t, docWithOrigin), "--dry-run", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "Dry run: sampling rule definition is valid")
	assert.Empty(t, server.Requests())
}

// --- update ---

func TestUpdateSamplingRule_UnchangedDocumentReportsNoChanges(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders})
	// The response bumps the version, as the real server does.
	server.OnPattern(http.MethodPut, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureUpdateResponse, Validator: testutil.RequireHeaders})

	out, err := execute(t, "sampling-rules", "update", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, `Sampling rule "Keep checkout errors": no changes`)
	body := decodeRule(t, findRequest(t, server.Requests(), http.MethodPut, apiPath+"/"+testOrigin).Body)
	require.NotNil(t, body.Metadata.Labels)
	assert.Nil(t, body.Metadata.Labels.Dash0Comdataset, "a dataset label would win over --dataset")
	assert.Nil(t, body.Metadata.Labels.Dash0Comorigin)
}

func TestUpdateSamplingRule_ByOrigin_DoesNotSendID(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders})
	server.OnPattern(http.MethodPut, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureUpdateResponse, Validator: testutil.RequireHeaders})

	_, err := execute(t, "sampling-rules", "update", "-f", writeDoc(t, docWithOriginAndForeignID), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	body := decodeRule(t, findRequest(t, server.Requests(), http.MethodPut, apiPath+"/"+testOrigin).Body)
	if body.Metadata.Labels != nil {
		assert.Nil(t, body.Metadata.Labels.Dash0Comid)
	}
}

func TestUpdateSamplingRule_ByID_KeepsID(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders})
	server.OnPattern(http.MethodPut, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureUpdateResponse, Validator: testutil.RequireHeaders})

	_, err := execute(t, "sampling-rules", "update", "-f", writeDoc(t, docWithIDOnly), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	body := decodeRule(t, findRequest(t, server.Requests(), http.MethodPut, apiPath+"/"+testID).Body)
	require.NotNil(t, body.Metadata.Labels)
	require.NotNil(t, body.Metadata.Labels.Dash0Comid)
	assert.Equal(t, testID, *body.Metadata.Labels.Dash0Comid)
}

func TestUpdateSamplingRule_ChangedFieldShowsDiff(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders})

	changed := strings.Replace(docWithOrigin, "rate: 100", "rate: 250", 1)
	out, err := execute(t, "sampling-rules", "update", "-f", writeDoc(t, changed), "--dry-run", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "-    rate: 100")
	assert.Contains(t, out, "+    rate: 250")
	assert.Zero(t, countMethod(server.Requests(), http.MethodPut), "--dry-run must not write")
}

// Enabling a disabled rule when no other rule is enabled turns sampling on,
// so update warns, also with --dry-run.
func TestUpdateSamplingRule_EnablingTheOnlyRuleWarns(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	stored := map[string]any{
		"kind": "Dash0Sampling",
		"metadata": map[string]any{
			"name":   "checkout-errors",
			"labels": map[string]any{"dash0.com/id": testID, "dash0.com/origin": testOrigin, "dash0.com/dataset": "default"},
		},
		"spec": map[string]any{
			"display":    map[string]any{"name": testName},
			"enabled":    false,
			"conditions": map[string]any{"kind": "error", "spec": map[string]any{}},
		},
	}
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, Body: stored, Validator: testutil.RequireHeaders})
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, Body: map[string]any{"samplingRules": []any{stored}}, Validator: testutil.RequireHeaders})

	out, stderr, err := executeWithStderr(t, "sampling-rules", "update", "-f", writeDoc(t, docWithOrigin), "--dry-run", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "-  enabled: false")
	assert.Contains(t, stderr, "is the first enabled sampling rule in dataset \"default\", so it turns sampling on")
	assert.Zero(t, countMethod(server.Requests(), http.MethodPut))
}

func TestUpdateSamplingRule_MissingRuleFailsInsteadOfCreating(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: fixtureNotFound, Validator: testutil.RequireHeaders})

	_, err := execute(t, "sampling-rules", "update", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.Zero(t, countMethod(server.Requests(), http.MethodPut), "the PUT endpoint upserts, so update must stop before it")
}

func TestUpdateSamplingRule_DryRunInAnotherDatasetFails(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusBadRequest, BodyFile: fixtureWrongDataset, Validator: testutil.RequireHeaders})

	out, err := execute(t, "sampling-rules", "update", "-f", writeDoc(t, docWithOrigin), "--dry-run", "--dataset", "staging", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists in another dataset")
	assert.NotContains(t, out, "---", "no diff is printed")
	assert.Equal(t, []string{"GET " + apiPath + "/" + testOrigin}, describe(server.Requests()), "nothing is written")
}

func TestUpdateSamplingRule_ArgumentMustMatchFile(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	_, err := execute(t, "sampling-rules", "update", "some-other-origin", "-f", writeDoc(t, docWithOrigin), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match the file's origin")
	assert.Empty(t, server.Requests())
}

func TestUpdateSamplingRule_NoIdentifierFails(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	_, err := execute(t, "sampling-rules", "update", "-f", writeDoc(t, docWithoutIdentifier), "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no sampling rule origin or id given")
	assert.Empty(t, server.Requests())
}

// --- delete ---

func TestDeleteSamplingRule_Success(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	// The live API answers a delete with 200 and an empty body.
	server.OnPattern(http.MethodDelete, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, Validator: testutil.RequireHeaders})

	out, err := execute(t, "sampling-rules", "delete", testOrigin, "--force", "--dataset", "staging", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, `Sampling rule "`+testOrigin+`" deleted`)
	del := findRequest(t, server.Requests(), http.MethodDelete, apiPath+"/"+testOrigin)
	query, err := url.ParseQuery(del.Query)
	require.NoError(t, err)
	assert.Equal(t, "staging", query.Get("dataset"))
}

func TestDeleteSamplingRule_AlreadyDeletedWithForce(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: fixtureNotFound, Validator: testutil.RequireHeaders})

	_, stderr, err := executeWithStderr(t, "sampling-rules", "delete", "gone", "--force", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, stderr, "already deleted")
}

// The rule exists in another dataset, so --force must not report it as
// already deleted.
func TestDeleteSamplingRule_InAnotherDatasetFailsEvenWithForce(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusBadRequest, BodyFile: fixtureWrongDataset, Validator: testutil.RequireHeaders})

	_, err := execute(t, "sampling-rules", "delete", testOrigin, "--force", "--dataset", "staging", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists in another dataset")
}

func TestDeleteSamplingRule_NotFoundWithoutForce(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, originOrIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: fixtureNotFound, Validator: testutil.RequireHeaders})

	// Agent mode skips the confirmation prompt without implying --force.
	prev := agentmode.Enabled
	agentmode.Enabled = true
	t.Cleanup(func() { agentmode.Enabled = prev })

	_, err := execute(t, "sampling-rules", "delete", "gone", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestSamplingRules_IsNotExperimental(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListEmpty, Validator: testutil.RequireHeaders})

	_, err := execute(t, "sampling-rules", "list", "--api-url", server.URL, "--auth-token", testAuthToken)
	require.NoError(t, err)

	_, err = execute(t, "-X", "sampling-rules", "list", "--api-url", server.URL, "--auth-token", testAuthToken)
	require.NoError(t, err, "-X is accepted and ignored")
}
