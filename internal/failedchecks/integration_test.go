//go:build integration

package failedchecks

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiPathFailedChecks   = "/api/alerting/failed-checks"
	fixtureQuerySuccess   = "failedchecks/query_success.json"
	fixtureQueryEmpty     = "failedchecks/query_empty.json"
	fixtureUnauthorized   = "dashboards/error_unauthorized.json"
	testFailedChecksToken = "auth_test_token"
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
	assert.Contains(t, output, "STATUS")
	assert.Contains(t, output, "BLOCK DEPLOYMENTS")
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

func TestQueryFailedChecks_WithStatus(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathFailedChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newFailedChecksCmd()
	cmd.SetArgs([]string{"failed-checks", "query", "--status", "critical,degraded", "--api-url", server.URL, "--auth-token", testFailedChecksToken})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	reqs := server.Requests()
	require.Len(t, reqs, 1)
	var body dash0api.GetFailedChecksRequest
	require.NoError(t, json.Unmarshal(reqs[0].Body, &body))
	require.NotNil(t, body.Filter)
	require.Len(t, *body.Filter, 1, "expected exactly one filter (status)")
	assert.Equal(t, "dash0.issue.status", string((*body.Filter)[0].Key))
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
	cmd.SetArgs([]string{"failed-checks", "query", "--status", "critical", "--active", "--api-url", server.URL, "--auth-token", testFailedChecksToken})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	reqs := server.Requests()
	require.Len(t, reqs, 1)
	var body dash0api.GetFailedChecksRequest
	require.NoError(t, json.Unmarshal(reqs[0].Body, &body))
	require.NotNil(t, body.Filter)
	require.Len(t, *body.Filter, 2, "expected status + active filters")
	assert.Equal(t, "dash0.issue.status", string((*body.Filter)[0].Key))
	assert.Equal(t, "dash0.issue.end_time", string((*body.Filter)[1].Key))
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
	assert.Contains(t, output, "dash0.issue.check_rule_name,dash0.issue.status,dash0.issue.start_time,dash0.issue.summary")
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
	errMsg := strings.ToLower(err.Error())
	assert.True(t,
		strings.Contains(errMsg, "auth") || strings.Contains(errMsg, "401"),
		"expected auth/401-related error, got: %s", err.Error(),
	)
}

func TestQueryFailedChecks_ExplorerURLGoesToStderrInTableOutput(t *testing.T) {
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
	var stdout string
	stderr := testutil.CaptureStderr(t, func() {
		stdout = testutil.CaptureStdout(t, func() {
			err = cmd.Execute()
		})
	})

	require.NoError(t, err)
	assert.Contains(t, stderr, "Open this query in Dash0:")
	assert.NotContains(t, stdout, "Open this query in Dash0:")
}

func TestQueryFailedChecks_ExplorerURLIsEmbeddedInJSONOutput(t *testing.T) {
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

	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(output), &payload))
	assert.Contains(t, payload, "dash0ExplorerUrl")
	assert.Contains(t, payload, "issues")
}

// --status and --active are shortcuts for filters on well-known issue
// attributes, and are expanded before the link is built, so they are carried
// like any other filter.
func TestQueryFailedChecks_ExplorerURLCarriesStatusAndActiveShortcuts(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathFailedChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newFailedChecksCmd()
	cmd.SetArgs([]string{"failed-checks", "query", "-o", "json", "--active", "--status", "critical",
		"--api-url", server.URL, "--auth-token", testFailedChecksToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)

	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(output), &payload))

	var explorerURL string
	require.NoError(t, json.Unmarshal(payload["dash0ExplorerUrl"], &explorerURL))

	decoded, decodeErr := url.QueryUnescape(explorerURL)
	require.NoError(t, decodeErr)
	assert.Contains(t, decoded, "dash0.issue.status")
	assert.Contains(t, decoded, "dash0.issue.end_time")
}
