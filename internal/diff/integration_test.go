//go:build integration

package diff

import (
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAuthToken = "auth_test_token"

var viewIDPattern = regexp.MustCompile(`^/api/views/[^/]+$`)

func withAgentMode(t *testing.T, enabled bool) {
	t.Helper()
	prev := agentmode.Enabled
	agentmode.Enabled = enabled
	t.Cleanup(func() { agentmode.Enabled = prev })
}

func writeViewFixture(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(`kind: View
metadata:
  name: my-view
  labels:
    dash0.com/id: view-id
spec:
  type: logs
  display:
    name: my-view
`), 0644))
	return path
}

func TestDiff_Create(t *testing.T) {
	testutil.SetupTestEnv(t)
	tmpDir := t.TempDir()
	yamlFile := writeViewFixture(t, tmpDir, "view.yaml")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		Body:       map[string]any{"error": map[string]any{"code": 404, "message": "not found"}},
	})

	cmd := NewDiffCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken, "--experimental"})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.Error(t, cmdErr, "a pending create must surface as a non-nil error so main() exits 1")
	var pending *PendingDifferencesError
	require.ErrorAs(t, cmdErr, &pending)
	assert.Equal(t, 1, pending.Count)
	assert.Contains(t, output, "Create")
	assert.Contains(t, output, "my-view")
}

func TestDiff_UpdateWithNoChanges(t *testing.T) {
	testutil.SetupTestEnv(t)
	tmpDir := t.TempDir()
	yamlFile := writeViewFixture(t, tmpDir, "view.yaml")

	id := "view-id"
	existing := &dash0api.ViewDefinition{}
	existing.Kind = dash0api.ViewDefinitionKind("View")
	existing.Metadata.Name = "my-view"
	existing.Metadata.Labels = &dash0api.ViewLabels{Dash0Comid: &id}
	existing.Spec.Type = dash0api.ViewType("logs")
	existing.Spec.Display = dash0api.ViewDisplay{Name: "my-view"}

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       existing,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewDiffCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken, "--experimental"})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr, "an update with no real change must exit clean (0)")
	assert.Contains(t, output, "No differences")
}

func TestDiff_UpdateWithChanges(t *testing.T) {
	testutil.SetupTestEnv(t)
	tmpDir := t.TempDir()
	yamlFile := writeViewFixture(t, tmpDir, "view.yaml")

	id := "view-id"
	existing := &dash0api.ViewDefinition{}
	existing.Metadata.Name = "old-name"
	existing.Metadata.Labels = &dash0api.ViewLabels{Dash0Comid: &id}
	existing.Spec.Type = dash0api.ViewType("logs")
	existing.Spec.Display = dash0api.ViewDisplay{Name: "old-name"}

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       existing,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewDiffCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken, "--experimental"})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.Error(t, cmdErr)
	var pending *PendingDifferencesError
	require.ErrorAs(t, cmdErr, &pending)
	assert.Equal(t, 1, pending.Count)
	assert.Contains(t, output, "old-name")
	assert.Contains(t, output, "my-view")
}

func TestDiff_FetchFailure_Aborts(t *testing.T) {
	testutil.SetupTestEnv(t)
	tmpDir := t.TempDir()
	yamlFile := writeViewFixture(t, tmpDir, "view.yaml")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusInternalServerError,
		Body:       map[string]any{"error": map[string]any{"code": 500, "message": "boom"}},
	})

	cmd := NewDiffCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken, "--experimental"})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.Error(t, cmdErr)
	var pending *PendingDifferencesError
	assert.False(t, errors.As(cmdErr, &pending), "a genuine fetch failure must not be reported as PendingDifferencesError")
	assert.Empty(t, output, "the all-or-nothing fetch gate must print nothing when a fetch fails")
}

func TestDiff_JSONOutput_Create(t *testing.T) {
	testutil.SetupTestEnv(t)
	withAgentMode(t, true)
	tmpDir := t.TempDir()
	yamlFile := writeViewFixture(t, tmpDir, "view.yaml")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		Body:       map[string]any{"error": map[string]any{"code": 404, "message": "not found"}},
	})

	cmd := NewDiffCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken, "--experimental"})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.Error(t, cmdErr)
	assert.Contains(t, output, `"op": "create"`)
	assert.Contains(t, output, `"path": "`+yamlFile+`"`)
}

