//go:build integration

package automerge

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAuthToken    = "auth_test_token"
	repositoryID     = "0b7f1c2e-4a5d-4e8f-9c61-2d3a4b5c6d7e"
	repositorySlug   = "acme/api"
	repositoriesPath = "/api/darkplane/repositories"
	settingsPath     = repositoriesPath + "/" + repositoryID + "/settings"

	fixtureRepositories = "automerge/list_repositories.json"
	fixtureSettings     = "automerge/get_settings.json"
	fixtureUpdated      = "automerge/put_settings.json"
	fixtureSuperseded   = "automerge/error_superseded.json"
	fixtureForbidden    = "automerge/error_forbidden.json"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewAutoMergeCmd())
	return root
}

func runCmd(t *testing.T, serverURL string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetArgs(append(append([]string{"-X", "automerge", "settings"}, args...), "--api-url", serverURL, "--auth-token", testAuthToken))
	stderr = testutil.CaptureStderr(t, func() {
		stdout = testutil.CaptureStdout(t, func() {
			err = cmd.Execute()
		})
	})
	return stdout, stderr, err
}

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "automerge.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func newServer(t *testing.T) *testutil.MockServer {
	t.Helper()
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, repositoriesPath, respond(http.StatusOK, fixtureRepositories))
	server.On(http.MethodGet, settingsPath, respond(http.StatusOK, fixtureSettings))
	return server
}

func respond(status int, fixture string) testutil.MockResponse {
	return testutil.MockResponse{StatusCode: status, BodyFile: fixture, Validator: testutil.RequireHeaders}
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

func exportedYAML(t *testing.T, serverURL string) string {
	t.Helper()
	stdout, _, err := runCmd(t, serverURL, "get", repositorySlug, "-o", "yaml")
	require.NoError(t, err)
	return stdout
}

func sentSettings(t *testing.T, server *testutil.MockServer) dash0api.RepositorySettingsRequest {
	t.Helper()
	puts := requestsFor(server, http.MethodPut, settingsPath)
	require.Len(t, puts, 1)
	var sent dash0api.RepositorySettingsRequest
	require.NoError(t, json.Unmarshal(puts[0].Body, &sent))
	return sent
}

func TestAutoMerge_RequiresExperimental(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"automerge", "settings", "get", repositorySlug, "--api-url", server.URL, "--auth-token", testAuthToken})
	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental")
	assert.Empty(t, server.Requests())
}

func TestGet_TableBySlug(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)

	stdout, _, err := runCmd(t, server.URL, "get", "Acme/API", "-o", "table")

	require.NoError(t, err)
	assert.Contains(t, stdout, "Repository: acme/api")
	assert.Contains(t, stdout, "Repository ID: "+repositoryID)
	assert.Contains(t, stdout, "Version: 2")
	assert.Contains(t, stdout, "Score threshold: 75")
	assert.Contains(t, stdout, "Blast radius (weight 0.5, floor 40)")
	assert.Contains(t, stdout, "Evidence quality (weight 0.5, no floor)")
	assert.Contains(t, stdout, "protected: deploy/**")
	assert.Contains(t, stdout, "auto_merge: docs/** (1 author)")
	searches := requestsFor(server, http.MethodGet, repositoriesPath)
	require.Len(t, searches, 1)
	query, err := url.ParseQuery(searches[0].Query)
	require.NoError(t, err)
	assert.Equal(t, "Acme/API", query.Get("search"))
}

func TestGet_ByIDSkipsTheLookup(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)

	_, _, err := runCmd(t, server.URL, "get", repositoryID, "-o", "json")

	require.NoError(t, err)
	assert.Empty(t, requestsFor(server, http.MethodGet, repositoriesPath))
	assert.Len(t, requestsFor(server, http.MethodGet, settingsPath), 1)
}

func TestGet_UnknownSlugIsNotFound(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)

	_, _, err := runCmd(t, server.URL, "get", "acme/ap")

	require.Error(t, err)
	assert.Contains(t, err.Error(), `repository "acme/ap" not found`)
	assert.Empty(t, requestsFor(server, http.MethodGet, settingsPath))
}

func TestUpdate_SendsSpecAndVersionAndShowsDiff(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodPut, settingsPath, respond(http.StatusOK, fixtureUpdated))
	edited := strings.Replace(exportedYAML(t, server.URL), "softThreshold: 75", "softThreshold: 80", 1)

	stdout, stderr, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, edited))

	require.NoError(t, err)
	assert.Empty(t, stderr, "an exported document must not warn")
	assert.Contains(t, stdout, "-softThreshold: 75")
	assert.Contains(t, stdout, "+softThreshold: 80")
	assert.Contains(t, stdout, `AutoMerge settings "acme/api" updated (version 3)`)
	sent := sentSettings(t, server)
	assert.Equal(t, dash0api.RepositorySettingsRequestKind("DarkplaneRepositorySettings"), sent.Kind)
	require.NotNil(t, sent.ApiVersion)
	assert.Equal(t, dash0api.RepositorySettingsApiVersionDash0Comv1alpha1, *sent.ApiVersion)
	require.NotNil(t, sent.Metadata)
	assert.Equal(t, "2", *sent.Metadata.Labels.Dash0Comversion)
	assert.Equal(t, float32(80), sent.Spec.SoftThreshold)
	assert.Equal(t, []string{"deploy/**"}, sent.Spec.ProtectedPaths)
	require.Len(t, sent.Spec.SoftRiskCriteria, 2)
}

