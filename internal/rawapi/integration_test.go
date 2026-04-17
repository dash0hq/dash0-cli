//go:build integration

package rawapi

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAuthToken = "auth_test_token"

// newExperimentalAPICmd builds a root command tree mirroring the real CLI,
// with the -X persistent flag and the `api` subcommand attached.
func newExperimentalAPICmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewAPICmd())
	return root
}

func TestAPI_GETAutoInjectsDatasetFromFlag(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/signal-to-metrics/configs", testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{"ok": true},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalAPICmd()
	cmd.SetArgs([]string{
		"-X", "api", "/api/signal-to-metrics/configs",
		"--api-url", server.URL,
		"--auth-token", testAuthToken,
		"--dataset", "production",
	})

	output := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.Execute())
	})

	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Equal(t, "dataset=production", req.Query)
	assert.Equal(t, "Bearer "+testAuthToken, req.Header.Get("Authorization"))
	assert.Contains(t, output, `"ok":true`)
}

func TestAPI_GETWithoutDatasetOptsOut(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/organization/settings", testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{"ok": true},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalAPICmd()
	cmd.SetArgs([]string{
		"-X", "api", "/api/organization/settings",
		"--api-url", server.URL,
		"--auth-token", testAuthToken,
		"--dataset", "",
	})

	testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.Execute())
	})

	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Equal(t, "", req.Query, "expected no query parameters when dataset is opted out")
}

func TestAPI_GETWithQueryInPathPreservesParams(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/signal-to-metrics/configs", testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{"ok": true},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalAPICmd()
	cmd.SetArgs([]string{
		"-X", "api", "/api/signal-to-metrics/configs?limit=50&enabled=true",
		"--api-url", server.URL,
		"--auth-token", testAuthToken,
		"--dataset", "",
	})

	testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.Execute())
	})

	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Contains(t, req.Query, "limit=50")
	assert.Contains(t, req.Query, "enabled=true")
}

func TestAPI_POSTWithFile(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, "/api/signal-to-metrics/configs", testutil.MockResponse{
		StatusCode: http.StatusCreated,
		Body:       map[string]any{"id": "new-id"},
		Validator:  testutil.RequireHeaders,
	})

	dir := t.TempDir()
	bodyPath := dir + "/body.json"
	require.NoError(t, os.WriteFile(bodyPath, []byte(`{"name":"my-config","enabled":true}`), 0o600))

	cmd := newExperimentalAPICmd()
	cmd.SetArgs([]string{
		"-X", "api", "POST", "/api/signal-to-metrics/configs",
		"--api-url", server.URL,
		"--auth-token", testAuthToken,
		"--dataset", "production",
		"-f", bodyPath,
	})

	output := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.Execute())
	})

	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Equal(t, "dataset=production", req.Query)
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	assert.JSONEq(t, `{"name":"my-config","enabled":true}`, string(req.Body))
	assert.Contains(t, output, `"id":"new-id"`)
}

func TestAPI_POSTWithStdin(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, "/api/signal-to-metrics/configs", testutil.MockResponse{
		StatusCode: http.StatusCreated,
		Body:       map[string]any{"id": "stdin-id"},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalAPICmd()
	cmd.SetArgs([]string{
		"-X", "api", "POST", "/api/signal-to-metrics/configs",
		"--api-url", server.URL,
		"--auth-token", testAuthToken,
		"--dataset", "production",
		"-f", "-",
	})
	cmd.SetIn(strings.NewReader(`{"name":"from-stdin"}`))

	testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.Execute())
	})

	req := server.LastRequest()
	require.NotNil(t, req)
	assert.JSONEq(t, `{"name":"from-stdin"}`, string(req.Body))
}

func TestAPI_GETWithFileIsRejected(t *testing.T) {
	testutil.SetupTestEnv(t)

	dir := t.TempDir()
	bodyPath := dir + "/body.json"
	require.NoError(t, os.WriteFile(bodyPath, []byte(`{}`), 0o600))

	cmd := newExperimentalAPICmd()
	cmd.SetArgs([]string{
		"-X", "api", "/api/foo",
		"--api-url", "http://unused",
		"--auth-token", testAuthToken,
		"--dataset", "",
		"-f", bodyPath,
	})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot combine GET with --file")
}

