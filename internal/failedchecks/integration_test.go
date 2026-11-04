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

// runFailedChecksQuery executes the command with the given extra arguments and
// returns stdout and stderr separately.
func runFailedChecksQuery(t *testing.T, serverURL string, extraArgs ...string) (stdout, stderr string) {
	t.Helper()

	args := append([]string{"failed-checks", "query", "--api-url", serverURL, "--auth-token", testFailedChecksToken}, extraArgs...)
	cmd := newFailedChecksCmd()
	cmd.SetArgs(args)

	var err error
	stderr = testutil.CaptureStderr(t, func() {
		stdout = testutil.CaptureStdout(t, func() {
			err = cmd.Execute()
		})
	})
	require.NoError(t, err)
	return stdout, stderr
}

// explorerURLFromJSON extracts and parses the dash0ExplorerUrl field of a JSON
// result document.
func explorerURLFromJSON(t *testing.T, stdout string) *url.URL {
	t.Helper()

	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload),
		"stdout must be a single valid JSON document")
	require.Contains(t, payload, "dash0ExplorerUrl")

	var raw string
	require.NoError(t, json.Unmarshal(payload["dash0ExplorerUrl"], &raw))

	parsed, err := url.Parse(raw)
	require.NoError(t, err, "the emitted link must be a parseable URL")
	return parsed
}

// explorerURLFromStderr extracts and parses the link from human-readable output,
// which prints it on its own indented line below a label.
func explorerURLFromStderr(t *testing.T, stderr string) *url.URL {
	t.Helper()

	require.Contains(t, stderr, "Open this query in Dash0:")

	var raw string
	for _, line := range strings.Split(stderr, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "http") {
			raw = trimmed
			break
		}
	}
	require.NotEmpty(t, raw, "stderr must carry a URL line below the label")

	parsed, err := url.Parse(raw)
	require.NoError(t, err, "the emitted link must be a parseable URL")
	return parsed
}

// assertFailedChecksExplorerURL checks everything about the link that does not
// depend on the query: it must point at the failed-checks view of the Dash0 web
// app derived from the API URL.
func assertFailedChecksExplorerURL(t *testing.T, u *url.URL) {
	t.Helper()

	assert.Equal(t, "https", u.Scheme, "the web app is always https, even against a plain-http API URL")
	assert.True(t, strings.HasPrefix(u.Host, "app."),
		"the host is derived from the API URL by prefixing app., got %q", u.Host)
	assert.Equal(t, "/goto/alerting/failed-checks", u.Path)
}

// decodeExplorerFilters unmarshals the filter query parameter, which carries the
// query's filters as a JSON array.
func decodeExplorerFilters(t *testing.T, u *url.URL) []dash0api.DeeplinkFilter {
	t.Helper()

	rawFilter := u.Query().Get("filter")
	require.NotEmpty(t, rawFilter, "the link must carry a filter parameter")

	var filters []dash0api.DeeplinkFilter
	require.NoError(t, json.Unmarshal([]byte(rawFilter), &filters),
		"the filter parameter must be a JSON array the web app can parse")
	return filters
}

func TestQueryFailedChecks_ExplorerURLGoesToStderrInTableOutput(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathFailedChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	stdout, stderr := runFailedChecksQuery(t, server.URL)

	assert.NotContains(t, stdout, "Open this query in Dash0:",
		"the link must stay out of the piped result stream")

	u := explorerURLFromStderr(t, stderr)
	assertFailedChecksExplorerURL(t, u)

	// No filters were passed, so the link scopes only the time range.
	assert.Empty(t, u.Query().Get("filter"))
	assert.Equal(t, "now-15m", u.Query().Get("from"), "the default time range is carried verbatim")
	assert.Equal(t, "now", u.Query().Get("to"))
}

func TestQueryFailedChecks_ExplorerURLIsEmbeddedInJSONOutput(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathFailedChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	stdout, stderr := runFailedChecksQuery(t, server.URL, "-o", "json")

	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	assert.Contains(t, payload, "issues", "the payload stays a sibling of the link")

	u := explorerURLFromJSON(t, stdout)
	assertFailedChecksExplorerURL(t, u)

	assert.NotContains(t, stderr, "Open this query in Dash0:",
		"the link must not be duplicated onto stderr when it is already in the document")
}

