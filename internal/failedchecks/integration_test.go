//go:build integration

package failedchecks

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiPathFailedChecks     = "/api/alerting/failed-checks"
	fixtureQuerySuccess     = "failedchecks/query_success.json"
	fixtureQueryEmpty       = "failedchecks/query_empty.json"
	fixtureUnauthorized     = "dashboards/error_unauthorized.json"
	testFailedChecksToken   = "auth_test_token"
)

func newFailedChecksCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(NewFailedChecksCmd())
	return root
}

func TestQueryFailedChecks_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathFailedChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newFailedChecksCmd()
	cmd.SetArgs([]string{"failed-checks", "query", "--api-url", server.URL, "--auth-token", testFailedChecksToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "CHECK RULE")
	assert.Contains(t, output, "PRIORITY")
	assert.Contains(t, output, "STATUS")
	assert.Contains(t, output, "BLOCK DEPLOYMENTS")
	assert.Contains(t, output, "p1")
	assert.Contains(t, output, "critical")
}

func TestQueryFailedChecks_ListAlias(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathFailedChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newFailedChecksCmd()
	cmd.SetArgs([]string{"failed-checks", "list", "--api-url", server.URL, "--auth-token", testFailedChecksToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "BLOCK DEPLOYMENTS")
}

func TestQueryFailedChecks_Empty(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathFailedChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQueryEmpty,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newFailedChecksCmd()
	cmd.SetArgs([]string{"failed-checks", "query", "--api-url", server.URL, "--auth-token", testFailedChecksToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "No failed checks found.")
}

func TestQueryFailedChecks_WithPriority(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathFailedChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newFailedChecksCmd()
	cmd.SetArgs([]string{"failed-checks", "query", "--priority", "p1,p2", "--api-url", server.URL, "--auth-token", testFailedChecksToken})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	reqs := server.Requests()
	require.Len(t, reqs, 1)
	var body failedChecksRequest
	require.NoError(t, json.Unmarshal(reqs[0].Body, &body))
	require.Len(t, body.Filter, 1, "expected exactly one filter (priority)")
	assert.Equal(t, "priority", string(body.Filter[0].Key))
}

func TestQueryFailedChecks_WithActive(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathFailedChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newFailedChecksCmd()
	cmd.SetArgs([]string{"failed-checks", "query", "--priority", "p1", "--active", "--api-url", server.URL, "--auth-token", testFailedChecksToken})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	reqs := server.Requests()
	require.Len(t, reqs, 1)
	var body failedChecksRequest
	require.NoError(t, json.Unmarshal(reqs[0].Body, &body))
	require.Len(t, body.Filter, 2, "expected priority + active filters")
	assert.Equal(t, "priority", string(body.Filter[0].Key))
	assert.Equal(t, "dash0.issue.end_time", string(body.Filter[1].Key))
}

func TestQueryFailedChecks_JSON(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathFailedChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newFailedChecksCmd()
	cmd.SetArgs([]string{"failed-checks", "query", "-o", "json", "--api-url", server.URL, "--auth-token", testFailedChecksToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(strings.TrimSpace(output), "{"), "expected JSON output")
	assert.Contains(t, output, "issues")
}

func TestQueryFailedChecks_CSV(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathFailedChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newFailedChecksCmd()
	cmd.SetArgs([]string{"failed-checks", "query", "-o", "csv", "--api-url", server.URL, "--auth-token", testFailedChecksToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "check_rule,priority,status,started,summary,id")
	assert.Contains(t, output, "BLOCK DEPLOYMENTS")
}

func TestQueryFailedChecks_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathFailedChecks, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   fixtureUnauthorized,
	})

	cmd := newFailedChecksCmd()
	cmd.SetArgs([]string{"failed-checks", "query", "--api-url", server.URL, "--auth-token", "auth_bad"})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}
