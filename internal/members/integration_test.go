//go:build integration

package members

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
	apiPathMembers = "/api/members"
	testAuthToken  = "auth_test_token"
)

// newExperimentalMembersCmd creates a root command with the --experimental persistent
// flag and the members subcommand attached, mirroring the real command tree.
func newExperimentalMembersCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewMembersCmd())
	return root
}

func TestListMembers_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureMembersListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalMembersCmd()
	cmd.SetArgs([]string{"-X", "members", "list", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "EMAIL")
	assert.Contains(t, output, "Alice Smith")
	assert.Contains(t, output, "alice@example.com")
	assert.Contains(t, output, "Bob Jones")
	assert.Contains(t, output, "Carol Williams")
}

func TestListMembers_Empty(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureMembersListEmpty,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalMembersCmd()
	cmd.SetArgs([]string{"-X", "members", "list", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "No members found.")
}

func TestListMembers_JSON(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureMembersListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalMembersCmd()
	cmd.SetArgs([]string{"-X", "members", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	var parsed []interface{}
	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Len(t, parsed, 3)
}

func TestListMembers_CSV(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureMembersListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalMembersCmd()
	cmd.SetArgs([]string{"-X", "members", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "csv"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.GreaterOrEqual(t, len(lines), 4) // header + 3 members
	assert.Equal(t, "name,email,id,url", lines[0])
	assert.Contains(t, lines[1], "Alice Smith")
}

func TestListMembers_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   testutil.FixtureMembersUnauthorized,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalMembersCmd()
	cmd.SetArgs([]string{"-X", "members", "list", "--api-url", server.URL, "--auth-token", "auth_invalid"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestListMembers_RequiresExperimental(t *testing.T) {
	testutil.SetupTestEnv(t)

	cmd := newExperimentalMembersCmd()
	cmd.SetArgs([]string{"members", "list", "--api-url", "http://unused", "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental")
}

func TestInviteMember_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalMembersCmd()
	cmd.SetArgs([]string{"-X", "members", "invite", "user@example.com", "--role", "basic_member", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "Invitation sent to user@example.com")
}

func TestInviteMultipleMembers_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalMembersCmd()
	cmd.SetArgs([]string{"-X", "members", "invite", "user1@example.com", "user2@example.com", "--role", "basic_member", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "Invitations sent to 2 email addresses")
}

func TestInviteMember_InvalidRole(t *testing.T) {
	testutil.SetupTestEnv(t)

	cmd := newExperimentalMembersCmd()
	cmd.SetArgs([]string{"-X", "members", "invite", "user@example.com", "--role", "superadmin", "--api-url", "http://unused", "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown role")
	assert.Contains(t, err.Error(), "superadmin")
}

func TestRemoveMember_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureMembersListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodDelete, regexp.MustCompile(`^/api/members/[^/]+$`), testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalMembersCmd()
	cmd.SetArgs([]string{"-X", "members", "remove", "user_member1", "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "Member user_member1 removed")
}

func TestRemoveMember_WithEmail(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureMembersListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodDelete, regexp.MustCompile(`^/api/members/[^/]+$`), testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalMembersCmd()
	cmd.SetArgs([]string{"-X", "members", "remove", "alice@example.com", "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "alice@example.com")
	assert.Contains(t, output, "m1-0000-0000-0000-000000000001")
	assert.Contains(t, output, "removed")
}

func TestRemoveMember_WithEmailNotFound(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureMembersListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalMembersCmd()
	cmd.SetArgs([]string{"-X", "members", "remove", "unknown@example.com", "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), `no member found with email "unknown@example.com"`)
}

// TestRemoveMember_ForceIdempotentOn404 asserts that when the API returns
// 404 for a resolvable member, `remove --force` treats it as idempotent
// success — the member is already gone. Companion coverage to the
// dashboards/teams delete tests; issue #217 promises the same behavior on
// every `delete` and `remove`-shaped path.
func TestRemoveMember_ForceIdempotentOn404(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureMembersListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodDelete, regexp.MustCompile(`^/api/members/[^/]+$`), testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		Body:       map[string]any{"error": map[string]any{"code": 404, "message": "not found"}},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalMembersCmd()
	cmd.SetArgs([]string{"-X", "members", "remove", "user_member1", "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	stderr := testutil.CaptureStderr(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err, "expected exit 0 with --force on 404")
	assert.Contains(t, stderr, "was already deleted")
	assert.Contains(t, stderr, "user_member1")
}

// TestRemoveMember_ForceContinuesOn404Across asserts that a 404 on one
// member does not abort the whole `--force` call — the loop keeps going
// and the remaining members are still processed.
func TestRemoveMember_ForceContinuesOn404Across(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathMembers, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureMembersListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	// user_member1 is already gone (404); user_member2 succeeds.
	server.On(http.MethodDelete, apiPathMembers+"/user_member1", testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		Body:       map[string]any{"error": map[string]any{"code": 404, "message": "not found"}},
		Validator:  testutil.RequireHeaders,
	})
	server.On(http.MethodDelete, apiPathMembers+"/user_member2", testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalMembersCmd()
	cmd.SetArgs([]string{"-X", "members", "remove", "user_member1", "user_member2", "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

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
	assert.Contains(t, stdout, "removed")
}