func TestAPI_AuthorizationHeaderIsRejected(t *testing.T) {
	testutil.SetupTestEnv(t)

	cmd := newExperimentalAPICmd()
	cmd.SetArgs([]string{
		"-X", "api", "/api/foo",
		"--api-url", "http://unused",
		"--auth-token", testAuthToken,
		"--dataset", "",
		"-H", "Authorization: Bearer other",
	})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot set Authorization")
}

func TestAPI_DatasetConflictError(t *testing.T) {
	testutil.SetupTestEnv(t)

	cmd := newExperimentalAPICmd()
	cmd.SetArgs([]string{
		"-X", "api", "/api/foo?dataset=baked",
		"--api-url", "http://unused",
		"--auth-token", testAuthToken,
		"--dataset", "production",
	})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dataset is set in both")
}

func TestAPI_NonTwoXXReturnsError(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/foo", testutil.MockResponse{
		StatusCode: http.StatusBadRequest,
		Body:       map[string]any{"error": "bad"},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalAPICmd()
	cmd.SetArgs([]string{
		"-X", "api", "/api/foo",
		"--api-url", server.URL,
		"--auth-token", testAuthToken,
		"--dataset", "",
	})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
	// Body is still streamed to stdout so the caller can inspect it.
	assert.Contains(t, output, `"error":"bad"`)
}

func TestAPI_RequiresExperimental(t *testing.T) {
	testutil.SetupTestEnv(t)

	cmd := newExperimentalAPICmd()
	cmd.SetArgs([]string{
		"api", "/api/foo",
		"--api-url", "http://unused",
		"--auth-token", testAuthToken,
	})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental")
}

func TestAPI_CustomHeaderIsSent(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/api/foo", testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{"ok": true},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalAPICmd()
	cmd.SetArgs([]string{
		"-X", "api", "/api/foo",
		"--api-url", server.URL,
		"--auth-token", testAuthToken,
		"--dataset", "",
		"-H", "X-Request-Id: abc123",
	})

	testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.Execute())
	})

	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Equal(t, "abc123", req.Header.Get("X-Request-Id"))
}

func TestAPI_VerboseModePrintsRequestAndResponse(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, "/api/signal-to-metrics/configs", testutil.MockResponse{
		StatusCode: http.StatusCreated,
		Body:       map[string]any{"id": "new-id"},
		Validator:  testutil.RequireHeaders,
	})

	dir := t.TempDir()
	bodyPath := dir + "/body.json"
	require.NoError(t, os.WriteFile(bodyPath, []byte(`{"name":"verbose-test"}`), 0o600))

	cmd := newExperimentalAPICmd()
	cmd.SetArgs([]string{
		"-X", "api", "POST", "/api/signal-to-metrics/configs",
		"--api-url", server.URL,
		"--auth-token", testAuthToken,
		"--dataset", "",
		"-f", bodyPath,
		"-v",
	})

	var stderrOutput string
	testutil.CaptureStdout(t, func() {
		stderrOutput = testutil.CaptureStderr(t, func() {
			require.NoError(t, cmd.Execute())
		})
	})

	// Request line
	assert.Contains(t, stderrOutput, "> POST ")
	assert.Contains(t, stderrOutput, "/api/signal-to-metrics/configs")

	// Authorization header is redacted
	assert.Contains(t, stderrOutput, "> Authorization: <redacted>")
	assert.NotContains(t, stderrOutput, testAuthToken)

	// Request body is included
	assert.Contains(t, stderrOutput, `{"name":"verbose-test"}`)

	// Response status
	assert.Contains(t, stderrOutput, "< 201")
}

func TestAPI_AbsoluteURLDifferentHostWithoutExplicitTokenRejected(t *testing.T) {
	testutil.SetupTestEnv(t)
	t.Setenv("DASH0_API_URL", "https://api.dev.example")
	t.Setenv("DASH0_AUTH_TOKEN", testAuthToken)

	cmd := newExperimentalAPICmd()
	cmd.SetArgs([]string{
		"-X", "api", "https://api.production.example/endpoint",
		"--dataset", "",
	})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match the profile's api-url host")
	assert.Contains(t, err.Error(), "--auth-token")
}

func TestAPI_AbsoluteURLBypassesAPIPrefix(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, "/direct/path", testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{"ok": true},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalAPICmd()
	cmd.SetArgs([]string{
		"-X", "api", server.URL + "/direct/path",
		"--auth-token", testAuthToken,
		"--api-url", server.URL,
		"--dataset", "",
	})

	testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.Execute())
	})

	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Equal(t, "/direct/path", req.Path)
}
