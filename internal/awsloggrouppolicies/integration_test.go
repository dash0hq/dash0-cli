//go:build integration

package awsloggrouppolicies

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sigsyaml "sigs.k8s.io/yaml"
)

const (
	testAuthToken = "auth_test_token"
	integrationID = "27755ed0-0009-422d-8c11-19c6dd72df98"
	apiPath       = "/api/aws-log-group-instrumentation-policies/" + integrationID

	fixtureActive       = "awsloggrouppolicies/get_active.json"
	fixtureDeleted      = "awsloggrouppolicies/get_deleted.json"
	fixtureNotFound     = "awsloggrouppolicies/error_not_found.json"
	fixtureUnauthorized = "awsmetricscrapeconfigs/error_unauthorized.json"

	policyYAML = `apiVersion: dash0.com/v1alpha1
kind: Dash0AwsLogGroupInstrumentationPolicy
metadata:
  labels:
    dash0.com/id: ` + integrationID + `
spec:
  rules:
    - conditions:
        - key: dash0.cloud.tags.team
          operator: is_one_of
          values: [checkout, payments]
        - key: dash0.cloud.tags.env
          operator: is
          value: production
    - conditions:
        - key: dash0.cloud.tags.dash0-forward
          operator: is_set
`
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewAwsLogGroupPoliciesCmd())
	return root
}

func runCmd(t *testing.T, serverURL string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetArgs(append(append([]string{"-X", "aws-log-group-policies"}, args...), "--api-url", serverURL, "--auth-token", testAuthToken))
	stderr = testutil.CaptureStderr(t, func() {
		stdout = testutil.CaptureStdout(t, func() {
			err = cmd.Execute()
		})
	})
	return stdout, stderr, err
}

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "policy.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func newServer(t *testing.T) *testutil.MockServer {
	t.Helper()
	return testutil.NewMockServer(t, testutil.FixturesDir())
}

func respond(status int, fixture string) testutil.MockResponse {
	return testutil.MockResponse{StatusCode: status, BodyFile: fixture, Validator: testutil.RequireHeaders}
}

func requestsFor(server *testutil.MockServer, method string) []testutil.RecordedRequest {
	var out []testutil.RecordedRequest
	for _, r := range server.Requests() {
		if r.Method == method && r.Path == apiPath {
			out = append(out, r)
		}
	}
	return out
}

func TestAwsLogGroupPolicies_RequireExperimental(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"aws-log-group-policies", "get", integrationID, "--api-url", server.URL, "--auth-token", testAuthToken})
	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental")
	assert.Empty(t, server.Requests())
}

func TestGet_Table(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusOK, fixtureActive))

	stdout, _, err := runCmd(t, server.URL, "get", integrationID)

	require.NoError(t, err)
	assert.Contains(t, stdout, "Integration ID: "+integrationID)
	assert.Contains(t, stdout, "Revision: 3")
	assert.Contains(t, stdout, `1. dash0.cloud.tags.team is_one_of [checkout, payments] AND dash0.cloud.tags.env is "production"`)
	assert.Contains(t, stdout, "2. dash0.cloud.tags.dash0-forward is_set")
	assert.NotContains(t, stdout, "Deleted:")
}

func TestGet_DeletedShowsDisposition(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusOK, fixtureDeleted))

	stdout, _, err := runCmd(t, server.URL, "get", integrationID)

	require.NoError(t, err)
	assert.Contains(t, stdout, "Rules: none (matches no log group)")
	assert.Contains(t, stdout, "Deleted: 2026-11-18 11:00:00")
	assert.Contains(t, stdout, "Deletion disposition: keep")
}

func TestGet_YAMLAndJSON(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusOK, fixtureActive))

	yamlOut, _, err := runCmd(t, server.URL, "get", integrationID, "-o", "yaml")
	require.NoError(t, err)
	var fromYAML dash0api.AwsLogGroupInstrumentationPolicy
	require.NoError(t, sigsyaml.Unmarshal([]byte(yamlOut), &fromYAML))
	assert.Equal(t, integrationID, dash0api.GetAwsLogGroupInstrumentationPolicyID(&fromYAML))

	jsonOut, _, err := runCmd(t, server.URL, "get", integrationID, "-o", "json")
	require.NoError(t, err)
	var fromJSON dash0api.AwsLogGroupInstrumentationPolicy
	require.NoError(t, json.Unmarshal([]byte(jsonOut), &fromJSON))
	require.Len(t, fromJSON.Spec.Rules, 2)
}

func TestGet_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusNotFound, fixtureNotFound))

	_, _, err := runCmd(t, server.URL, "get", integrationID)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGet_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusUnauthorized, fixtureUnauthorized))

	_, _, err := runCmd(t, server.URL, "get", integrationID)

	require.Error(t, err)
}

