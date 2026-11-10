//go:build integration

package diff

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
	"github.com/dash0hq/dash0-cli/internal/apply"
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

func writeViewFixture(t *testing.T, dir, file, id, name string) string {
	t.Helper()
	path := filepath.Join(dir, file)
	require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf(`kind: View
metadata:
  name: %[2]s
  labels:
    dash0.com/id: %[1]s
spec:
  type: logs
  display:
    name: %[2]s
`, id, name)), 0644))
	return path
}

func storedView(id, name string) *dash0api.ViewDefinition {
	view := &dash0api.ViewDefinition{}
	view.Kind = dash0api.ViewDefinitionKind("View")
	view.Metadata.Name = name
	view.Metadata.Labels = &dash0api.ViewLabels{Dash0Comid: &id}
	view.Spec.Type = dash0api.ViewType("logs")
	view.Spec.Display = dash0api.ViewDisplay{Name: name}
	return view
}

func TestDiff_Create(t *testing.T) {
	testutil.SetupTestEnv(t)
	tmpDir := t.TempDir()
	yamlFile := writeViewFixture(t, tmpDir, "view.yaml", "view-id", "my-view")

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
	yamlFile := writeViewFixture(t, tmpDir, "view.yaml", "view-id", "my-view")

	existing := storedView("view-id", "my-view")

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
	yamlFile := writeViewFixture(t, tmpDir, "view.yaml", "view-id", "my-view")

	existing := storedView("view-id", "old-name")

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
	yamlFile := writeViewFixture(t, tmpDir, "view.yaml", "view-id", "my-view")

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
	yamlFile := writeViewFixture(t, tmpDir, "view.yaml", "view-id", "my-view")

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
	yamlFile := writeViewFixture(t, t.TempDir(), "view.yaml", "view-id", "my-view")

	deletedAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	existing := storedView("view-id", "my-view")
	existing.Metadata.Annotations = &dash0api.ViewAnnotations{Dash0ComdeletedAt: &deletedAt}

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
	writeViewFixture(t, target, "view.yaml", "view-id", "my-view")
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
// stored ids before it compares, with one members lookup per team. A failed
// members lookup means diff cannot compare the team's members, so it aborts
// like any other failed read instead of reporting a guessed update.
func TestDiff_TeamMembers(t *testing.T) {
	tests := []struct {
		name    string
		members testutil.MockResponse
		check   func(t *testing.T, server *testutil.MockServer, output string, err error)
	}{
		{
			name:    "emails match stored ids",
			members: testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureMembersListSuccess, Validator: testutil.RequireHeaders},
			check: func(t *testing.T, server *testutil.MockServer, output string, err error) {
				require.NoError(t, err, output)
				assert.Contains(t, output, "No differences")
				assert.Equal(t, 1, countRequests(server, http.MethodGet, "/api/members"), "one members lookup must cover the stored and the local team")
			},
		},
		{
			name: "lookup failure aborts",
			members: testutil.MockResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       map[string]any{"error": map[string]any{"code": 500, "message": "boom"}},
				Validator:  testutil.RequireHeaders,
			},
			check: func(t *testing.T, _ *testutil.MockServer, output string, err error) {
				require.Error(t, err)
				var pending *PendingDifferencesError
				assert.False(t, errors.As(err, &pending), "a failed members lookup must exit 2, not 1")
				assert.Contains(t, err.Error(), "boom", "the error must come from the members lookup")
				assert.Empty(t, output, "no plan may be printed when a read fails")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			server.On(http.MethodGet, "/api/members", tt.members)

			output, err := runDiffCmd(t, "-f", yamlFile, "--api-url", server.URL)

			tt.check(t, server, output, err)
		})
	}
}

func countRequests(server *testutil.MockServer, method, path string) int {
	count := 0
	for _, req := range server.Requests() {
		if req.Method == method && req.Path == path {
			count++
		}
	}
	return count
}

