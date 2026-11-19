//go:build integration

package evals

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAuthToken = "auth_test_token"
	apiPathEvals  = "/api/generative-ai-evaluations"
	evalOrigin    = "dash0-cli_answer-relevance"
	evalID        = "eval_01k9z3d4q5r6s7t8u9v0w1x2y3"
	evalID2       = "eval_01k9z3d4q5r6s7t8u9v0w1x2y4"
	pathByOrigin  = apiPathEvals + "/" + evalOrigin
	pathByID      = apiPathEvals + "/" + evalID

	evalBody = `kind: Dash0GenerativeAiEvaluation
metadata:
  name: answer-relevance
%s  annotations:
    dash0.com/folder-path: /checkout
spec:
  display:
    name: Answer relevance
    description: Judges whether the answer addresses the question.
  enabled: true
  judge:
    model: standard
    output:
      kind: boolean
    systemPrompt: You judge whether an answer is relevant to the question.
    userPrompt: "Question: {{ gen_ai.input.messages }}\nAnswer: {{ gen_ai.output.messages }}"
  match:
    filters:
      - key: gen_ai.operation.name
        operator: is_set
  sample:
    rate: 0.25
`
)

var evalPathPattern = regexp.MustCompile(`^/api/generative-ai-evaluations/[^/]+$`)

// evalYAML returns the eval document with the given metadata.labels block (or none).
func evalYAML(labels string) string {
	if labels == "" {
		return fmt.Sprintf(evalBody, "")
	}
	return fmt.Sprintf(evalBody, "  labels:\n"+labels)
}

// exportedEvalYAML is what `evals get -o yaml` prints: every server-set field is still attached.
var exportedEvalYAML = evalYAML(`    dash0.com/id: ` + evalID + `
    dash0.com/origin: ` + evalOrigin + `
    dash0.com/version: "3"
    dash0.com/source: dash0-cli
    dash0.com/dataset: default
`)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewEvalsCmd())
	return root
}

func runCmd(t *testing.T, serverURL string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetArgs(append(append([]string{"-X", "evals"}, args...), "--api-url", serverURL, "--auth-token", testAuthToken))
	stderr = testutil.CaptureStderr(t, func() {
		stdout = testutil.CaptureStdout(t, func() {
			err = cmd.Execute()
		})
	})
	return stdout, stderr, err
}

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eval.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func requestsFor(server *testutil.MockServer, method, path string) []testutil.RecordedRequest {
	var out []testutil.RecordedRequest
	for _, r := range server.Requests() {
		if r.Method == method && r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

func decodeLabels(t *testing.T, req testutil.RecordedRequest) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(req.Body, &body))
	labels, _ := body["metadata"].(map[string]any)["labels"].(map[string]any)
	return labels
}

func TestEvals_RequireExperimental(t *testing.T) {
	testutil.SetupTestEnv(t)
	for _, sub := range [][]string{{"list"}, {"get", evalID}, {"create", "-f", "x.yaml"}, {"update", "-f", "x.yaml"}, {"delete", evalID}} {
		cmd := newRootCmd()
		cmd.SetArgs(append([]string{"evals"}, sub...))
		err := cmd.Execute()
		require.Error(t, err, sub[0])
		assert.Contains(t, err.Error(), "experimental command", sub[0])
	}
}

func TestListEvals_Formats(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathEvals, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsListSuccess, Validator: testutil.RequireHeaders})

	out, _, err := runCmd(t, server.URL, "list")
	require.NoError(t, err)
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "ENABLED")
	assert.Contains(t, out, "Answer relevance")
	assert.Contains(t, out, "Toxicity score")
	assert.NotContains(t, out, "ORIGIN")

	out, _, err = runCmd(t, server.URL, "list", "-o", "wide")
	require.NoError(t, err)
	assert.Contains(t, out, "ORIGIN")
	assert.Contains(t, out, evalOrigin)

	out, _, err = runCmd(t, server.URL, "list", "-o", "csv")
	require.NoError(t, err)
	assert.Contains(t, out, "name,id,enabled,dataset,origin")
	assert.Contains(t, out, "Answer relevance,"+evalID+",true,default,"+evalOrigin)
	assert.Contains(t, out, "Toxicity score,"+evalID2+",false,default,")

	// --limit 1 keeps only the first eval.
	out, _, err = runCmd(t, server.URL, "list", "-o", "json", "--limit", "1")
	require.NoError(t, err)
	var items []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &items))
	require.Len(t, items, 1)
	assert.Equal(t, "Dash0GenerativeAiEvaluation", items[0]["kind"])

	out, _, err = runCmd(t, server.URL, "list", "-o", "yaml")
	require.NoError(t, err)
	assert.Contains(t, out, "kind: Dash0GenerativeAiEvaluation")
	assert.Contains(t, out, "\n---\n", "YAML output is a multi-document stream")
}

