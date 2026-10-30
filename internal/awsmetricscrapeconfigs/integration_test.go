//go:build integration

package awsmetricscrapeconfigs

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
	apiPath       = "/api/aws-metric-scrape-configs/" + integrationID

	fixtureConfigured   = "awsmetricscrapeconfigs/get_configured.json"
	fixtureUnconfigured = "awsmetricscrapeconfigs/get_unconfigured.json"
	fixtureNotFound     = "awsmetricscrapeconfigs/error_not_found.json"
	fixtureUnauthorized = "awsmetricscrapeconfigs/error_unauthorized.json"

	configYAML = `apiVersion: dash0.com/v1alpha1
kind: Dash0AwsMetricScrapeConfig
metadata:
  name: production-account
  labels:
    dash0.com/id: ` + integrationID + `
spec:
  namespaces:
    - namespace: AWS/S3
      metrics:
        - BucketSizeBytes
    - namespace: AWS/Lambda
      metrics:
        - Invocations
        - Errors
        - Duration
      filter:
        - key: aws.lambda.function.name
          operator: starts_with
          value: checkout-
`
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewAwsMetricScrapeConfigsCmd())
	return root
}

func runCmd(t *testing.T, serverURL string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetArgs(append(append([]string{"-X", "aws-metric-scrape-configs"}, args...), "--api-url", serverURL, "--auth-token", testAuthToken))
	stderr = testutil.CaptureStderr(t, func() {
		stdout = testutil.CaptureStdout(t, func() {
			err = cmd.Execute()
		})
	})
	return stdout, stderr, err
}

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scrape-config.yaml")
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

func TestAwsMetricScrapeConfigs_RequireExperimental(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"aws-metric-scrape-configs", "get", integrationID, "--api-url", server.URL, "--auth-token", testAuthToken})
	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental")
	assert.Empty(t, server.Requests())
}

func TestGet_Table(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusOK, fixtureConfigured))

	stdout, _, err := runCmd(t, server.URL, "get", integrationID)

	require.NoError(t, err)
	assert.Contains(t, stdout, "Kind: AWS metric scrape config")
	assert.Contains(t, stdout, "Integration: production-account")
	assert.Contains(t, stdout, "Integration ID: "+integrationID)
	assert.Contains(t, stdout, "AWS/Lambda (3 metrics, filtered)")
	assert.Contains(t, stdout, "AWS/S3 (1 metric)")
}

func TestGet_Unconfigured(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusOK, fixtureUnconfigured))

	stdout, _, err := runCmd(t, server.URL, "get", integrationID)

	require.NoError(t, err)
	assert.Contains(t, stdout, "Namespaces: none (nothing is scraped)")
	assert.NotContains(t, stdout, "Origin:")
}

func TestGet_YAMLAndJSON(t *testing.T) {
	for _, format := range []string{"yaml", "json"} {
		t.Run(format, func(t *testing.T) {
			testutil.SetupTestEnv(t)
			server := newServer(t)
			server.On(http.MethodGet, apiPath, respond(http.StatusOK, fixtureConfigured))

			stdout, _, err := runCmd(t, server.URL, "get", integrationID, "-o", format)

			require.NoError(t, err)
			var cfg dash0api.AwsMetricScrapeConfigDefinition
			require.NoError(t, sigsyaml.Unmarshal([]byte(stdout), &cfg))
			assert.Equal(t, "Dash0AwsMetricScrapeConfig", string(cfg.Kind))
			assert.Equal(t, integrationID, dash0api.GetAwsMetricScrapeConfigID(&cfg))
			require.Len(t, cfg.Spec.Namespaces, 2)
			assert.Equal(t, "AWS/Lambda", cfg.Spec.Namespaces[0].Namespace)
		})
	}
}

func TestGet_InvalidFormatMakesNoAPICall(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)

	_, _, err := runCmd(t, server.URL, "get", integrationID, "-o", "wide")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown output format")
	assert.Empty(t, server.Requests())
}

func TestGet_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusNotFound, fixtureNotFound))

	stdout, _, err := runCmd(t, server.URL, "get", integrationID)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `AWS integration "`+integrationID+`" not found`)
	assert.Empty(t, stdout)
}

func TestGet_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusUnauthorized, fixtureUnauthorized))

	stdout, _, err := runCmd(t, server.URL, "get", integrationID)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
	assert.Empty(t, stdout)
}

func TestGet_Forbidden(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusForbidden, Body: json.RawMessage(`{"error":{"code":403,"message":"Forbidden"}}`)})

	_, _, err := runCmd(t, server.URL, "get", integrationID)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestGet_MalformedResponseNamesTheField(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, Body: json.RawMessage(`{"apiVersion":"dash0.com/v1alpha1","kind":"Dash0AwsMetricScrapeConfig","metadata":{},"spec":{"namespaces":"AWS/S3"}}`)})

	stdout, _, err := runCmd(t, server.URL, "get", integrationID)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespaces")
	assert.Empty(t, stdout)
}