func TestUpdate_ExportedSettingsReapplyWithoutDiff(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)

	stdout, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, exportedYAML(t, server.URL)), "--dry-run")

	require.NoError(t, err)
	assert.Contains(t, stdout, `AutoMerge settings "acme/api": no changes`)
}

func TestUpdate_DryRunMakesNoPut(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	edited := strings.Replace(exportedYAML(t, server.URL), "softThreshold: 75", "softThreshold: 90", 1)

	stdout, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, edited), "--dry-run")

	require.NoError(t, err)
	assert.Contains(t, stdout, "+softThreshold: 90")
	assert.Empty(t, requestsFor(server, http.MethodPut, settingsPath))
}

func TestUpdate_RemovingAProtectedRuleAlsoUnprotectsItsPath(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodPut, settingsPath, respond(http.StatusOK, fixtureUpdated))
	doc := `kind: DarkplaneRepositorySettings
metadata:
  name: acme/api
spec:
  botMode: auto_approve
  triggerMode: auto
  waitForChecks: true
  softThreshold: 75
  softRiskCriteria: []
  pathRules:
  - level: protected
    paths: ["deploy/**"]
    authors: ["1234"]
`
	_, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, doc))

	require.NoError(t, err)
	sent := sentSettings(t, server)
	assert.Equal(t, []string{}, sent.Spec.ProtectedPaths, "an author-scoped protected rule must not protect the path from everyone")
	assert.Nil(t, sent.Metadata, "a document without a version must not send one")
}

func TestUpdate_LegacyProtectedPathIsFoldedIntoARule(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodPut, settingsPath, respond(http.StatusOK, fixtureUpdated))
	edited := strings.Replace(exportedYAML(t, server.URL), "  - deploy/**\n  reviewer:", "  - deploy/**\n  - secrets/**\n  reviewer:", 1)
	require.Contains(t, edited, "secrets/**")

	_, stderr, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, edited))

	require.NoError(t, err)
	assert.Contains(t, stderr, "added a protected path rule for secrets/**")
	sent := sentSettings(t, server)
	assert.Equal(t, []string{"deploy/**", "secrets/**"}, sent.Spec.ProtectedPaths)
	require.NotNil(t, sent.Spec.PathRules)
	rules := *sent.Spec.PathRules
	require.Len(t, rules, 3)
	assert.Equal(t, dash0api.PathRuleLevelProtected, rules[2].Level)
	assert.Equal(t, []string{"secrets/**"}, rules[2].Paths)
}

func TestUpdate_SupersededVersionExplainsWhatToDo(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodPut, settingsPath, respond(http.StatusBadRequest, fixtureSuperseded))

	_, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, exportedYAML(t, server.URL)))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "changed since they were read")
	assert.Contains(t, err.Error(), "dash0.com/version")
}

func TestUpdate_ForbiddenNamesPermissions(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	server.On(http.MethodPut, settingsPath, respond(http.StatusForbidden, fixtureForbidden))

	_, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, exportedYAML(t, server.URL)))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

func TestUpdate_ArgumentMustMatchTheFileID(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	other := strings.Replace(exportedYAML(t, server.URL), repositoryID, "7f1c2e04-0d55-4f5e-9a51-2b7c1c1d9e11", 1)

	_, _, err := runCmd(t, server.URL, "update", repositorySlug, "-f", writeYAML(t, other))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "dash0.com/id label is 7f1c2e04")
	assert.Empty(t, requestsFor(server, http.MethodPut, settingsPath))
}

func TestUpdate_MissingCriteriaFailsBeforeAnyCall(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	doc := "kind: DarkplaneRepositorySettings\nmetadata:\n  name: acme/api\nspec:\n  botMode: auto_merge\n"

	_, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, doc))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "spec.softRiskCriteria")
	assert.Empty(t, server.Requests())
}

func TestUpdate_UnknownSpecFieldIsRejected(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newServer(t)
	edited := strings.Replace(exportedYAML(t, server.URL), "softThreshold: 75", "softTreshold: 75\n  softThreshold: 75", 1)

	_, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, edited))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "softTreshold")
	assert.Empty(t, requestsFor(server, http.MethodPut, settingsPath))
}
