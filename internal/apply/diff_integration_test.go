//go:build integration

package apply

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const diffCheckRuleID = "47b6ccbe-82ab-47c6-a613-ce0d7f34353e"

func newDiffRoot() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "")
	root.AddCommand(NewDiffCmd())
	return root
}

func runDiffCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newDiffRoot()
	root.SetArgs(append([]string{"diff"}, args...))
	var err error
	out := testutil.CaptureStdout(t, func() { err = root.Execute() })
	return out, err
}

func writeDiffFile(t *testing.T, content string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "checkrule.yaml")
	require.NoError(t, os.WriteFile(f, []byte(content), 0644))
	return f
}

func requireNoWrites(t *testing.T, server *testutil.MockServer) {
	t.Helper()
	for _, r := range server.Requests() {
		assert.Equal(t, http.MethodGet, r.Method, "diff must not write: %s %s", r.Method, r.Path)
	}
}

func requireExitCode(t *testing.T, err error, code int) {
	t.Helper()
	var exitErr *ExitError
	require.True(t, errors.As(err, &exitErr), "expected ExitError, got %v", err)
	assert.Equal(t, code, exitErr.Code)
}

func TestDiff_RequiresExperimentalFlag(t *testing.T) {
	testutil.SetupTestEnv(t)
	f := writeDiffFile(t, "kind: CheckRule\nname: r\nexpression: up == 0\n")

	_, err := runDiffCmd(t, "-f", f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--experimental")
}

func TestDiff_Create(t *testing.T) {
	testutil.SetupTestEnv(t)
	f := writeDiffFile(t, "kind: CheckRule\nid: "+diffCheckRuleID+"\nname: test-check-rule\nexpression: up == 0\n")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureCheckRulesNotFound,
		Validator:  testutil.RequireHeaders,
	})

	out, err := runDiffCmd(t, "-X", "-f", f, "--api-url", server.URL, "--auth-token", testAuthToken)

	requireExitCode(t, err, ExitCodeDiffFound)
	assert.Contains(t, out, "Check rule")
	assert.Contains(t, out, "would be created")
	requireNoWrites(t, server)
}

func TestDiff_UpdateShowsDiff(t *testing.T) {
	testutil.SetupTestEnv(t)
	f := writeDiffFile(t, "kind: CheckRule\nid: "+diffCheckRuleID+"\nname: test-check-rule\nexpression: up == 12345\n")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/alerting/check-rules/"+diffCheckRuleID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureCheckRulesImportSuccess,
		Validator:  testutil.RequireHeaders,
	})

	out, err := runDiffCmd(t, "-X", "-f", f, "--api-url", server.URL, "--auth-token", testAuthToken)

	requireExitCode(t, err, ExitCodeDiffFound)
	assert.Contains(t, out, "--- Check rule (before)")
	assert.Contains(t, out, "+++ Check rule (after)")
	assert.Contains(t, out, "12345")
	requireNoWrites(t, server)
}

func TestDiff_NoChanges(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/alerting/check-rules/"+diffCheckRuleID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureCheckRulesImportSuccess,
		Validator:  testutil.RequireHeaders,
	})

	// Export the fixture state and diff it against itself.
	exportPath := filepath.Join(t.TempDir(), "export.yaml")
	body, err := os.ReadFile(filepath.Join(testutil.FixturesDir(), testutil.FixtureCheckRulesImportSuccess))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(exportPath, body, 0644))

	// ponytail: JSON is valid YAML, so the fixture doubles as the input file.
	out, err := runDiffCmd(t, "-X", "-f", exportPath, "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "no changes")
	requireNoWrites(t, server)
}

func TestDiff_FetchFailureAbortsWithExitCode2(t *testing.T) {
	testutil.SetupTestEnv(t)
	f := writeDiffFile(t, "kind: CheckRule\nid: "+diffCheckRuleID+"\nname: test-check-rule\nexpression: up == 0\n")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   testutil.FixtureDashboardsUnauthorized,
		Validator:  testutil.RequireHeaders,
	})

	out, err := runDiffCmd(t, "-X", "-f", f, "--api-url", server.URL, "--auth-token", testAuthToken)

	requireExitCode(t, err, ExitCodeDiffError)
	assert.Empty(t, out)
	requireNoWrites(t, server)
}

