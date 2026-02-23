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
	fixtureGetSuccess         = "traces/get_success.json"
	fixtureGetWithLinks       = "traces/get_with_links.json"
	fixtureTracesUnauthorized = "dashboards/error_unauthorized.json"
	testTracesAuthToken       = "auth_test_token"
)

func newExperimentalTracesCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewTracesCmd())
	return root
}

func TestGetTrace_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathSpans, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTracesCmd()
	cmd.SetArgs([]string{"-X", "traces", "get", "0af7651916cd43dd8448eb211c80319c",
		"--api-url", server.URL, "--auth-token", testTracesAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "TIMESTAMP")
	assert.Contains(t, output, "SPAN ID")
	assert.Contains(t, output, "PARENT ID")
	assert.Contains(t, output, "SPAN NAME")
	assert.Contains(t, output, "TRACE ID")
	assert.Contains(t, output, "GET /api/users")
	assert.Contains(t, output, "SELECT * FROM users")
	assert.Contains(t, output, "serialize response")
}

func TestGetTrace_CsvFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathSpans, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTracesCmd()
	cmd.SetArgs([]string{"-X", "traces", "get", "0af7651916cd43dd8448eb211c80319c",
		"--api-url", server.URL, "--auth-token", testTracesAuthToken, "-o", "csv"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.GreaterOrEqual(t, len(lines), 4) // header + 3 spans
	assert.Equal(t, "otel.span.start_time,otel.span.duration,otel.trace.id,otel.span.id,otel.parent.id,otel.span.name,otel.span.status.code,service.name,otel.span.links", lines[0])
}

func TestGetTrace_OtlpJsonFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathSpans, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTracesCmd()
	cmd.SetArgs([]string{"-X", "traces", "get", "0af7651916cd43dd8448eb211c80319c",
		"--api-url", server.URL, "--auth-token", testTracesAuthToken, "-o", "json"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Contains(t, parsed, "resourceSpans")
}

func TestGetTrace_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathSpans, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   fixtureTracesUnauthorized,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTracesCmd()
	cmd.SetArgs([]string{"-X", "traces", "get", "0af7651916cd43dd8448eb211c80319c",
		"--api-url", server.URL, "--auth-token", "auth_invalid_token"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestGetTrace_EmptyResult(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathSpans, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   "spans/query_empty.json",
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalTracesCmd()
	cmd.SetArgs([]string{"-X", "traces", "get", "0af7651916cd43dd8448eb211c80319c",
		"--api-url", server.URL, "--auth-token", testTracesAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "No spans found for this trace.")
}