// TestDiff_Since_DeletionPreview exercises --since's deletion preview using
// the checked-in whole-file-deletion git scenario (a dashboard file removed
// entirely between "before" and HEAD, alongside a surviving view file). It
// pins that diff --since reports both the surviving document's fetch-based
// classification (view-b has no matching server-side asset yet, so it's a
// create) and the deletion candidate side by side, without ever calling a
// delete endpoint.
func TestDiff_Since_DeletionPreview(t *testing.T) {
	testutil.SetupTestEnv(t)
	repoDir, ref := testutil.BuildGitScenario(t, "whole-file-deletion")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		Body:       map[string]any{"error": map[string]any{"code": 404, "message": "not found"}},
	})

	cmd := NewDiffCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"-f", repoDir, "--since", ref, "--api-url", server.URL, "--auth-token", testAuthToken, "--experimental"})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.Error(t, cmdErr)
	var pending *PendingDifferencesError
	require.ErrorAs(t, cmdErr, &pending)
	assert.Equal(t, 2, pending.Count, "one create (view-b) plus one delete (dashboard-a)")
	assert.Contains(t, output, "Delete")
	assert.Contains(t, output, "Dashboard A")
	assert.Contains(t, output, "Create")

	// Never a delete API call -- diff must only preview.
	for _, req := range server.Requests() {
		assert.NotEqual(t, http.MethodDelete, req.Method, "diff must never call a delete endpoint")
	}
}

// TestDiff_SoftDeletedAssetIsCreate pins that a soft-deleted live asset is
// reported as a create, because apply restores it. The diff normalization
// strips the deleted-at marker, so comparing against it would report "No
// differences" for an asset apply is about to bring back.
func TestDiff_SoftDeletedAssetIsCreate(t *testing.T) {
	testutil.SetupTestEnv(t)
	yamlFile := writeViewFixture(t, t.TempDir(), "view.yaml")

	id := "view-id"
	deletedAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	existing := &dash0api.ViewDefinition{}
	existing.Kind = dash0api.ViewDefinitionKind("View")
	existing.Metadata.Name = "my-view"
	existing.Metadata.Labels = &dash0api.ViewLabels{Dash0Comid: &id}
	existing.Metadata.Annotations = &dash0api.ViewAnnotations{Dash0ComdeletedAt: &deletedAt}
	existing.Spec.Type = dash0api.ViewType("logs")
	existing.Spec.Display = dash0api.ViewDisplay{Name: "my-view"}

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       existing,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewDiffCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken, "--experimental"})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	var pending *PendingDifferencesError
	require.ErrorAs(t, cmdErr, &pending)
	assert.Equal(t, 1, pending.Count)
	assert.Contains(t, output, `Create View "my-view"`)
}

// TestDiff_SLOFetchedByOrigin pins that an SLO is looked up by its origin
// (which wins over id, as in apply), so an unchanged SLO reports no
// differences instead of a create.
func TestDiff_SLOFetchedByOrigin(t *testing.T) {
	testutil.SetupTestEnv(t)
	yamlFile := filepath.Join(t.TempDir(), "slo.yaml")
	doc := `apiVersion: dash0.com/v1alpha1
kind: SLO
metadata:
  name: checkout-availability
  labels:
    dash0.com/origin: checkout-slo
    dash0.com/id: some-other-id
spec:
  display:
    name: Checkout availability
`
	require.NoError(t, os.WriteFile(yamlFile, []byte(doc), 0644))

	var existing dash0api.SloDefinition
	require.NoError(t, dash0yaml.Unmarshal([]byte(doc), &existing))

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/slos/checkout-slo", testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       &existing,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewDiffCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken, "--experimental"})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "No differences")
}

func runDiffCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewDiffCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs(append(args, "--auth-token", testAuthToken, "--experimental"))
	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})
	return output, cmdErr
}

// With --since, a target that holds no YAML file anymore is an all-deletions
// run, as in apply --since, not an input error.
func TestDiff_Since_EmptyTargetListsEveryDeletion(t *testing.T) {
	testutil.SetupTestEnv(t)
	repoDir, ref := testutil.BuildGitScenario(t, "whole-directory-deletion")
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	output, err := runDiffCmd(t, "-f", repoDir, "--since", ref, "--api-url", server.URL)

	var pending *PendingDifferencesError
	require.ErrorAs(t, err, &pending)
	assert.Equal(t, 2, pending.Count)
	assert.Contains(t, output, `Delete Dashboard "Dashboard A" (dash-a)`)
	assert.Contains(t, output, `Delete View "View B" (view-b)`)
}