func TestListEvals_Empty(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathEvals, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsListEmpty, Validator: testutil.RequireHeaders})

	out, _, err := runCmd(t, server.URL, "list")
	require.NoError(t, err)
	assert.Contains(t, out, "No evals found.")
}

func TestListEvals_AuthError(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathEvals, testutil.MockResponse{StatusCode: http.StatusUnauthorized, BodyFile: "dashboards/error_unauthorized.json", Validator: testutil.RequireHeaders})

	_, _, err := runCmd(t, server.URL, "list")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestListEvals_DatasetQueryParam(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathEvals, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsListEmpty, Validator: testutil.RequireHeaders})

	_, _, err := runCmd(t, server.URL, "list", "--dataset", "staging")
	require.NoError(t, err)
	require.NotNil(t, server.LastRequest())
	assert.Contains(t, server.LastRequest().Query, "dataset=staging")
}

func TestGetEval_Formats(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsGetSuccess, Validator: testutil.RequireHeaders})

	out, _, err := runCmd(t, server.URL, "get", evalOrigin)
	require.NoError(t, err)
	assert.Contains(t, out, "Kind: Dash0GenerativeAiEvaluation")
	assert.Contains(t, out, "Name: Answer relevance")
	assert.Contains(t, out, "ID: "+evalID)
	assert.Contains(t, out, "Origin: "+evalOrigin)
	assert.Contains(t, out, "Sample rate: 0.25")
	assert.NotContains(t, out, "Deleted:")

	out, _, err = runCmd(t, server.URL, "get", evalOrigin, "-o", "yaml")
	require.NoError(t, err)
	assert.Contains(t, out, "kind: Dash0GenerativeAiEvaluation")
	assert.Contains(t, out, "name: answer-relevance")

	out, _, err = runCmd(t, server.URL, "get", evalOrigin, "-o", "json")
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, "Dash0GenerativeAiEvaluation", got["kind"])
}

func TestGetEval_Deleted(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, pathByID, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsGetDeleted, Validator: testutil.RequireHeaders})

	out, _, err := runCmd(t, server.URL, "get", evalID)
	require.NoError(t, err)
	assert.Contains(t, out, "Deleted: 2026-11-03 08:00:00")
}

func TestGetEval_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, evalPathPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: testutil.FixtureEvalsNotFound, Validator: testutil.RequireHeaders})

	_, _, err := runCmd(t, server.URL, "get", "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `Eval "missing" not found`)
}

// An origin PUT creates the eval when the origin is new, so no POST is needed,
// and the body never carries dash0.com/version or dash0.com/id.
func TestCreateEval_UpsertByOrigin_NewOrigin(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: testutil.FixtureEvalsNotFound, Validator: testutil.RequireHeaders})
	server.On(http.MethodPut, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsCreateSuccess, Validator: testutil.RequireHeaders})

	out, _, err := runCmd(t, server.URL, "create", "-f", writeYAML(t, exportedEvalYAML))
	require.NoError(t, err)
	assert.Contains(t, out, `Eval "Answer relevance" created`)

	puts := requestsFor(server, http.MethodPut, pathByOrigin)
	require.Len(t, puts, 1)
	assert.Empty(t, requestsFor(server, http.MethodPost, apiPathEvals), "origin input must never POST")

	labels := decodeLabels(t, puts[0])
	assert.NotContains(t, labels, "dash0.com/version", "the version must never be sent")
	assert.NotContains(t, labels, "dash0.com/id")
	assert.NotContains(t, labels, "dash0.com/source")
	assert.NotContains(t, labels, "dash0.com/dataset")
	assert.Equal(t, evalOrigin, labels["dash0.com/origin"], "the client strip keeps the origin in the body")
}

func TestCreateEval_UpsertByOrigin_Existing(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsGetSuccess, Validator: testutil.RequireHeaders})
	server.On(http.MethodPut, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsUpdateSuccess, Validator: testutil.RequireHeaders})

	out, _, err := runCmd(t, server.URL, "create", "-f", writeYAML(t, evalYAML("    dash0.com/origin: "+evalOrigin+"\n")))
	require.NoError(t, err)
	assert.Contains(t, out, `Eval "Answer relevance" updated`)
}

