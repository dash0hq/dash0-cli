//go:build integration

package teams

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiPathTeams   = "/api/teams"
	apiPathMembers = "/api/members"
	testAuthToken  = "auth_test_token"
)

// newExperimentalTeamsCmd creates a root command with the --experimental persistent
// flag and the teams subcommand attached, mirroring the real command tree.
func newExperimentalTeamsCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewTeamsCmd())
	return root
}

func TestListTeams_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathTeams, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureTeamsListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "list", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "ID")
	assert.Contains(t, output, "MEMBERS")
	assert.Contains(t, output, "Backend Team")
	assert.Contains(t, output, "Frontend Team")
}

func TestListTeams_Empty(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathTeams, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureTeamsListEmpty,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "list", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "No teams found.")
}

func TestListTeams_JSON(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathTeams, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureTeamsListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	var parsed []interface{}
	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Len(t, parsed, 2)
}

func TestListTeams_CSV(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathTeams, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureTeamsListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "csv"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.GreaterOrEqual(t, len(lines), 3) // header + 2 teams
	assert.Equal(t, "name,id,members,origin,url", lines[0])
	assert.Contains(t, lines[1], "Backend Team")
}

func TestListTeams_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathTeams, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   testutil.FixtureTeamsUnauthorized,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "list", "--api-url", server.URL, "--auth-token", "auth_invalid"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestListTeams_RequiresExperimental(t *testing.T) {
	testutil.SetupTestEnv(t)

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"teams", "list", "--api-url", "http://unused", "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental")
}

func TestGetTeam_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, regexp.MustCompile(`^/api/teams/[^/]+$`), testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureTeamsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "get", "a1b2c3d4-5678-90ab-cdef-1234567890ab", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "Kind:        Team")
	assert.Contains(t, output, "Name:        Backend Team")
	assert.Contains(t, output, "Members:     2")
	assert.Contains(t, output, "Alice Smith")
	assert.Contains(t, output, "Bob Jones")
}

func TestGetTeam_JSON(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, regexp.MustCompile(`^/api/teams/[^/]+$`), testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureTeamsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	// The JSON path resolves member IDs to emails via GET /api/members.
	server.On(http.MethodGet, "/api/members", testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureMembersListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "get", "a1b2c3d4-5678-90ab-cdef-1234567890ab", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	// New shape: `teams get -o json` emits the CRD envelope only
	// (apiVersion/kind/metadata/spec), not the enriched {team, members, ...}
	// wrapper. See .chloggen/teams-get-json-envelope.yaml.
	assert.Contains(t, parsed, "kind")
	assert.Contains(t, parsed, "metadata")
	assert.Contains(t, parsed, "spec")
	assert.NotContains(t, parsed, "team")
}

func TestGetTeam_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, regexp.MustCompile(`^/api/teams/[^/]+$`), testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureTeamsNotFound,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "get", "nonexistent-id", "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestCreateTeam_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathTeams, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureTeamsCreateSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "create", "New Team", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "Team \"New Team\" created")
}

// assertPUTPath finds a PUT request in the recorded stream that targets the
// given path. Tolerates a trailing /api/members lookup (invoked when the
// GetTeam pre-check populates the "before" state for the apply diff).
func assertPUTPath(t *testing.T, requests []testutil.RecordedRequest, wantPath, msg string) {
	t.Helper()
	for _, req := range requests {
		if req.Method == http.MethodPut && req.Path == wantPath {
			return
		}
	}
	seen := make([]string, 0, len(requests))
	for _, req := range requests {
		seen = append(seen, req.Method+" "+req.Path)
	}
	t.Fatalf("%s\nwant: PUT %s\ngot requests: %v", msg, wantPath, seen)
}

// assertPOSTPath finds a POST request in the recorded stream that targets
// the given path. Mirrors assertPUTPath for the fall-through-to-create path.
func assertPOSTPath(t *testing.T, requests []testutil.RecordedRequest, wantPath, msg string) {
	t.Helper()
	for _, req := range requests {
		if req.Method == http.MethodPost && req.Path == wantPath {
			return
		}
	}
	seen := make([]string, 0, len(requests))
	for _, req := range requests {
		seen = append(seen, req.Method+" "+req.Path)
	}
	t.Fatalf("%s\nwant: POST %s\ngot requests: %v", msg, wantPath, seen)
}

