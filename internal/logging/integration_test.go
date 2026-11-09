//go:build integration

package logging

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
	apiPathLogs              = "/api/logs"
	fixtureQuerySuccess      = "logs/query_success.json"
	fixtureQueryEmpty        = "logs/query_empty.json"
	fixtureLogsUnauthorized  = "dashboards/error_unauthorized.json"
	testLogsAuthToken        = "auth_test_token"
)

// newExperimentalLogsCmd creates a root command with the --experimental persistent
// flag and the logs subcommand attached, mirroring the real command tree.
func newExperimentalLogsCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewLogsCmd())
	return root
}

func TestQueryLogs_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathLogs, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{"logs", "query", "--api-url", server.URL, "--auth-token", testLogsAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "TIMESTAMP")
	assert.Contains(t, output, "SEVERITY")
	assert.Contains(t, output, "BODY")
	assert.Contains(t, output, "Application started successfully")
	assert.Contains(t, output, "Connection timeout")
	assert.Contains(t, output, "INFO")
	assert.Contains(t, output, "ERROR")
}

func TestQueryLogs_Empty(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathLogs, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQueryEmpty,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{"logs", "query", "--api-url", server.URL, "--auth-token", testLogsAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "No log records found.")
}

func TestQueryLogs_OtlpJsonFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathLogs, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{"logs", "query", "--api-url", server.URL, "--auth-token", testLogsAuthToken, "-o", "json"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, `"resourceLogs"`)
	// Verify it's valid JSON
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Contains(t, parsed, "resourceLogs")
}

func TestQueryLogs_CsvFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathLogs, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{"logs", "query", "--api-url", server.URL, "--auth-token", testLogsAuthToken, "-o", "csv"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.GreaterOrEqual(t, len(lines), 4) // header + 3 records
	assert.Equal(t, "otel.log.time,otel.log.severity.range,otel.log.body", lines[0])
	assert.Contains(t, lines[1], "INFO")
	assert.Contains(t, lines[1], "Application started successfully")
}

func TestQueryLogs_WithFilter(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathLogs, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{
		"logs", "query",
		"--api-url", server.URL,
		"--auth-token", testLogsAuthToken,
		"--filter", "service.name is my-service",
	})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	// Verify filter was sent in request body
	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Equal(t, http.MethodPost, req.Method)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(req.Body, &body))
	filters, ok := body["filter"].([]interface{})
	require.True(t, ok, "expected filter array in request body")
	require.Len(t, filters, 1)
	filter := filters[0].(map[string]interface{})
	assert.Equal(t, "service.name", filter["key"])
	assert.Equal(t, "is", filter["operator"])
}

func TestQueryLogs_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathLogs, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   fixtureLogsUnauthorized,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{"logs", "query", "--api-url", server.URL, "--auth-token", "auth_invalid_token"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestQueryLogs_OtlpJsonLimitExceeded(t *testing.T) {
	testutil.SetupTestEnv(t)

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{"logs", "query", "--api-url", "http://unused", "--auth-token", testLogsAuthToken, "-o", "json", "--limit", "200"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "json output is limited to 100 records")
}

func TestQueryLogs_RequestParams(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathLogs, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQueryEmpty,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{
		"logs", "query",
		"--api-url", server.URL,
		"--auth-token", testLogsAuthToken,
		"--from", "now-2h",
		"--to", "now-1h",
		"--dataset", "my-dataset",
		"--limit", "10",
	})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	req := server.LastRequest()
	require.NotNil(t, req)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(req.Body, &body))

	// Verify time range
	timeRange, ok := body["timeRange"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "now-2h", timeRange["from"])
	assert.Equal(t, "now-1h", timeRange["to"])

	// Verify dataset
	assert.Equal(t, "my-dataset", body["dataset"])

	// Verify pagination limit
	pagination, ok := body["pagination"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(10), pagination["limit"])
}

func TestQueryLogs_BackwardCompatWithExperimentalFlag(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathLogs, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{"-X", "logs", "query", "--api-url", server.URL, "--auth-token", testLogsAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "Application started successfully")
}

// The deep link is commentary about the query, not part of its results, so in
// table and CSV output it goes to stderr and never pollutes the piped stream.
func TestQueryLogs_ExplorerURLGoesToStderrInTableOutput(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathLogs, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{"logs", "query", "--api-url", server.URL, "--auth-token", testLogsAuthToken})

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
	assert.Contains(t, stdout, "Application started successfully", "results still go to stdout")
}

func TestQueryLogs_ExplorerURLGoesToStderrInCSVOutput(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathLogs, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{"logs", "query", "-o", "csv", "--api-url", server.URL, "--auth-token", testLogsAuthToken})

	var err error
	var stdout string
	stderr := testutil.CaptureStderr(t, func() {
		stdout = testutil.CaptureStdout(t, func() {
			err = cmd.Execute()
		})
	})

	require.NoError(t, err)
	assert.Contains(t, stderr, "Open this query in Dash0:")
	assert.NotContains(t, stdout, "Open this query in Dash0:",
		"a stray line here would corrupt the CSV for any parser reading stdout")
}

// No Dash0 app host can be derived from an IP address, so the link is omitted
// instead of pointing to a host such as app.0.1.
func TestQueryLogs_NoExplorerURLForIPAddressAPIURL(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathLogs, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})
	ipURL := "http://" + server.Listener.Addr().String()

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{"logs", "query", "-o", "json", "--api-url", ipURL, "--auth-token", testLogsAuthToken})

	var err error
	var stdout string
	stderr := testutil.CaptureStderr(t, func() {
		stdout = testutil.CaptureStdout(t, func() {
			err = cmd.Execute()
		})
	})

	require.NoError(t, err)
	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	assert.Contains(t, payload, "resourceLogs")
	assert.NotContains(t, payload, "dash0ExplorerUrl")
	assert.NotContains(t, stderr, "Open this query in Dash0:")
}

// In JSON output the link is carried in the document itself, as a sibling of
// the OTLP payload, so an agent consuming stdout alone still gets it.
func TestQueryLogs_ExplorerURLIsEmbeddedInJSONOutput(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathLogs, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{"logs", "query", "-o", "json", "--api-url", server.URL, "--auth-token", testLogsAuthToken})

	var err error
	var stdout string
	stderr := testutil.CaptureStderr(t, func() {
		stdout = testutil.CaptureStdout(t, func() {
			err = cmd.Execute()
		})
	})

	require.NoError(t, err)

	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload),
		"stdout must remain a single valid JSON document")
	assert.Contains(t, payload, "dash0ExplorerUrl")
	assert.Contains(t, payload, "resourceLogs", "the OTLP payload stays at the top level")

	var explorerURL string
	require.NoError(t, json.Unmarshal(payload["dash0ExplorerUrl"], &explorerURL))
	assert.Contains(t, explorerURL, "/goto/logs")

	assert.NotContains(t, stderr, "Open this query in Dash0:",
		"the link must not be duplicated onto stderr when it is already in the document")
}

// The link describes the query, not its results, so it is still offered when
// nothing matched — that is often exactly when a human wants to look in the UI.
func TestQueryLogs_ExplorerURLPrintedForEmptyResult(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathLogs, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQueryEmpty,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalLogsCmd()
	cmd.SetArgs([]string{"logs", "query", "--api-url", server.URL, "--auth-token", testLogsAuthToken})

	var err error
	stderr := testutil.CaptureStderr(t, func() {
		_ = testutil.CaptureStdout(t, func() {
			err = cmd.Execute()
		})
	})

	require.NoError(t, err)
	assert.Contains(t, stderr, "Open this query in Dash0:")
}
