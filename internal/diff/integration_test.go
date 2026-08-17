//go:build integration

package diff

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
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