// TestCreateTeamFromFile_UpsertByID asserts that a declarative team YAML
// carrying only a dash0.com/id label (no origin) routes to PUT
// /api/teams/{id} rather than POST when the team exists in the target env.
// Regression coverage for the "download team from platform UI, reapply via
// CLI" idempotency loop: the UI download carries id but not origin, so
// upsert must fall back to id.
func TestCreateTeamFromFile_UpsertByID(t *testing.T) {
	testutil.SetupTestEnv(t)

	const teamID = "team_01k5vpx97efdnrkqan15b41k84"
	teamsIDPattern := regexp.MustCompile(`^/api/teams/team_[^/]+$`)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, teamsIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureTeamsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPut, teamsIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureTeamsCreateSuccess,
		Validator:  testutil.RequireHeaders,
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "team.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: dash0.com/v1alpha1
kind: Dash0Team
metadata:
  name: some-new-team
  labels:
    dash0.com/id: `+teamID+`
    dash0.com/source: ui
spec:
  display:
    color:
      from: "#fb7185"
      to: "#be123c"
    name: Some new team
  members: []
`), 0644)
	require.NoError(t, err)

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	assertPUTPath(t, server.Requests(), "/api/teams/"+teamID, "expected PUT-by-id, got POST — the id label should route to upsert")
}

// TestCreateTeamFromFile_UpsertByID_FallsBackToPOSTWhenNotFound asserts that
// when the id in the input YAML does not exist in the target environment
// (cross-environment apply — the classic "download from org A, apply to
// org B" case), the CLI falls back to POST instead of returning a 404 from
// PUT. Without this, `dash0 apply` is not idempotent across environments
// and users see a scary "Forbidden" error on what should be a fresh create.
func TestCreateTeamFromFile_UpsertByID_FallsBackToPOSTWhenNotFound(t *testing.T) {
	testutil.SetupTestEnv(t)

	const teamID = "team_01k5vpx97efdnrkqan15b41k84"
	teamsIDPattern := regexp.MustCompile(`^/api/teams/team_[^/]+$`)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	// Pre-check GET returns 404 — the id belongs to a different org.
	server.OnPattern(http.MethodGet, teamsIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureTeamsNotFound,
		Validator:  testutil.RequireHeaders,
	})
	// POST fallback returns the freshly-created team.
	server.On(http.MethodPost, apiPathTeams, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureTeamsCreateSuccess,
		Validator:  testutil.RequireHeaders,
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "team.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: dash0.com/v1alpha1
kind: Dash0Team
metadata:
  name: some-new-team
  labels:
    dash0.com/id: `+teamID+`
    dash0.com/source: ui
spec:
  display:
    color:
      from: "#fb7185"
      to: "#be123c"
    name: Some new team
  members: []
`), 0644)
	require.NoError(t, err)

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err, "cross-env apply must not fail — the id from a different org should trigger POST fallback")

	// No PUT should have been attempted.
	for _, req := range server.Requests() {
		assert.NotEqual(t, http.MethodPut, req.Method, "PUT to unknown id would 404; expected POST fallback instead")
	}
	assertPOSTPath(t, server.Requests(), apiPathTeams, "expected POST fallback after GET 404")
}

// TestCreateTeamFromFile_UpsertByOrigin asserts that a declarative team YAML
// carrying a dash0.com/origin label routes to PUT /api/teams/{origin}.
func TestCreateTeamFromFile_UpsertByOrigin(t *testing.T) {
	testutil.SetupTestEnv(t)

	const origin = "my-team-origin"
	teamsOriginPattern := regexp.MustCompile(`^/api/teams/[^/]+$`)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, teamsOriginPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureTeamsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPut, teamsOriginPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureTeamsCreateSuccess,
		Validator:  testutil.RequireHeaders,
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "team.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: dash0.com/v1alpha1
kind: Dash0Team
metadata:
  name: some-new-team
  labels:
    dash0.com/origin: `+origin+`
spec:
  display:
    color:
      from: "#fb7185"
      to: "#be123c"
    name: Some new team
  members: []
`), 0644)
	require.NoError(t, err)

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	assertPUTPath(t, server.Requests(), "/api/teams/"+origin, "expected PUT to /api/teams/{origin}")
}

// TestCreateTeamFromFile_OriginWinsOverID asserts that when both origin and
// id labels are present, origin is the upsert key.
func TestCreateTeamFromFile_OriginWinsOverID(t *testing.T) {
	testutil.SetupTestEnv(t)

	const origin = "my-team-origin"
	const teamID = "team_01k5vpx97efdnrkqan15b41k84"
	teamsIDPattern := regexp.MustCompile(`^/api/teams/[^/]+$`)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, teamsIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureTeamsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPut, teamsIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureTeamsCreateSuccess,
		Validator:  testutil.RequireHeaders,
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "team.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: dash0.com/v1alpha1
kind: Dash0Team
metadata:
  name: some-new-team
  labels:
    dash0.com/origin: `+origin+`
    dash0.com/id: `+teamID+`
spec:
  display:
    color:
      from: "#fb7185"
      to: "#be123c"
    name: Some new team
  members: []
`), 0644)
	require.NoError(t, err)

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	assertPUTPath(t, server.Requests(), "/api/teams/"+origin, "origin must win over id when both are present")
}

func TestDeleteTeam_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, regexp.MustCompile(`^/api/teams/[^/]+$`), testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "delete", "a1b2c3d4-5678-90ab-cdef-1234567890ab", "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "deleted")
}

