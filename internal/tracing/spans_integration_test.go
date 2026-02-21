//go:build integration

package tracing

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
	apiPathSpans             = "/api/spans"
	fixtureQuerySuccess      = "spans/query_success.json"
	fixtureQueryEmpty        = "spans/query_empty.json"
	fixtureSpansUnauthorized = "dashboards/error_unauthorized.json"
	testSpansAuthToken       = "auth_test_token"
)

// newExperimentalSpansCmd creates a root command with the --experimental persistent
// flag and the spans subcommand attached, mirroring the real command tree.
func newExperimentalSpansCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewSpansCmd())
	return root
}

func TestQuerySpans_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathSpans, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireAuthHeader,
	})

	cmd := newExperimentalSpansCmd()
	cmd.SetArgs([]string{"-X", "spans", "query", "--api-url", server.URL, "--auth-token", testSpansAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "TIMESTAMP")
	assert.Contains(t, output, "DURATION")
	assert.Contains(t, output, "SPAN NAME")
	assert.Contains(t, output, "STATUS")
	assert.Contains(t, output, "SERVICE NAME")
	assert.Contains(t, output, "PARENT ID")
	assert.Contains(t, output, "TRACE ID")
	assert.Contains(t, output, "GET /api/users")
	assert.Contains(t, output, "SELECT * FROM users")
	assert.Contains(t, output, "POST /api/orders")
}

func TestQuerySpans_Empty(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathSpans, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQueryEmpty,
		Validator:  testutil.RequireAuthHeader,
	})

	cmd := newExperimentalSpansCmd()
	cmd.SetArgs([]string{"-X", "spans", "query", "--api-url", server.URL, "--auth-token", testSpansAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "No spans found.")
}

func TestQuerySpans_OtlpJsonFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathSpans, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireAuthHeader,
	})

	cmd := newExperimentalSpansCmd()
	cmd.SetArgs([]string{"-X", "spans", "query", "--api-url", server.URL, "--auth-token", testSpansAuthToken, "-o", "json"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, `"resourceSpans"`)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Contains(t, parsed, "resourceSpans")
}

func TestQuerySpans_CsvFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathSpans, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireAuthHeader,
	})

	cmd := newExperimentalSpansCmd()
	cmd.SetArgs([]string{"-X", "spans", "query", "--api-url", server.URL, "--auth-token", testSpansAuthToken, "-o", "csv"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.GreaterOrEqual(t, len(lines), 4) // header + 3 spans
	assert.Equal(t, "otel.span.start_time,otel.span.duration,otel.span.name,otel.span.status.code,service.name,otel.parent.id,otel.trace.id,otel.span.links", lines[0])
	assert.Contains(t, lines[1], "GET /api/users")
}

func TestQuerySpans_WithFilter(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathSpans, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQuerySuccess,
		Validator:  testutil.RequireAuthHeader,
	})

	cmd := newExperimentalSpansCmd()
	cmd.SetArgs([]string{
		"-X", "spans", "query",
		"--api-url", server.URL,
		"--auth-token", testSpansAuthToken,
		"--filter", "service.name is my-service",
	})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

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

func TestQuerySpans_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathSpans, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   fixtureSpansUnauthorized,
		Validator:  testutil.RequireAuthHeader,
	})

	cmd := newExperimentalSpansCmd()
	cmd.SetArgs([]string{"-X", "spans", "query", "--api-url", server.URL, "--auth-token", "auth_invalid_token"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestQuerySpans_OtlpJsonLimitExceeded(t *testing.T) {
	testutil.SetupTestEnv(t)

	cmd := newExperimentalSpansCmd()
	cmd.SetArgs([]string{"-X", "spans", "query", "--api-url", "http://unused", "--auth-token", testSpansAuthToken, "-o", "json", "--limit", "200"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "json output is limited to 100 records")
}

func TestQuerySpans_RequestParams(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathSpans, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureQueryEmpty,
		Validator:  testutil.RequireAuthHeader,
	})

	cmd := newExperimentalSpansCmd()
	cmd.SetArgs([]string{
		"-X", "spans", "query",
		"--api-url", server.URL,
		"--auth-token", testSpansAuthToken,
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

	timeRange, ok := body["timeRange"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "now-2h", timeRange["from"])
	assert.Equal(t, "now-1h", timeRange["to"])
	assert.Equal(t, "my-dataset", body["dataset"])

	pagination, ok := body["pagination"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(10), pagination["limit"])
}