// Origins are unique per organization, so a PUT to an origin that another dataset
// uses answers 404 even though a PUT to a new origin creates the eval.
func TestCreateEval_UpsertByOrigin_OriginInAnotherDataset(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: testutil.FixtureEvalsNotFound, Validator: testutil.RequireHeaders})
	server.On(http.MethodPut, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: testutil.FixtureEvalsNotFound, Validator: testutil.RequireHeaders})

	_, _, err := runCmd(t, server.URL, "create", "-f", writeYAML(t, evalYAML("    dash0.com/origin: "+evalOrigin+"\n")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `eval origin "`+evalOrigin+`" already exists in another dataset`)
	assert.Contains(t, err.Error(), "\nHint: origins are unique per organization")
}

func TestCreateEval_UpsertByOrigin_SurfacesNon404PreflightError(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusUnauthorized, BodyFile: "dashboards/error_unauthorized.json", Validator: testutil.RequireHeaders})

	_, _, err := runCmd(t, server.URL, "create", "-f", writeYAML(t, evalYAML("    dash0.com/origin: "+evalOrigin+"\n")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
	assert.Empty(t, requestsFor(server, http.MethodPut, pathByOrigin))
}

// An id from another organization is not found, so the eval is created with POST,
// and the foreign id is not sent.
func TestCreateEval_UpsertByID_FallsBackToPOSTWhenNotFound(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, pathByID, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: testutil.FixtureEvalsNotFound, Validator: testutil.RequireHeaders})
	server.On(http.MethodPost, apiPathEvals, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsCreateSuccess, Validator: testutil.RequireHeaders})

	out, _, err := runCmd(t, server.URL, "create", "-f", writeYAML(t, evalYAML("    dash0.com/id: "+evalID+"\n")))
	require.NoError(t, err)
	assert.Contains(t, out, `Eval "Answer relevance" created`)

	posts := requestsFor(server, http.MethodPost, apiPathEvals)
	require.Len(t, posts, 1)
	assert.NotContains(t, decodeLabels(t, posts[0]), "dash0.com/id")
	assert.Empty(t, requestsFor(server, http.MethodPut, pathByID))
}

func TestCreateEval_UpsertByID_Existing(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, pathByID, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsGetSuccess, Validator: testutil.RequireHeaders})
	server.On(http.MethodPut, pathByID, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsUpdateSuccess, Validator: testutil.RequireHeaders})

	_, _, err := runCmd(t, server.URL, "create", "-f", writeYAML(t, evalYAML("    dash0.com/id: "+evalID+"\n")))
	require.NoError(t, err)
	require.Len(t, requestsFor(server, http.MethodPut, pathByID), 1)
	assert.Empty(t, requestsFor(server, http.MethodPost, apiPathEvals))
}

func TestCreateEval_RestoresDeleted(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsGetDeleted, Validator: testutil.RequireHeaders})
	server.On(http.MethodPut, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsUpdateSuccess, Validator: testutil.RequireHeaders})

	out, _, err := runCmd(t, server.URL, "create", "-f", writeYAML(t, evalYAML("    dash0.com/origin: "+evalOrigin+"\n")))
	require.NoError(t, err)
	assert.Contains(t, out, `Eval "Answer relevance" restored`)
}

func TestCreateEval_DryRun(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	out, _, err := runCmd(t, server.URL, "create", "-f", writeYAML(t, exportedEvalYAML), "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out, "Dry run: eval definition is valid")
	assert.Empty(t, server.Requests())
}

func TestCreateEval_InvalidDocumentFailsBeforeAnyRequest(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	doc := strings.Replace(evalYAML("    dash0.com/origin: "+evalOrigin+"\n"), "operator: is_set", "operator: is_not_set", 1)

	for _, args := range [][]string{{"create", "-f", writeYAML(t, doc), "--dry-run"}, {"create", "-f", writeYAML(t, doc)}, {"update", "-f", writeYAML(t, doc)}} {
		_, _, err := runCmd(t, server.URL, args...)
		require.Error(t, err, args)
		assert.Contains(t, err.Error(), "requires the filter {key: gen_ai.operation.name, operator: is_set}", args)
	}
	assert.Empty(t, server.Requests())
}

func TestUpdateEval_ExportedDocumentStripsServerFields(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsGetSuccess, Validator: testutil.RequireHeaders})
	server.On(http.MethodPut, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsUpdateSuccess, Validator: testutil.RequireHeaders})

	out, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, exportedEvalYAML))
	require.NoError(t, err)
	assert.Contains(t, out, "rate: 0.5", "the diff shows the change the server stored")

	puts := requestsFor(server, http.MethodPut, pathByOrigin)
	require.Len(t, puts, 1, "the update addresses the origin, not the id")
	labels := decodeLabels(t, puts[0])
	assert.NotContains(t, labels, "dash0.com/version", "the version must never be sent")
	assert.NotContains(t, labels, "dash0.com/id")
	var body map[string]any
	require.NoError(t, json.Unmarshal(puts[0].Body, &body))
	annotations := body["metadata"].(map[string]any)["annotations"].(map[string]any)
	assert.Equal(t, map[string]any{"dash0.com/folder-path": "/checkout"}, annotations)
}

