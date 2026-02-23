//go:build integration

package teams

import (
	"encoding/json"
	"net/http"
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
	assert.Contains(t, output, "Kind:    Team")
	assert.Contains(t, output, "Name:    Backend Team")
	assert.Contains(t, output, "Members: 2")
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

	cmd := newExperimentalTeamsCmd()
	cmd.SetArgs([]string{"-X", "teams", "get", "a1b2c3d4-5678-90ab-cdef-1234567890ab", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Contains(t, parsed, "team")
	assert.Contains(t, parsed, "members")
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