// The link must reproduce the query, so each --filter becomes one filter object
// with the same key, operator, and value the user asked for.
func TestQueryFailedChecks_ExplorerURLReproducesFilters(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathFailedChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	stdout, _ := runFailedChecksQuery(t, server.URL, "-o", "json",
		"--filter", "priority is p1",
		"--filter", "dash0.issue.summary contains timeout",
		"--from", "now-6h", "--to", "now-1h")

	u := explorerURLFromJSON(t, stdout)
	assertFailedChecksExplorerURL(t, u)

	assert.Equal(t, []dash0api.DeeplinkFilter{
		{Key: "priority", Operator: "is", Value: "p1"},
		{Key: "dash0.issue.summary", Operator: "contains", Value: "timeout"},
	}, decodeExplorerFilters(t, u), "filters are carried in order, with operators preserved")

	assert.Equal(t, "now-6h", u.Query().Get("from"))
	assert.Equal(t, "now-1h", u.Query().Get("to"))
}

// --status and --active are shortcuts for filters on well-known issue
// attributes. They are expanded before the link is built, so the link must carry
// them as real filter objects rather than dropping them.
func TestQueryFailedChecks_ExplorerURLExpandsStatusAndActiveShortcuts(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathFailedChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	stdout, _ := runFailedChecksQuery(t, server.URL, "-o", "json", "--active", "--status", "critical,degraded")

	u := explorerURLFromJSON(t, stdout)

	assert.Equal(t, []dash0api.DeeplinkFilter{
		// --status becomes one is_one_of filter whose values are space-separated.
		{Key: "dash0.issue.status", Operator: "is_one_of", Value: "critical degraded"},
		// --active means "not yet resolved", i.e. the end time is absent. A
		// no-value operator carries no value at all.
		{Key: "dash0.issue.end_time", Operator: "is_not_set"},
	}, decodeExplorerFilters(t, u))
}

// A user-supplied --filter and the shortcuts combine, rather than one replacing
// the other.
func TestQueryFailedChecks_ExplorerURLCombinesFiltersWithShortcuts(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathFailedChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	stdout, _ := runFailedChecksQuery(t, server.URL, "-o", "json", "--filter", "owner is sre", "--active")

	assert.Equal(t, []dash0api.DeeplinkFilter{
		{Key: "owner", Operator: "is", Value: "sre"},
		{Key: "dash0.issue.end_time", Operator: "is_not_set"},
	}, decodeExplorerFilters(t, explorerURLFromJSON(t, stdout)))
}

// The dataset is part of what a link must reproduce, or it would open the wrong
// data. It is omitted for the default dataset, which the web app opens anyway.
func TestQueryFailedChecks_ExplorerURLCarriesNonDefaultDataset(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathFailedChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	stdout, _ := runFailedChecksQuery(t, server.URL, "-o", "json", "--dataset", "staging")

	assert.Equal(t, "staging", explorerURLFromJSON(t, stdout).Query().Get("dataset"))
}

func TestQueryFailedChecks_ExplorerURLOmitsDefaultDataset(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathFailedChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	stdout, _ := runFailedChecksQuery(t, server.URL, "-o", "json", "--dataset", "default")

	assert.Empty(t, explorerURLFromJSON(t, stdout).Query().Get("dataset"),
		"the web app opens the default dataset without being told to")
}

// The link describes the query, not its results, so it is still offered when
// nothing matched — that is often exactly when a human wants to look in the UI.
func TestQueryFailedChecks_ExplorerURLOfferedForEmptyResult(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathFailedChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQueryEmpty,
		Validator:  testutil.RequireHeaders,
	})

	_, stderr := runFailedChecksQuery(t, server.URL, "--filter", "priority is p1")

	u := explorerURLFromStderr(t, stderr)
	assertFailedChecksExplorerURL(t, u)
	assert.Equal(t, []dash0api.DeeplinkFilter{{Key: "priority", Operator: "is", Value: "p1"}},
		decodeExplorerFilters(t, u), "the link still reproduces the query that matched nothing")
}