func TestGet_InvalidIntegrationIDMakesNoAPICall(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)

	_, _, err := runCmd(t, server.URL, "get", "not-a-uuid")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid UUID")
	assert.Empty(t, server.Requests())
}

func TestUpdate_ReplacesConfigAndShowsDiff(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusOK, fixtureUnconfigured))
	server.On(http.MethodPut, apiPath, respond(http.StatusOK, fixtureConfigured))

	stdout, stderr, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, configYAML))

	require.NoError(t, err)
	assert.Contains(t, stdout, `AWS metric scrape config "production-account" created (integration id: `+integrationID+`)`)
	assert.Empty(t, stderr, "an exported-shape document must not warn")

	puts := requestsFor(server, http.MethodPut)
	require.Len(t, puts, 1)
	var sent dash0api.AwsMetricScrapeConfigDefinition
	require.NoError(t, json.Unmarshal(puts[0].Body, &sent))
	assert.Equal(t, "dash0.com/v1alpha1", string(sent.ApiVersion))
	assert.Nil(t, sent.Metadata.Labels, "the CLI must not send an origin or other labels")
	assert.Nil(t, sent.Metadata.Name)
	require.Len(t, sent.Spec.Namespaces, 2)
	assert.Equal(t, "AWS/S3", sent.Spec.Namespaces[0].Namespace)
	require.NotNil(t, sent.Spec.Namespaces[1].Filter)
}

// Re-applying what the server returned must report no changes, even though
// the server sorts namespaces and metrics.
func TestUpdate_ExportedConfigReappliesWithoutDiff(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusOK, fixtureConfigured))
	server.On(http.MethodPut, apiPath, respond(http.StatusOK, fixtureConfigured))

	exported, _, err := runCmd(t, server.URL, "get", integrationID, "-o", "yaml")
	require.NoError(t, err)

	stdout, stderr, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, exported))

	require.NoError(t, err)
	assert.Contains(t, stdout, `AWS metric scrape config "production-account": no changes`)
	assert.Empty(t, stderr)
}

func TestUpdate_DryRunShowsDiffWithoutPut(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusOK, fixtureConfigured))

	doc := `apiVersion: dash0.com/v1alpha1
kind: Dash0AwsMetricScrapeConfig
metadata:
  labels:
    dash0.com/id: ` + integrationID + `
spec:
  namespaces:
    - namespace: AWS/S3
      metrics: [BucketSizeBytes, NumberOfObjects]
`
	stdout, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, doc), "--dry-run")

	require.NoError(t, err)
	assert.Contains(t, stdout, "-  - filter:")
	assert.Contains(t, stdout, "+    - NumberOfObjects")
	assert.NotContains(t, stdout, "origin", "the origin is not written, so it must not show up as a change")
	assert.Empty(t, requestsFor(server, http.MethodPut))
}

func TestUpdate_PositionalIDMustMatchFile(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)

	_, _, err := runCmd(t, server.URL, "update", "11111111-1111-4111-8111-111111111111", "-f", writeYAML(t, configYAML))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match the file's id")
	assert.Empty(t, server.Requests())
}

func TestUpdate_NoIntegrationID(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	doc := `apiVersion: dash0.com/v1alpha1
kind: Dash0AwsMetricScrapeConfig
spec:
  namespaces: []
`

	_, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, doc))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no AWS integration id given")
	assert.Empty(t, server.Requests())
}

func TestUpdate_PositionalIDWhenFileHasNone(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusOK, fixtureConfigured))
	server.On(http.MethodPut, apiPath, respond(http.StatusOK, fixtureUnconfigured))
	doc := `apiVersion: dash0.com/v1alpha1
kind: Dash0AwsMetricScrapeConfig
spec:
  namespaces: []
`

	_, _, err := runCmd(t, server.URL, "update", integrationID, "-f", writeYAML(t, doc))

	require.NoError(t, err)
	require.Len(t, requestsFor(server, http.MethodPut), 1)
}