// With --since, a target directory removed together with its files is an
// all-deletions run too.
func TestDiff_Since_RemovedTargetListsEveryDeletion(t *testing.T) {
	testutil.SetupTestEnv(t)
	repoDir := t.TempDir()
	git := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", repoDir}, args...)...).CombinedOutput()
		require.NoErrorf(t, err, "git %v: %s", args, out)
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "test")
	target := filepath.Join(repoDir, "views")
	require.NoError(t, os.MkdirAll(target, 0o755))
	writeViewFixture(t, target, "view.yaml")
	git("add", "-A")
	git("commit", "-q", "-m", "add view")
	ref := git("rev-parse", "HEAD")
	require.NoError(t, os.RemoveAll(target))
	git("add", "-A")
	git("commit", "-q", "-m", "remove views directory")
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	output, err := runDiffCmd(t, "-f", target, "--since", ref, "--api-url", server.URL)

	var pending *PendingDifferencesError
	require.ErrorAs(t, err, &pending)
	assert.Equal(t, 1, pending.Count)
	assert.Contains(t, output, `Delete View "my-view" (view-id)`)
}

// The server stores member ids and the file lists emails, so diff resolves the
// stored ids before it compares.
func TestDiff_TeamMemberEmailsMatchStoredIDs(t *testing.T) {
	testutil.SetupTestEnv(t)
	yamlFile := filepath.Join(t.TempDir(), "team.yaml")
	doc := `apiVersion: dash0.com/v1alpha1
kind: Dash0Team
metadata:
  name: backend-team
  labels:
    dash0.com/origin: backend-team
spec:
  display:
    name: Backend Team
  members:
    - alice@example.com
`
	require.NoError(t, os.WriteFile(yamlFile, []byte(doc), 0644))
	var stored dash0api.TeamDefinitionV1Alpha1
	require.NoError(t, dash0yaml.Unmarshal([]byte(doc), &stored))
	stored.Spec.Members = []string{"user-1"}

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/teams/backend-team", testutil.MockResponse{StatusCode: http.StatusOK, Body: map[string]any{"team": &stored}, Validator: testutil.RequireHeaders})
	server.On(http.MethodGet, "/api/members", testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureMembersListSuccess, Validator: testutil.RequireHeaders})

	output, err := runDiffCmd(t, "-f", yamlFile, "--api-url", server.URL)

	require.NoError(t, err, output)
	assert.Contains(t, output, "No differences")
}

// A dataset update keeps every spec field the file leaves out, so a partial
// file that matches the stored values is not a change.
func TestDiff_PartialDatasetKeepsStoredFields(t *testing.T) {
	testutil.SetupTestEnv(t)
	yamlFile := filepath.Join(t.TempDir(), "dataset.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`apiVersion: dash0.com/v1alpha1
kind: Dash0Dataset
metadata:
  name: Staging
  labels:
    dash0.com/id: staging
spec:
  retention:
    logs: 60d
`), 0644))
	var stored dash0api.DatasetDefinition
	require.NoError(t, dash0yaml.Unmarshal([]byte(`apiVersion: dash0.com/v1alpha1
kind: Dash0Dataset
metadata:
  name: Staging
  labels:
    dash0.com/id: staging
spec:
  retention:
    traces: 30d
    logs: 60d
  processing:
    spanEventsToLogs: false
`), &stored))

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/datasets/staging", testutil.MockResponse{StatusCode: http.StatusOK, Body: &stored, Validator: testutil.RequireHeaders})

	output, err := runDiffCmd(t, "-f", yamlFile, "--api-url", server.URL)

	require.NoError(t, err, output)
	assert.Contains(t, output, "No differences")
}

func TestDiff_ViewFoundByLocalIDWithServerAssignedID(t *testing.T) {
	testutil.SetupTestEnv(t)
	yamlFile := filepath.Join(t.TempDir(), "view.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`apiVersion: dash0.com/v1alpha1
kind: View
metadata:
  name: Error Logs
  labels:
    dash0.com/id: error-logs
spec:
  display:
    name: Error Logs
  type: logs
`), 0644))
	var stored dash0api.ViewDefinition
	require.NoError(t, dash0yaml.Unmarshal([]byte(`kind: View
metadata:
  name: Error Logs
  labels:
    dash0.com/id: 5a939b78-98c2-424e-94c0-98a5f9814c21
    dash0.com/source: api
spec:
  display:
    name: Error Logs
  type: logs
`), &stored))

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/views/error-logs", testutil.MockResponse{StatusCode: http.StatusOK, Body: &stored, Validator: testutil.RequireHeaders})

	output, err := runDiffCmd(t, "-f", yamlFile, "--api-url", server.URL)

	require.NoError(t, err, output)
	assert.Contains(t, output, "No differences")
}