// TestDeleteTeam_ForceIdempotentOn404 asserts that `dash0 teams delete
// <id> --force` treats a missing team as idempotent success. Regression
// coverage for issue #217; the reported example is a `teams delete`.
func TestDeleteTeam_ForceIdempotentOn404(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, regexp.MustCompile(`^/api/teams/[^/]+$`), testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   "teams/error_not_found.json",
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "delete", "team_01FAKE", "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	stderr := testutil.CaptureStderr(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err, "expected exit 0 with --force on 404")
	assert.Contains(t, stderr, "was already deleted")
	assert.Contains(t, stderr, "team_01FAKE")
}

func TestAddMembers_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureMembersListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPost, regexp.MustCompile(`^/api/teams/[^/]+/members$`), testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "add-members", "team-1", "user_member1", "user_member2", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "2 members added to team")
}

func TestListTeamMembers_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, regexp.MustCompile(`^/api/teams/[^/]+$`), testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureTeamsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "list-members", "a1b2c3d4-5678-90ab-cdef-1234567890ab", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "EMAIL")
	assert.Contains(t, output, "Alice Smith")
	assert.Contains(t, output, "Bob Jones")
}

func TestListTeamMembers_JSON(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, regexp.MustCompile(`^/api/teams/[^/]+$`), testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureTeamsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "list-members", "a1b2c3d4-5678-90ab-cdef-1234567890ab", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	var parsed []interface{}
	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Len(t, parsed, 2)
}

func TestListTeamMembers_CSV(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, regexp.MustCompile(`^/api/teams/[^/]+$`), testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureTeamsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "list-members", "a1b2c3d4-5678-90ab-cdef-1234567890ab", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "csv"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.GreaterOrEqual(t, len(lines), 3) // header + 2 members
	assert.Equal(t, "name,email,id,url", lines[0])
	assert.Contains(t, lines[1], "Alice Smith")
}

func TestRemoveMembers_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureMembersListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodDelete, regexp.MustCompile(`^/api/teams/[^/]+/members/[^/]+$`), testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "remove-members", "team-1", "user_member1", "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "Member user_member1 removed from team")
}

func TestAddMembers_WithEmail(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureMembersListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPost, regexp.MustCompile(`^/api/teams/[^/]+/members$`), testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "add-members", "team-1", "alice@example.com", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "1 member added to team")
}

func TestAddMembers_WithEmailNotFound(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureMembersListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "add-members", "team-1", "unknown@example.com", "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), `no member found with email "unknown@example.com"`)
}

func TestRemoveMembers_WithEmail(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureMembersListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodDelete, regexp.MustCompile(`^/api/teams/[^/]+/members/[^/]+$`), testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "remove-members", "team-1", "bob@example.com", "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "bob@example.com")
	assert.Contains(t, output, "removed from team")
}

// TestRemoveMembers_ForceIdempotentOn404 asserts that when the team-member
// deletion returns 404, `remove-members --force` treats it as idempotent
// success — the member is already gone from the team. Companion to the
// members-remove test; issue #217 promises the same behavior on every
// `delete` and `remove`-shaped path.
func TestRemoveMembers_ForceIdempotentOn404(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureMembersListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodDelete, regexp.MustCompile(`^/api/teams/[^/]+/members/[^/]+$`), testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		Body:       map[string]any{"error": map[string]any{"code": 404, "message": "not found"}},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "remove-members", "team-1", "user_member1", "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	stderr := testutil.CaptureStderr(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err, "expected exit 0 with --force on 404")
	assert.Contains(t, stderr, "was already deleted")
	assert.Contains(t, stderr, "user_member1")
}

// TestRemoveMembers_ForceContinuesOn404Across asserts that a 404 on one
// team-member removal does not abort the whole `--force` call — the loop
// keeps going and the remaining members are still processed.
func TestRemoveMembers_ForceContinuesOn404Across(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureMembersListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.On(http.MethodDelete, "/api/teams/team-1/members/user_member1", testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		Body:       map[string]any{"error": map[string]any{"code": 404, "message": "not found"}},
		Validator:  testutil.RequireHeaders,
	})
	server.On(http.MethodDelete, "/api/teams/team-1/members/user_member2", testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "remove-members", "team-1", "user_member1", "user_member2", "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	var stdout, stderr string
	stdout = testutil.CaptureStdout(t, func() {
		stderr = testutil.CaptureStderr(t, func() {
			err = cmd.Execute()
		})
	})

	require.NoError(t, err, "one 404 must not abort the whole --force call")
	assert.Contains(t, stderr, "user_member1")
	assert.Contains(t, stderr, "was already deleted")
	// user_member2 succeeded — its success line lands on stdout.
	assert.Contains(t, stdout, "user_member2")
	assert.Contains(t, stdout, "removed from team")
}