// The CLI sends no metadata: no version, so the file wins over concurrent
// edits, and no origin, so it never conflicts with the owner's origin.
func TestUpdate_SendsSpecOnlyAndCreates(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusNotFound, fixtureNotFound))
	server.On(http.MethodPut, apiPath, respond(http.StatusOK, fixtureActive))

	stdout, stderr, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, policyYAML))

	require.NoError(t, err)
	assert.Equal(t, `AWS log-group policy of integration "`+integrationID+`" created`+"\n", stdout)
	assert.Empty(t, stderr)

	puts := requestsFor(server, http.MethodPut)
	require.Len(t, puts, 1)
	var sent map[string]any
	require.NoError(t, json.Unmarshal(puts[0].Body, &sent))
	assert.Equal(t, map[string]any{}, sent["metadata"], "the CLI must not send labels, version, or origin")
	spec := sent["spec"].(map[string]any)
	assert.Len(t, spec["rules"], 2)
}

func TestUpdate_DeletedPolicyIsRecreated(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusOK, fixtureDeleted))
	server.On(http.MethodPut, apiPath, respond(http.StatusOK, fixtureActive))

	stdout, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, policyYAML))

	require.NoError(t, err)
	assert.Contains(t, stdout, "created")
}

func TestUpdate_ExportedPolicyReappliesWithoutDiff(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusOK, fixtureActive))
	exported, err := os.ReadFile(filepath.Join(testutil.FixturesDir(), fixtureActive))
	require.NoError(t, err)

	stdout, stderr, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, string(exported)), "--dry-run")

	require.NoError(t, err)
	assert.Contains(t, stdout, "no changes")
	assert.Empty(t, stderr, "an exported document must not warn")
	assert.Empty(t, requestsFor(server, http.MethodPut))
}

func TestUpdate_DryRunShowsDiffWithoutPut(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusOK, fixtureActive))
	changed := strings.Replace(policyYAML, "value: production", "value: staging", 1)

	stdout, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, changed), "--dry-run")

	require.NoError(t, err)
	assert.Contains(t, stdout, "-      value: production")
	assert.Contains(t, stdout, "+      value: staging")
	assert.Empty(t, requestsFor(server, http.MethodPut))
}

func TestUpdate_ValidationFailsBeforeAnyAPICall(t *testing.T) {
	cases := map[string]struct {
		doc     string
		wantErr string
	}{
		"missing rules": {
			doc:     strings.Split(policyYAML, "spec:")[0] + "spec:\n  onNoLongerMatching: keep\n",
			wantErr: "spec.rules is required",
		},
		"unknown spec field": {
			doc:     policyYAML + "  ruless: []\n",
			wantErr: "invalid spec",
		},
		"unsupported operator": {
			doc:     strings.Replace(policyYAML, "operator: is_set", "operator: matches", 1),
			wantErr: `spec.rules[1].conditions[0].operator "matches" is not supported`,
		},
		"invalid disposition": {
			doc:     policyYAML + "  onNoLongerMatching: delete\n",
			wantErr: "spec.onNoLongerMatching must be",
		},
		"wrong kind": {
			doc:     strings.Replace(policyYAML, "Dash0AwsLogGroupInstrumentationPolicy", "Dash0AwsMetricScrapeConfig", 1),
			wantErr: "kind must be Dash0AwsLogGroupInstrumentationPolicy",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			testutil.SetupTestEnv(t)
			server := newServer(t)

			_, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, tc.doc))

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Empty(t, server.Requests())
		})
	}
}

func TestUpdate_PositionalIDMustMatchFile(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)

	_, _, err := runCmd(t, server.URL, "update", "00000000-0000-4000-8000-000000000000", "-f", writeYAML(t, policyYAML))

	require.Error(t, err)
	assert.Empty(t, server.Requests())
}

func TestDelete_DefaultKeepsForwarding(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodDelete, apiPath, respond(http.StatusOK, fixtureDeleted))

	stdout, _, err := runCmd(t, server.URL, "delete", integrationID, "--force")

	require.NoError(t, err)
	assert.Contains(t, stdout, "its log groups keep forwarding")
	deletes := requestsFor(server, http.MethodDelete)
	require.Len(t, deletes, 1)
	assert.Equal(t, "disposition=keep", deletes[0].Query)
}

func TestDelete_RemoveForwardingSendsRemove(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodDelete, apiPath, respond(http.StatusOK, fixtureDeleted))

	stdout, _, err := runCmd(t, server.URL, "delete", integrationID, "--force", "--remove-forwarding")

	require.NoError(t, err)
	assert.Contains(t, stdout, "forwarding is being removed")
	deletes := requestsFor(server, http.MethodDelete)
	require.Len(t, deletes, 1)
	assert.Equal(t, "disposition=remove", deletes[0].Query)
}

func TestDelete_PromptNamesRemoval(t *testing.T) {
	testutil.SetupTestEnv(t)
	t.Cleanup(confirmation.SetReaderForTest(strings.NewReader("n\n")))
	server := newServer(t)

	stdout, stderr, err := runCmd(t, server.URL, "delete", integrationID, "--remove-forwarding")

	require.NoError(t, err)
	assert.Contains(t, stdout+stderr, "remove forwarding from every log group it matched")
	assert.Contains(t, stdout, "Deletion cancelled")
	assert.Empty(t, server.Requests())
}

func TestDelete_ForceAlreadyGone(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodDelete, apiPath, respond(http.StatusNotFound, fixtureNotFound))

	stdout, stderr, err := runCmd(t, server.URL, "delete", integrationID, "--force")

	require.NoError(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "was already deleted")
}