// A file whose assets are all unchanged has nothing pending, so it is left out
// of the agent-mode JSON.
func TestDiff_JSONOutput_LeavesOutUnchangedFile(t *testing.T) {
	testutil.SetupTestEnv(t)
	withAgentMode(t, true)
	dir := t.TempDir()
	writeViewFixture(t, dir, "changed.yaml", "view-changed", "new-name")
	writeViewFixture(t, dir, "unchanged.yaml", "view-unchanged", "same-name")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/views/view-changed", testutil.MockResponse{StatusCode: http.StatusOK, Body: storedView("view-changed", "old-name"), Validator: testutil.RequireHeaders})
	server.On(http.MethodGet, "/api/views/view-unchanged", testutil.MockResponse{StatusCode: http.StatusOK, Body: storedView("view-unchanged", "same-name"), Validator: testutil.RequireHeaders})

	output, err := runDiffCmd(t, "-f", dir, "--api-url", server.URL)

	var pending *PendingDifferencesError
	require.ErrorAs(t, err, &pending, output)
	var files []struct {
		Path    string            `json:"path"`
		Changes []json.RawMessage `json:"changes"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &files), output)
	require.Len(t, files, 1, output)
	assert.Equal(t, "changed.yaml", files[0].Path)
	assert.Len(t, files[0].Changes, 1)
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

const skillDoc = `kind: Dash0Skill
metadata:
  labels:
    dash0.com/origin: dash0-cli_incident-triage
spec:
  files:
    - relativePath: SKILL.md
      content: |
        ---
        name: incident-triage
        description: Triage a production incident from its failed checks.
        ---

        Start from the failed check, then follow the trace to the failing service.
    - relativePath: references/runbook.md
      content: |
        # Runbook

        Check the deploy log first.
  permissions:
    - role: basic_member
      actions:
        - custom_skill:read
      defaultEnabled: false
`

func TestDiff_Skill(t *testing.T) {
	const skillOriginPath = "/api/agent0/custom-skills/dash0-cli_incident-triage"
	cases := map[string]struct {
		stored     testutil.MockResponse
		wantOutput string
		wantErr    bool
	}{
		"stored skill matches": {
			stored:     testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureAgent0SkillsGetSuccess, Validator: testutil.RequireHeaders},
			wantOutput: "No differences",
		},
		"deleted skill is a create, because apply restores it": {
			stored:     testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: testutil.FixtureAgent0SkillsGetDeleted, Validator: testutil.RequireHeaders},
			wantOutput: `Create Agent0 skill "incident-triage" (dash0-cli_incident-triage)`,
			wantErr:    true,
		},
		"new origin is a create": {
			stored:     testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: testutil.FixtureAgent0SkillsNotFound, Validator: testutil.RequireHeaders},
			wantOutput: `Create Agent0 skill "incident-triage" (dash0-cli_incident-triage)`,
			wantErr:    true,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			testutil.SetupTestEnv(t)
			yamlFile := filepath.Join(t.TempDir(), "skill.yaml")
			require.NoError(t, os.WriteFile(yamlFile, []byte(skillDoc), 0644))
			server := testutil.NewMockServer(t, testutil.FixturesDir())
			server.On(http.MethodGet, skillOriginPath, c.stored)

			output, err := runDiffCmd(t, "-f", yamlFile, "--api-url", server.URL)

			assert.Equal(t, c.wantErr, err != nil, "differences pending is reported as an error so the exit code is 1")
			assert.Contains(t, output, c.wantOutput)
			for _, request := range server.Requests() {
				assert.Equal(t, http.MethodGet, request.Method, "diff must never write")
			}
		})
	}
}

func TestDiff_SyntheticCheckFoundByLocalIDWithServerAssignedID(t *testing.T) {
	testutil.SetupTestEnv(t)
	local := `apiVersion: dash0.com/v1alpha1
kind: Dash0SyntheticCheck
metadata:
  name: checkout-health
  labels:
    dash0.com/id: checkout-health
  annotations:
    dash0.com/folder-path: /checkout
spec:
  enabled: false
`
	yamlFile := filepath.Join(t.TempDir(), "check.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(local), 0644))
	var stored dash0api.SyntheticCheckDefinition
	require.NoError(t, dash0yaml.Unmarshal([]byte(strings.Replace(local, "dash0.com/id: checkout-health", "dash0.com/id: 64617368-3073-796e-7468-50cf467aaec7", 1)), &stored))

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/synthetic-checks/checkout-health", testutil.MockResponse{StatusCode: http.StatusOK, Body: &stored, Validator: testutil.RequireHeaders})

	output, err := runDiffCmd(t, "-f", yamlFile, "--api-url", server.URL)

	require.NoError(t, err, output)
	assert.Contains(t, output, "No differences")
}

func TestDiff_AwsMetricScrapeConfig(t *testing.T) {
	const integrationID = "27755ed0-0009-422d-8c11-19c6dd72df98"
	exported, err := os.ReadFile(filepath.Join(testutil.FixturesDir(), "awsmetricscrapeconfigs", "get_configured.json"))
	require.NoError(t, err)
	cases := map[string]struct {
		storedFixture string
		wantOutput    string
		wantErr       bool
	}{
		"exported config is unchanged":           {storedFixture: "awsmetricscrapeconfigs/get_configured.json", wantOutput: "No differences"},
		"integration without config is a create": {storedFixture: "awsmetricscrapeconfigs/get_unconfigured.json", wantOutput: "Create AWS metric scrape config", wantErr: true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			testutil.SetupTestEnv(t)
			// diff only discovers .yaml files; JSON is valid YAML.
			yamlFile := filepath.Join(t.TempDir(), "scrape.yaml")
			require.NoError(t, os.WriteFile(yamlFile, exported, 0644))
			server := testutil.NewMockServer(t, testutil.FixturesDir())
			server.On(http.MethodGet, "/api/aws-metric-scrape-configs/"+integrationID, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: c.storedFixture, Validator: testutil.RequireHeaders})

			output, err := runDiffCmd(t, "-f", yamlFile, "--api-url", server.URL)

			assert.Equal(t, c.wantErr, err != nil, output)
			assert.Contains(t, output, c.wantOutput)
			for _, request := range server.Requests() {
				assert.Equal(t, http.MethodGet, request.Method, "diff must never write")
			}
		})
	}
}

// writeNewDatasetWithView writes a dataset document for slug "staging" and a
// view document next to it, the shape of a run that creates a dataset and
// fills it.
func writeNewDatasetWithView(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dataset.yaml"), []byte(`apiVersion: dash0.com/v1alpha1
kind: Dash0Dataset
metadata:
  name: Staging
  labels:
    dash0.com/id: staging
spec:
  retention:
    logs: 60d
`), 0644))
	writeViewFixture(t, dir, "view.yaml", "view-id", "my-view")
	return dir
}

var forbiddenResponse = testutil.MockResponse{
	StatusCode: http.StatusForbidden,
	Body:       map[string]any{"error": map[string]any{"code": 403, "message": "Forbidden: The given principal does not have the required permissions."}},
	Validator:  testutil.RequireHeaders,
}

// The API answers 403 for a read in a dataset that does not exist yet. apply
// creates the dataset first, so the view in it is a create, not an error.
func TestDiff_ViewInDatasetCreatedInSameRunIsCreate(t *testing.T) {
	testutil.SetupTestEnv(t)
	dir := writeNewDatasetWithView(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/datasets/staging", testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		Body:       map[string]any{"error": map[string]any{"code": 404, "message": "not found"}},
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodGet, viewIDPattern, forbiddenResponse)

	output, err := runDiffCmd(t, "-f", dir, "--dataset", "staging", "--api-url", server.URL)

	var pending *PendingDifferencesError
	require.ErrorAs(t, err, &pending, output)
	assert.Equal(t, 2, pending.Count)
	assert.Contains(t, output, `Create Dataset "Staging" (staging)`)
	assert.Contains(t, output, `Create View "my-view" (view-id)`)
}

// A token without access to an existing dataset also gets a 404 for the
// dataset and a 403 for the view, so the dropped 403 must be visible.
func TestDiff_DroppedForbiddenReadWarnsOnStderr(t *testing.T) {
	testutil.SetupTestEnv(t)
	withAgentMode(t, true)
	dir := writeNewDatasetWithView(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/datasets/staging", testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		Body:       map[string]any{"error": map[string]any{"code": 404, "message": "not found"}},
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodGet, viewIDPattern, forbiddenResponse)

	var output string
	var err error
	stderr := testutil.CaptureStderr(t, func() {
		output, err = runDiffCmd(t, "-f", dir, "--dataset", "staging", "--api-url", server.URL)
	})

	var pending *PendingDifferencesError
	require.ErrorAs(t, err, &pending, output)
	assert.Equal(t, 2, pending.Count)
	assert.True(t, json.Valid([]byte(output)), "stdout must stay valid JSON in agent mode: %s", output)
	assert.Contains(t, stderr,
		`warning: View "my-view" (view-id): read denied in dataset "staging", assumed new because this run creates "staging"; a token without access to "staging" gives the same result`+"\n")
}

// A 403 is only read as "nothing stored" for the dataset this run creates.
// Anywhere else it is a real permission error and aborts the preview.
func TestDiff_ForbiddenReadOutsideNewDatasetAborts(t *testing.T) {
	var stored dash0api.DatasetDefinition
	require.NoError(t, dash0yaml.Unmarshal([]byte(`apiVersion: dash0.com/v1alpha1
kind: Dash0Dataset
metadata:
  name: Staging
  labels:
    dash0.com/id: staging
spec:
  retention:
    logs: 60d
`), &stored))
	notFound := testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		Body:       map[string]any{"error": map[string]any{"code": 404, "message": "not found"}},
		Validator:  testutil.RequireHeaders,
	}

	for _, tc := range []struct {
		name           string
		datasetRead    testutil.MockResponse
		datasetFlagArg string
	}{
		{name: "dataset already exists", datasetRead: testutil.MockResponse{StatusCode: http.StatusOK, Body: &stored, Validator: testutil.RequireHeaders}, datasetFlagArg: "staging"},
		{name: "view read from another dataset", datasetRead: notFound, datasetFlagArg: "production"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.SetupTestEnv(t)
			dir := writeNewDatasetWithView(t)
			server := testutil.NewMockServer(t, testutil.FixturesDir())
			server.On(http.MethodGet, "/api/datasets/staging", tc.datasetRead)
			server.OnPattern(http.MethodGet, viewIDPattern, forbiddenResponse)

			output, err := runDiffCmd(t, "-f", dir, "--dataset", tc.datasetFlagArg, "--api-url", server.URL)

			require.Error(t, err)
			var pending *PendingDifferencesError
			assert.False(t, errors.As(err, &pending), "a real 403 must abort, not report pending differences")
			assert.Contains(t, err.Error(), "403")
			assert.Empty(t, output)
		})
	}
}

// A failed read aborts the preview, so diff stops starting new reads once one
// has failed instead of reading every document.
func TestDiff_FetchFailure_StopsStartingReads(t *testing.T) {
	testutil.SetupTestEnv(t)
	dir := t.TempDir()
	const documents = 3 * maxConcurrentReads
	for i := range documents {
		path := filepath.Join(dir, fmt.Sprintf("view-%02d.yaml", i))
		require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf("kind: View\nmetadata:\n  name: view-%02d\n  labels:\n    dash0.com/id: view-%02d\nspec:\n  type: logs\n", i, i)), 0644))
	}
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{StatusCode: http.StatusUnauthorized, BodyFile: testutil.FixtureDashboardsUnauthorized, Validator: testutil.RequireHeaders})

	output, err := runDiffCmd(t, "-f", dir, "--api-url", server.URL)

	require.Error(t, err)
	var pending *PendingDifferencesError
	assert.False(t, errors.As(err, &pending))
	assert.Empty(t, output)
	reads := 0
	for _, request := range server.Requests() {
		if request.Method == http.MethodGet && viewIDPattern.MatchString(request.Path) {
			reads++
		}
	}
	assert.LessOrEqual(t, reads, maxConcurrentReads, "only the reads already running when the first one failed may reach the API")
}

// TestDiff_TeamOriginUnresolvableFallsBackToID pins that a team carrying both
// dash0.com/origin and dash0.com/id is found when the origin cannot be
// resolved but the id can.
//
// SSO/SCIM-provisioned teams carry an origin like
// "saml://samlc_<x>/scim_grp_<y>". `dash0 -X teams get -o yaml` exports both
// labels, but PlanTeam reads only by the origin whenever one is present, and
// the teams endpoint answers that origin with 404 -- verified against a live
// org, both percent-escaped and raw, while the same team's id resolves fine.
// The export of a team that plainly exists is therefore reported as a create.
//
// The origin stays the write key, as documented -- this only widens the read,
// mirroring the id fallback PlanTeam already applies when origin is absent.
func TestDiff_TeamOriginUnresolvableFallsBackToID(t *testing.T) {
	testutil.SetupTestEnv(t)

	const (
		origin = "saml://samlc_3HaZJapoevVMBqEQlS26ECW7SIs/scim_grp_3HrbZLyAWOCqso7zjnuJKfeBa92"
		teamID = "team_01m6dypvgbex58dp137ngtrhhp"
	)

	yamlFile := filepath.Join(t.TempDir(), "team.yaml")
	doc := fmt.Sprintf(`apiVersion: dash0.com/v1alpha1
kind: Dash0Team
metadata:
  name: Dash0PushGroup
  labels:
    dash0.com/id: %s
    dash0.com/origin: %s
spec:
  display:
    name: Dash0PushGroup
  members: []
`, teamID, origin)
	require.NoError(t, os.WriteFile(yamlFile, []byte(doc), 0644))

	var stored dash0api.TeamDefinitionV1Alpha1
	require.NoError(t, dash0yaml.Unmarshal([]byte(doc), &stored))

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	// The identity provider's origin is not an addressable key for this endpoint.
	server.On(http.MethodGet, "/api/teams/"+origin, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		Body:       map[string]any{"error": map[string]any{"code": 404, "message": "not found"}},
	})
	server.On(http.MethodGet, "/api/teams/"+teamID, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{"team": &stored},
		Validator:  testutil.RequireHeaders,
	})

	output, err := runDiffCmd(t, "-f", yamlFile, "--api-url", server.URL)

	require.NoError(t, err, output)
	assert.Contains(t, output, "No differences", "an existing team must not be reported as a create just because its origin is unresolvable")
	assert.Equal(t, 1, countRequests(server, http.MethodGet, "/api/teams/"+teamID), "the id must be tried once the origin read 404s")
}

// TestDiff_TeamOriginWithSlashIsPercentEscapedIntoThePath pins the escaping
// of an origin that contains characters significant in a URL path.
//
// This passes today and is a regression pin, not a defect: the generated
// client escapes path parameters, so "saml://a/b" is sent as
// "saml:%2F%2Fa%2Fb" rather than becoming extra path segments. It is recorded
// next to the fallback test above so the two are not conflated -- escaping is
// correct and still does not make an identity-provider origin resolvable,
// which is why the fallback is needed.
func TestDiff_TeamOriginWithSlashIsPercentEscapedIntoThePath(t *testing.T) {
	testutil.SetupTestEnv(t)

	const origin = "saml://samlc_abc/scim_grp_def"

	yamlFile := filepath.Join(t.TempDir(), "team.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(fmt.Sprintf(`apiVersion: dash0.com/v1alpha1
kind: Dash0Team
metadata:
  name: Dash0PushGroup
  labels:
    dash0.com/origin: %s
spec:
  display:
    name: Dash0PushGroup
  members: []
`, origin)), 0644))

	var escapedPath string
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/teams/"+origin, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		Body:       map[string]any{"error": map[string]any{"code": 404, "message": "not found"}},
		Validator: func(r *http.Request) error {
			escapedPath = r.URL.EscapedPath()
			return nil
		},
	})

	_, _ = runDiffCmd(t, "-f", yamlFile, "--api-url", server.URL)

	assert.Equal(t, "/api/teams/"+url.PathEscape(origin), escapedPath,
		"an origin with path-significant characters must be percent-escaped, not concatenated raw")
}

// TestDiff_JSONKindMatchesApplyDryRun pins that `diff --agent-mode` and
// `apply --dry-run --agent-mode` report the same "kind" for the same document.
//
// The agent-mode JSON of the two commands is documented as the same shape,
// with "op" the only intended difference (diff distinguishes create from
// update). They disagree on "kind" for any document whose `kind:` is not the
// CLI's own display name for the asset: apply emits the document's declared
// kind (asset.Document.Kind), while diff emits a display kind hardcoded per
// branch of planDocument -- so a PersesDashboard reads as "PersesDashboard"
// from apply and "Dashboard" from diff. Consumers such as the sync-assets
// GitHub Action key off this field.
//
// PrometheusRule is deliberately not covered here: apply reports one row for
// the CRD while diff reports one row per alert, which is a documented
// difference in granularity, not a disagreement about one row's kind.
func TestDiff_JSONKindMatchesApplyDryRun(t *testing.T) {
	testutil.SetupTestEnv(t)
	withAgentMode(t, true)

	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "perses.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`apiVersion: perses.dev/v1alpha1
kind: PersesDashboard
metadata:
  name: my-perses-dashboard
spec:
  display:
    name: My Perses Dashboard
  duration: 5m
  panels: {}
`), 0644))

	server := testutil.NewMockServer(t, testutil.FixturesDir())

	diffOutput, _ := runDiffCmd(t, "-f", yamlFile, "--api-url", server.URL)

	applyCmd := apply.NewApplyCmd()
	applyCmd.SetArgs([]string{"-f", yamlFile, "--dry-run", "--api-url", server.URL, "--auth-token", testAuthToken})
	var applyErr error
	applyOutput := testutil.CaptureStdout(t, func() { applyErr = applyCmd.Execute() })
	require.NoError(t, applyErr, applyOutput)

	kindsOf := func(t *testing.T, raw string) []string {
		t.Helper()
		var files []struct {
			Changes []struct {
				Kind string `json:"kind"`
			} `json:"changes"`
		}
		require.NoError(t, json.Unmarshal([]byte(raw), &files), raw)
		var kinds []string
		for _, f := range files {
			for _, c := range f.Changes {
				kinds = append(kinds, c.Kind)
			}
		}
		return kinds
	}

	assert.Equal(t, kindsOf(t, applyOutput), kindsOf(t, diffOutput),
		"diff and apply --dry-run must agree on the kind they report for the same document")
}

// TestDiff_Since_MultiAlertCRDRemovalListsEachAlert pins that removing a whole
// multi-alert PrometheusRule CRD is previewed as one deletion per alert,
// keyed on the derived id each alert's check rule actually lives at.
//
// A CRD with two or more alerting rules upserts each alert at
// "<dash0.com/id>--<slug of '<group> - <alert>'>", so nothing is ever stored
// at the CRD's literal dash0.com/id. Today the preview collapses the removal
// into a single row -- Delete PrometheusRule "<crd name>" (<crd id>) -- naming
// a kind that is not a check rule and an id no check rule has, while hiding
// which alerts go away. The run itself does delete both derived check rules
// (verified against a live org: both carried dash0.com/deleted-at afterwards),
// so the preview under-reports what apply --since actually does.
//
// This is shared code: preview.Plan.AddDeletions backs both `diff --since` and
// `apply --since`, so apply's own output has the same gap.
func TestDiff_Since_MultiAlertCRDRemovalListsEachAlert(t *testing.T) {
	testutil.SetupTestEnv(t)
	repoDir, ref := testutil.BuildGitScenario(t, "prometheus-multi-alert-crd-removal")

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		Body:       map[string]any{"error": map[string]any{"code": 404, "message": "not found"}},
	})

	output, err := runDiffCmd(t, "-f", repoDir, "--since", ref, "--api-url", server.URL)

	require.Error(t, err)
	var pending *PendingDifferencesError
	require.ErrorAs(t, err, &pending)

	assert.Contains(t, output, `Delete Check rule "Alerting - HighLatency" (service-rule-id--alerting-highlatency)`,
		"each alert of a removed multi-alert CRD must be listed at the derived id its check rule lives at")
	assert.Contains(t, output, `Delete Check rule "Alerting - HighErrorRate" (service-rule-id--alerting-higherrorrate)`,
		"each alert of a removed multi-alert CRD must be listed at the derived id its check rule lives at")
	assert.NotContains(t, output, `Delete PrometheusRule "service-alerts" (service-rule-id)`,
		"no check rule is stored at a multi-alert CRD's literal dash0.com/id, so it must not be reported as the thing being deleted")
}

// A dropped 403 for an eval keeps its origin, so the warning names the eval by
// the key that apply would PUT to.
func TestDiff_EvalInDatasetCreatedInSameRunKeepsOrigin(t *testing.T) {
	testutil.SetupTestEnv(t)
	withAgentMode(t, true)
	dir := writeNewDatasetWithView(t)
	require.NoError(t, os.Remove(filepath.Join(dir, "view.yaml")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "eval.yaml"), []byte(`kind: Dash0GenerativeAiEvaluation
metadata:
  name: answer-relevance
  labels:
    dash0.com/origin: eval-origin
spec:
  enabled: true
  judge:
    model: standard
    output:
      kind: boolean
    userPrompt: "{{ gen_ai.input.messages }}"
  match:
    filters:
      - key: gen_ai.operation.name
        operator: is_set
  sample:
    rate: 1
`), 0644))
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/datasets/staging", testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		Body:       map[string]any{"error": map[string]any{"code": 404, "message": "not found"}},
		Validator:  testutil.RequireHeaders,
	})
	server.On(http.MethodGet, "/api/generative-ai-evaluations/eval-origin", forbiddenResponse)

	var output string
	var err error
	stderr := testutil.CaptureStderr(t, func() {
		output, err = runDiffCmd(t, "-f", dir, "--dataset", "staging", "--api-url", server.URL)
	})

	var pending *PendingDifferencesError
	require.ErrorAs(t, err, &pending, output)
	assert.Contains(t, output, `"originOrId": "eval-origin"`)
	assert.Contains(t, stderr, `Eval "answer-relevance" (eval-origin): read denied`)
}