func TestUpdateEval_UnchangedDocumentReportsNoChanges(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsGetSuccess, Validator: testutil.RequireHeaders})
	server.On(http.MethodPut, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsUpdateUnchanged, Validator: testutil.RequireHeaders})

	out, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, exportedEvalYAML))
	require.NoError(t, err)
	assert.Contains(t, out, `Eval "Answer relevance": no changes`)
	assert.NotContains(t, out, "dash0.com/version")
	assert.NotContains(t, out, "dash0.com/updated-at")
}

func TestUpdateEval_DryRunDoesNotWrite(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsGetSuccess, Validator: testutil.RequireHeaders})

	doc := evalYAML("    dash0.com/origin: " + evalOrigin + "\n")
	doc = regexp.MustCompile(`rate: 0.25`).ReplaceAllString(doc, "rate: 0.75")
	out, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, doc), "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out, "-    rate: 0.25")
	assert.Contains(t, out, "+    rate: 0.75")
	assert.Empty(t, requestsFor(server, http.MethodPut, pathByOrigin))
}

func TestUpdateEval_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, evalPathPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: testutil.FixtureEvalsNotFound, Validator: testutil.RequireHeaders})

	_, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, evalYAML("    dash0.com/origin: typo\n")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `Eval "typo" not found`)
	assert.Empty(t, requestsFor(server, http.MethodPut, apiPathEvals+"/typo"), "update must not create an eval at a mistyped origin")
}

func TestUpdateEval_ImmutableFieldErrorsAreExplained(t *testing.T) {
	for _, tc := range []struct {
		fixture, want, hint string
	}{
		{testutil.FixtureEvalsNameImmutable, "metadata.name cannot change after the eval is created", "metadata.name identifies the eval and its results"},
		{testutil.FixtureEvalsOutputKindImmutable, "spec.judge.output.kind cannot change after the eval is created", "results are stored per output kind"},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			testutil.SetupTestEnv(t)
			server := testutil.NewMockServer(t, testutil.FixturesDir())
			server.On(http.MethodGet, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureEvalsGetSuccess, Validator: testutil.RequireHeaders})
			server.On(http.MethodPut, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusBadRequest, BodyFile: tc.fixture, Validator: testutil.RequireHeaders})

			for _, args := range [][]string{{"update"}, {"create"}} {
				_, _, err := runCmd(t, server.URL, append(args, "-f", writeYAML(t, exportedEvalYAML))...)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.want, args[0])
				assert.Contains(t, err.Error(), "\nHint: "+tc.hint, args[0])
				assert.Contains(t, err.Error(), "Bad Request:", "the server's message is kept")
			}
		})
	}
}

func TestDeleteEval_Success(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodDelete, pathByOrigin, testutil.MockResponse{StatusCode: http.StatusOK, Validator: testutil.RequireHeaders})

	out, _, err := runCmd(t, server.URL, "delete", evalOrigin, "--force")
	require.NoError(t, err)
	assert.Contains(t, out, `Eval "`+evalOrigin+`" deleted`)
}

func TestDeleteEval_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodDelete, pathByID, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: testutil.FixtureEvalsNotFound, Validator: testutil.RequireHeaders})

	// With --force, a missing eval already holds the desired end state.
	out, stderr, err := runCmd(t, server.URL, "delete", evalID, "--force")
	require.NoError(t, err)
	assert.Contains(t, stderr, `Eval "`+evalID+`" was already deleted`)
	assert.NotContains(t, out, "deleted")

	// Without --force, the 404 is an error.
	restore := confirmation.SetReaderForTest(strings.NewReader("y\n"))
	defer restore()
	_, _, err = runCmd(t, server.URL, "delete", evalID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `Eval "`+evalID+`" not found`)
}