func TestUpdate_ValidationFailsBeforeAnyAPICall(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		wantErr string
	}{
		{
			name:    "missing namespaces",
			doc:     "apiVersion: dash0.com/v1alpha1\nkind: Dash0AwsMetricScrapeConfig\nspec:\n  namespace:\n    - namespace: AWS/S3\n      metrics: [BucketSizeBytes]\n",
			wantErr: "spec.namespaces is required",
		},
		{
			name:    "null namespaces",
			doc:     "apiVersion: dash0.com/v1alpha1\nkind: Dash0AwsMetricScrapeConfig\nspec:\n  namespaces:\n",
			wantErr: "spec.namespaces is required",
		},
		{
			name:    "missing spec",
			doc:     "apiVersion: dash0.com/v1alpha1\nkind: Dash0AwsMetricScrapeConfig\n",
			wantErr: "spec.namespaces is required",
		},
		{
			name:    "unknown spec field",
			doc:     "apiVersion: dash0.com/v1alpha1\nkind: Dash0AwsMetricScrapeConfig\nspec:\n  namespaces: []\n  integrationFilters: []\n",
			wantErr: `unknown field "integrationFilters"`,
		},
		{
			name:    "unknown namespace field",
			doc:     "apiVersion: dash0.com/v1alpha1\nkind: Dash0AwsMetricScrapeConfig\nspec:\n  namespaces:\n    - namespace: AWS/S3\n      metric: [BucketSizeBytes]\n",
			wantErr: `unknown field "metric"`,
		},
		{
			name:    "wrong apiVersion",
			doc:     "apiVersion: dash0.com/v1alpha2\nkind: Dash0AwsMetricScrapeConfig\nspec:\n  namespaces: []\n",
			wantErr: "apiVersion must be dash0.com/v1alpha1",
		},
		{
			name:    "wrong kind",
			doc:     "apiVersion: dash0.com/v1alpha1\nkind: Dash0Dataset\nspec:\n  namespaces: []\n",
			wantErr: "kind must be Dash0AwsMetricScrapeConfig",
		},
		{
			name:    "YAML syntax error",
			doc:     "apiVersion: dash0.com/v1alpha1\nkind: [\n",
			wantErr: "failed to parse",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetupTestEnv(t)
			server := newServer(t)

			_, _, err := runCmd(t, server.URL, "update", integrationID, "-f", writeYAML(t, tt.doc))

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, server.Requests())
		})
	}
}

func TestUpdate_WarnsAboutIgnoredMetadata(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusOK, fixtureConfigured))
	server.On(http.MethodPut, apiPath, respond(http.StatusOK, fixtureConfigured))
	doc := `apiVersion: dash0.com/v1alpha1
kind: Dash0AwsMetricScrapeConfig
metadata:
  labels:
    dash0.com/id: ` + integrationID + `
    team: platform
  annotations:
    owner: alice
spec:
  namespaces: []
`

	_, stderr, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, doc))

	require.NoError(t, err)
	assert.Contains(t, stderr, `warning: the Dash0 API ignores metadata.annotations["owner"], metadata.labels["team"]`)
}

func TestUpdate_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusNotFound, fixtureNotFound))

	_, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, configYAML))

	require.Error(t, err)
	assert.Contains(t, err.Error(), `AWS integration "`+integrationID+`" not found`)
	assert.Empty(t, requestsFor(server, http.MethodPut))
}

func TestUpdate_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusUnauthorized, fixtureUnauthorized))

	_, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, configYAML))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestUpdate_MalformedResponseNamesTheField(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodGet, apiPath, respond(http.StatusOK, fixtureConfigured))
	server.On(http.MethodPut, apiPath, testutil.MockResponse{StatusCode: http.StatusOK, Body: json.RawMessage(`{"spec":{"namespaces":[{"namespace":"AWS/S3","metrics":"oops"}]}}`)})

	stdout, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, configYAML))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "metrics")
	assert.Empty(t, stdout)
}

func TestDelete_Force(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodDelete, apiPath, testutil.MockResponse{StatusCode: http.StatusNoContent, ContentType: "text/plain", Validator: testutil.RequireHeaders})

	stdout, _, err := runCmd(t, server.URL, "delete", integrationID, "--force")

	require.NoError(t, err)
	assert.Equal(t, `AWS metric scrape config of integration "`+integrationID+`" cleared`+"\n", stdout)
	require.Len(t, requestsFor(server, http.MethodDelete), 1)
}

func TestDelete_PromptDeclined(t *testing.T) {
	testutil.SetupTestEnv(t)
	t.Cleanup(confirmation.SetReaderForTest(strings.NewReader("n\n")))
	server := newServer(t)

	stdout, _, err := runCmd(t, server.URL, "delete", integrationID)

	require.NoError(t, err)
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
	assert.Contains(t, stderr, `AWS integration "`+integrationID+`" was already deleted`)
}

func TestDelete_NotFoundWithoutForce(t *testing.T) {
	testutil.SetupTestEnv(t)
	t.Cleanup(confirmation.SetReaderForTest(strings.NewReader("y\n")))
	server := newServer(t)
	server.On(http.MethodDelete, apiPath, respond(http.StatusNotFound, fixtureNotFound))

	_, _, err := runCmd(t, server.URL, "delete", integrationID)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `AWS integration "`+integrationID+`" not found`)
}

func TestDelete_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodDelete, apiPath, respond(http.StatusUnauthorized, fixtureUnauthorized))

	_, _, err := runCmd(t, server.URL, "delete", integrationID, "--force")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}