func TestDiff_ValidationErrorExitsWithCode2(t *testing.T) {
	testutil.SetupTestEnv(t)
	f := writeDiffFile(t, "kind: Bogus\nname: x\n")

	_, err := runDiffCmd(t, "-X", "-f", f, "--api-url", "http://unused", "--auth-token", testAuthToken)

	requireExitCode(t, err, ExitCodeDiffError)
}

func TestDiff_AgentModeJSON(t *testing.T) {
	testutil.SetupTestEnv(t)
	withAgentMode(t, true)
	f := writeDiffFile(t, "kind: CheckRule\nid: "+diffCheckRuleID+"\nname: test-check-rule\nexpression: up == 0\n")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureCheckRulesNotFound,
		Validator:  testutil.RequireHeaders,
	})

	out, err := runDiffCmd(t, "-X", "-f", f, "--api-url", server.URL, "--auth-token", testAuthToken)

	requireExitCode(t, err, ExitCodeDiffFound)
	var files []dryRunFileJSON
	require.NoError(t, json.Unmarshal([]byte(out), &files))
	require.Len(t, files, 1)
	require.Len(t, files[0].Changes, 1)
	assert.Equal(t, "create", files[0].Changes[0].Op)
	assert.Equal(t, "Check rule", files[0].Changes[0].Kind)
}

func TestDiff_SinceReportsDeletions(t *testing.T) {
	testutil.SetupTestEnv(t)

	dir := t.TempDir()
	runGitCmd(t, dir, "init", "-q", "-b", "main")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "config", "commit.gpgsign", "false")

	writeFileFixture(t, dir, "dashboard.yaml", "apiVersion: dash0.com/v1alpha1\nkind: Dashboard\nmetadata:\n  name: my-dashboard\n  dash0Extensions:\n    id: a1b2c3d4-5678-90ab-cdef-1234567890ab\nspec:\n  display:\n    name: My Dashboard\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "add dashboard")
	before := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))

	require.NoError(t, os.Remove(filepath.Join(dir, "dashboard.yaml")))
	writeFileFixture(t, dir, "keep.yaml", "apiVersion: dash0.com/v1alpha1\nkind: View\nmetadata:\n  name: keep\n  labels:\n    dash0.com/id: keep-id\nspec:\n  query: \"true\"\n")
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-q", "-m", "remove dashboard")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureViewsNotFound,
		Validator:  testutil.RequireHeaders,
	})

	out, err := runDiffCmd(t, "-X", "-f", dir, "--since", before, "--api-url", server.URL, "--auth-token", testAuthToken)

	requireExitCode(t, err, ExitCodeDiffFound)
	assert.Contains(t, out, "keep.yaml: View")
	assert.Contains(t, out, "would be created")
	assert.Contains(t, out, "would be deleted")
	assert.Contains(t, out, "a1b2c3d4-5678-90ab-cdef-1234567890ab")
	requireNoWrites(t, server)
}

func TestDiff_UsageErrorsExitWithCode2(t *testing.T) {
	testutil.SetupTestEnv(t)
	f := writeDiffFile(t, "kind: CheckRule\nname: r\nexpression: up == 0\n")

	for name, args := range map[string][]string{
		"missing experimental": {"-f", f},
		"missing file":         {"-X"},
		"unknown flag":         {"-X", "-f", f, "--bogus"},
		"stdin with since":     {"-X", "-f", "-", "--since", "HEAD"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := runDiffCmd(t, args...)
			requireExitCode(t, err, ExitCodeDiffError)
		})
	}
}

func TestDiff_UnchangedDashboardExitsZero(t *testing.T) {
	testutil.SetupTestEnv(t)
	const id = "a1b2c3d4-5678-90ab-cdef-1234567890ab"
	const doc = "apiVersion: dash0.com/v1alpha1\nkind: Dashboard\nmetadata:\n  name: my-dashboard\n  dash0Extensions:\n    id: " + id + "\nspec:\n  display:\n    name: My Dashboard\n"
	f := writeDiffFile(t, doc)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/dashboards/"+id, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body: map[string]any{
			"kind":     "Dashboard",
			"metadata": map[string]any{"name": "my-dashboard", "dash0Extensions": map[string]any{"id": id}},
			"spec":     map[string]any{"display": map[string]any{"name": "My Dashboard"}},
		},
		Validator: testutil.RequireHeaders,
	})

	out, err := runDiffCmd(t, "-X", "-f", f, "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "no changes")
	requireNoWrites(t, server)
}
