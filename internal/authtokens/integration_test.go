//go:build integration

package authtokens

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiPathAuthTokens = "/api/auth-tokens"
	testAuthToken     = "auth_test_token"
	tokenID           = "6f1c0d1e-5b7a-4c2e-9a51-2b0c6c4b8a10"
	tokenOrigin       = "dash0-cli_ingest"

	tokenYAML = `apiVersion: dash0.com/v1alpha1
kind: Dash0AuthToken
metadata:
  name: Ingest
  labels:
    dash0.com/origin: dash0-cli_ingest
spec:
  actions:
    - ingest:*
`
	allActionsYAML = `apiVersion: dash0.com/v1alpha1
kind: Dash0AuthToken
metadata:
  name: Admin
spec:
  actions:
    - "*"
`
	noOriginYAML = `apiVersion: dash0.com/v1alpha1
kind: Dash0AuthToken
metadata:
  name: Ingest
spec:
  actions:
    - ingest:logs
`
)

var tokenPathPattern = regexp.MustCompile(`^/api/auth-tokens/[^/]+$`)

// newCanary returns a fresh secret per test, so a match can only come from this test's mock server.
func newCanary() string { return "auth_CANARY_" + strings.ReplaceAll(uuid.NewString(), "-", "") }

// tokenBody is an auth token response. A non-empty secret goes in spec.token,
// the way the API answers a create.
func tokenBody(name, secretValue string) map[string]any {
	spec := map[string]any{"actions": []string{"ingest:*"}}
	if secretValue != "" {
		spec["token"] = secretValue
	}
	return map[string]any{
		"apiVersion": "dash0.com/v1alpha1",
		"kind":       "Dash0AuthToken",
		"metadata": map[string]any{
			"name": name,
			"labels": map[string]any{
				"dash0.com/id":     tokenID,
				"dash0.com/origin": tokenOrigin,
				"dash0.com/source": "dash0-cli",
			},
			"annotations": map[string]any{"dash0.com/created-at": "2026-10-01T10:00:00Z"},
		},
		"spec": spec,
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewAuthTokensCmd())
	return root
}

// runCmd runs the command tree, which sees a non-terminal stdout, and captures stdout and stderr.
func runCmd(t *testing.T, serverURL string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetArgs(append(append([]string{"-X", "auth-tokens"}, args...), "--api-url", serverURL, "--auth-token", testAuthToken))
	stderr = testutil.CaptureStderr(t, func() {
		stdout = testutil.CaptureStdout(t, func() {
			err = cmd.Execute()
		})
	})
	return stdout, stderr, err
}

// runCreateOnTerminal calls runCreate the way an interactive terminal would.
func runCreateOnTerminal(t *testing.T, flags *createFlags) (stdout, stderr string, err error) {
	t.Helper()
	var out bytes.Buffer
	stderr = testutil.CaptureStderr(t, func() {
		err = runCreate(context.Background(), flags, &out, true)
	})
	return out.String(), stderr, err
}

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func withAgentMode(t *testing.T, enabled bool) {
	t.Helper()
	previous := agentmode.Enabled
	agentmode.Enabled = enabled
	t.Cleanup(func() { agentmode.Enabled = previous })
}

func requestsWith(server *testutil.MockServer, method string) []testutil.RecordedRequest {
	var out []testutil.RecordedRequest
	for _, request := range server.Requests() {
		if request.Method == method {
			out = append(out, request)
		}
	}
	return out
}

func respond(status int, body any) testutil.MockResponse {
	return testutil.MockResponse{StatusCode: status, Body: body, Validator: testutil.RequireHeaders}
}

var notFound = map[string]any{"error": map[string]any{"code": 404, "message": "The requested auth token does not exist."}}

// newCreateServer answers the origin preflight with 404 and the POST with the canary.
func newCreateServer(t *testing.T, canaryValue string) *testutil.MockServer {
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, tokenPathPattern, respond(http.StatusNotFound, notFound))
	server.On(http.MethodPost, apiPathAuthTokens, respond(http.StatusOK, tokenBody("Ingest", canaryValue)))
	server.OnPattern(http.MethodDelete, tokenPathPattern, respond(http.StatusOK, map[string]any{}))
	return server
}

func TestListAuthTokens_Formats(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathAuthTokens, respond(http.StatusOK, []any{tokenBody("Ingest", "")}))

	cases := map[string][]string{
		"table": {"NAME", "Ingest", tokenID, "ingest:*"},
		"wide":  {"ORIGIN", tokenOrigin},
		"csv":   {"name,id,actions,origin", "Ingest," + tokenID + ",ingest:*," + tokenOrigin},
		"json":  {`"dash0.com/origin": "dash0-cli_ingest"`},
		"yaml":  {"dash0.com/origin: dash0-cli_ingest"},
	}
	for format, want := range cases {
		t.Run(format, func(t *testing.T) {
			stdout, _, err := runCmd(t, server.URL, "list", "-o", format)
			require.NoError(t, err)
			for _, fragment := range want {
				assert.Contains(t, stdout, fragment)
			}
		})
	}
}

func TestListAuthTokens_Empty(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathAuthTokens, respond(http.StatusOK, []any{}))

	stdout, _, err := runCmd(t, server.URL, "list")
	require.NoError(t, err)
	assert.Equal(t, "No auth tokens found.\n", stdout)

	stdout, _, err = runCmd(t, server.URL, "list", "-o", "json")
	require.NoError(t, err)
	assert.JSONEq(t, "[]", stdout)
}

func TestListAuthTokens_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathAuthTokens, testutil.MockResponse{StatusCode: http.StatusUnauthorized, BodyFile: testutil.FixtureDatasetsUnauthorized})

	_, _, err := runCmd(t, server.URL, "list")
	require.ErrorContains(t, err, "authentication failed")
}

func TestAuthTokens_RequireExperimental(t *testing.T) {
	testutil.SetupTestEnv(t)
	cmd := newRootCmd()
	cmd.SetArgs([]string{"auth-tokens", "list", "--api-url", "http://unused", "--auth-token", testAuthToken})
	require.ErrorContains(t, cmd.Execute(), "experimental")
}

func TestGetAuthToken_Formats(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathAuthTokens+"/"+tokenID, respond(http.StatusOK, tokenBody("Ingest", "")))

	stdout, _, err := runCmd(t, server.URL, "get", tokenID)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Name:    Ingest")
	assert.Contains(t, stdout, "Origin:  "+tokenOrigin)
	assert.Contains(t, stdout, "Actions: ingest:*")

	stdout, _, err = runCmd(t, server.URL, "get", tokenID, "-o", "json")
	require.NoError(t, err)
	var decoded dash0api.AuthTokenResponseDefinition
	require.NoError(t, json.Unmarshal([]byte(stdout), &decoded))
	assert.Equal(t, "Ingest", decoded.Metadata.Name)

	stdout, _, err = runCmd(t, server.URL, "get", tokenID, "-o", "yaml")
	require.NoError(t, err)
	assert.Contains(t, stdout, "name: Ingest")
}

func TestGetAuthToken_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, tokenPathPattern, respond(http.StatusNotFound, notFound))

	_, _, err := runCmd(t, server.URL, "get", "missing")
	require.ErrorContains(t, err, `auth token "missing" not found`)
}

func TestCreateAuthToken_NoDestinationMakesNoAPICall(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newCreateServer(t, newCanary())

	_, _, err := runCmd(t, server.URL, "create", "-f", writeYAML(t, tokenYAML))

	require.ErrorContains(t, err, "--secret-file")
	assert.Empty(t, server.Requests())
}

func TestCreateAuthToken_TerminalRules(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newCreateServer(t, newCanary())
	file := writeYAML(t, tokenYAML)

	t.Run("agent mode refuses a terminal", func(t *testing.T) {
		withAgentMode(t, true)
		_, _, err := runCreateOnTerminal(t, &createFlags{ApiUrl: server.URL, AuthToken: testAuthToken, File: file})
		require.ErrorIs(t, err, errNoSecretDestination)
	})
	t.Run("json output refuses a terminal", func(t *testing.T) {
		_, _, err := runCreateOnTerminal(t, &createFlags{ApiUrl: server.URL, AuthToken: testAuthToken, File: file, Output: "json"})
		require.ErrorIs(t, err, errNoSecretDestination)
	})
	assert.Empty(t, server.Requests())
}

func TestCreateAuthToken_PrintsSecretOnceOnTerminal(t *testing.T) {
	testutil.SetupTestEnv(t)
	canaryValue := newCanary()
	server := newCreateServer(t, canaryValue)

	stdout, stderr, err := runCreateOnTerminal(t, &createFlags{ApiUrl: server.URL, AuthToken: testAuthToken, File: writeYAML(t, tokenYAML)})

	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(stdout, canaryValue))
	assert.Contains(t, stdout, `Auth token "Ingest" created`)
	assert.Contains(t, stderr, "shown only once")
	assert.NotContains(t, stderr, canaryValue)
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestCreateAuthToken_SecretPrintFailureDeletesToken(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newCreateServer(t, newCanary())

	var err error
	testutil.CaptureStderr(t, func() {
		err = runCreate(context.Background(), &createFlags{ApiUrl: server.URL, AuthToken: testAuthToken, File: writeYAML(t, tokenYAML)}, failingWriter{}, true)
	})

	require.ErrorContains(t, err, "failed to print the secret")
	assert.ErrorContains(t, err, "was deleted again")
	deletes := requestsWith(server, http.MethodDelete)
	require.Len(t, deletes, 1)
	assert.Equal(t, apiPathAuthTokens+"/"+tokenID, deletes[0].Path)
}

func TestCreateAuthToken_SecretFile(t *testing.T) {
	testutil.SetupTestEnv(t)
	canaryValue := newCanary()
	server := newCreateServer(t, canaryValue)
	secretPath := filepath.Join(t.TempDir(), "token.secret")

	stdout, stderr, err := runCmd(t, server.URL, "create", "-f", writeYAML(t, tokenYAML), "--secret-file", secretPath)

	require.NoError(t, err)
	content, err := os.ReadFile(secretPath)
	require.NoError(t, err)
	assert.Equal(t, canaryValue+"\n", string(content))
	assert.Contains(t, stdout, "Secret written to "+secretPath)
	assert.NotContains(t, stdout+stderr, canaryValue)
	posts := requestsWith(server, http.MethodPost)
	require.Len(t, posts, 1)
	var sent dash0api.AuthTokenDefinition
	require.NoError(t, json.Unmarshal(posts[0].Body, &sent))
	assert.Equal(t, tokenOrigin, *sent.Metadata.Labels.Dash0Comorigin)
}

func TestCreateAuthToken_WithoutOriginSendsNone(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newCreateServer(t, newCanary())

	_, _, err := runCmd(t, server.URL, "create", "-f", writeYAML(t, noOriginYAML), "--secret-file", filepath.Join(t.TempDir(), "s"))

	require.NoError(t, err)
	assert.Empty(t, requestsWith(server, http.MethodGet))
	posts := requestsWith(server, http.MethodPost)
	require.Len(t, posts, 1)
	var sent dash0api.AuthTokenDefinition
	require.NoError(t, json.Unmarshal(posts[0].Body, &sent))
	assert.Empty(t, originOf(&sent.Metadata))
}

func TestCreateAuthToken_ExistingOriginRefused(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathAuthTokens+"/"+tokenOrigin, respond(http.StatusOK, tokenBody("Ingest", "")))
	secretPath := filepath.Join(t.TempDir(), "token.secret")

	_, _, err := runCmd(t, server.URL, "create", "-f", writeYAML(t, tokenYAML), "--secret-file", secretPath)

	require.ErrorContains(t, err, "already exists")
	assert.ErrorContains(t, err, "auth-tokens update")
	assert.Empty(t, requestsWith(server, http.MethodPost))
	assert.Empty(t, requestsWith(server, http.MethodPut))
	_, statErr := os.Lstat(secretPath)
	assert.True(t, os.IsNotExist(statErr), "a failed create must not leave a secret file behind")
}

func TestAuthTokens_AllActionsNeedsItsOwnFlag(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := newCreateServer(t, newCanary())
	file := writeYAML(t, allActionsYAML)

	for _, args := range [][]string{
		{"create", "-f", file, "--secret-file", filepath.Join(t.TempDir(), "s")},
		{"update", tokenID, "-f", file},
	} {
		args := args
		t.Run(args[0], func(t *testing.T) {
			withAgentMode(t, true)
			_, _, err := runCmd(t, server.URL, args...)
			require.ErrorContains(t, err, "--allow-all-actions")
		})
	}
	assert.Empty(t, server.Requests())

	_, _, err := runCmd(t, server.URL, "create", "-f", file, "--secret-file", filepath.Join(t.TempDir(), "s"), "--allow-all-actions")
	require.NoError(t, err)
	assert.Len(t, requestsWith(server, http.MethodPost), 1)
}

func TestCreateAuthToken_DryRun(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	stdout, _, err := runCmd(t, server.URL, "create", "-f", writeYAML(t, tokenYAML), "--dry-run", "--secret-file", filepath.Join(t.TempDir(), "s"))

	require.NoError(t, err)
	assert.Equal(t, "Dry run: auth token definition is valid\n", stdout)
	assert.Empty(t, server.Requests())
}

func TestCreateAuthToken_DryRunNeedsNoSecretDestination(t *testing.T) {
	testutil.SetupTestEnv(t)
	withAgentMode(t, true)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	stdout, _, err := runCmd(t, server.URL, "create", "-f", writeYAML(t, tokenYAML), "--dry-run")

	require.NoError(t, err)
	assert.Equal(t, "Dry run: auth token definition is valid\n", stdout)
	assert.Empty(t, server.Requests())
}

func TestCompensate_DeletesAfterCancel(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, tokenPathPattern, respond(http.StatusOK, map[string]any{}))
	var created dash0api.AuthTokenResponseDefinition
	data, err := json.Marshal(tokenBody("Ingest", ""))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &created))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = compensate(ctx, server.URL, testAuthToken, &created, fmt.Errorf("interrupted"))

	require.ErrorContains(t, err, "was deleted again")
	deletes := requestsWith(server, http.MethodDelete)
	require.Len(t, deletes, 1)
	assert.Equal(t, apiPathAuthTokens+"/"+tokenID, deletes[0].Path)
}

func TestUpdateAuthToken_NotFoundSendsNoPut(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, tokenPathPattern, respond(http.StatusNotFound, notFound))

	_, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, tokenYAML))

	require.ErrorContains(t, err, "not found")
	assert.Empty(t, requestsWith(server, http.MethodPut))
}

func TestUpdateAuthToken_PutsByStoredIDAndShowsDiff(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathAuthTokens+"/"+tokenOrigin, respond(http.StatusOK, tokenBody("Ingest", "")))
	server.On(http.MethodPut, apiPathAuthTokens+"/"+tokenID, respond(http.StatusOK, tokenBody("Ingest (renamed)", "")))

	stdout, _, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, strings.Replace(tokenYAML, "name: Ingest", "name: Ingest (renamed)", 1)))

	require.NoError(t, err)
	assert.Contains(t, stdout, "-  name: Ingest\n")
	assert.Contains(t, stdout, "+  name: Ingest (renamed)\n")
	puts := requestsWith(server, http.MethodPut)
	require.Len(t, puts, 1)
	assert.Equal(t, apiPathAuthTokens+"/"+tokenID, puts[0].Path)
}

func TestUpdateAuthToken_IDMismatchWithoutArgumentNamesTheFile(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathAuthTokens+"/"+tokenOrigin, respond(http.StatusOK, tokenBody("Ingest", "")))
	otherID := "0b7e5d2a-1c3f-4e8a-9b6d-2f4a8c1e7d90"
	file := writeYAML(t, strings.Replace(tokenYAML, "  labels:\n", "  labels:\n    dash0.com/id: "+otherID+"\n", 1))

	_, _, err := runCmd(t, server.URL, "update", "-f", file)

	require.ErrorContains(t, err, `"`+tokenOrigin+`" from the file is auth token "`+tokenID+`", but the file names auth token "`+otherID+`"`)
	assert.NotContains(t, err.Error(), "argument")
	assert.Empty(t, requestsWith(server, http.MethodPut))
}

func TestUpdateAuthToken_PutThatCreatesIsRolledBack(t *testing.T) {
	testutil.SetupTestEnv(t)
	canaryValue := newCanary()
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, tokenPathPattern, respond(http.StatusOK, tokenBody("Ingest", "")))
	server.OnPattern(http.MethodPut, tokenPathPattern, respond(http.StatusOK, tokenBody("Ingest", canaryValue)))
	server.OnPattern(http.MethodDelete, tokenPathPattern, respond(http.StatusOK, map[string]any{}))

	stdout, stderr, err := runCmd(t, server.URL, "update", "-f", writeYAML(t, tokenYAML))

	require.ErrorContains(t, err, "deleted while it was being updated")
	assert.ErrorContains(t, err, "was deleted again")
	deletes := requestsWith(server, http.MethodDelete)
	require.Len(t, deletes, 1)
	assert.Equal(t, apiPathAuthTokens+"/"+tokenID, deletes[0].Path)
	assert.NotContains(t, stdout+stderr+err.Error(), canaryValue)
}

func TestDeleteAuthToken(t *testing.T) {
	testutil.SetupTestEnv(t)

	t.Run("force", func(t *testing.T) {
		server := testutil.NewMockServer(t, testutil.FixturesDir())
		server.OnPattern(http.MethodDelete, tokenPathPattern, respond(http.StatusOK, map[string]any{}))
		stdout, _, err := runCmd(t, server.URL, "delete", tokenID, "--force")
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("Auth token %q deleted\n", tokenID), stdout)
	})
	t.Run("already deleted with force", func(t *testing.T) {
		server := testutil.NewMockServer(t, testutil.FixturesDir())
		server.OnPattern(http.MethodDelete, tokenPathPattern, respond(http.StatusNotFound, notFound))
		_, stderr, err := runCmd(t, server.URL, "delete", tokenID, "--force")
		require.NoError(t, err)
		assert.Contains(t, stderr, "already deleted")
	})
	t.Run("not found without force", func(t *testing.T) {
		withAgentMode(t, false)
		t.Cleanup(confirmation.SetReaderForTest(strings.NewReader("y\n")))
		server := testutil.NewMockServer(t, testutil.FixturesDir())
		server.OnPattern(http.MethodDelete, tokenPathPattern, respond(http.StatusNotFound, notFound))
		_, _, err := runCmd(t, server.URL, "delete", tokenID)
		require.ErrorContains(t, err, "not found")
	})
	t.Run("agent mode requires force", func(t *testing.T) {
		withAgentMode(t, true)
		server := testutil.NewMockServer(t, testutil.FixturesDir())
		_, _, err := runCmd(t, server.URL, "delete", tokenID)
		require.ErrorContains(t, err, "requires --force when run in agent mode")
		assert.Empty(t, server.Requests())
	})
}

// TestAuthTokens_SecretNeverLeaks runs every subcommand in every output format,
// with agent mode on and off, against a server that puts a canary secret in
// every response, and checks that the canary reaches no output.
func TestAuthTokens_SecretNeverLeaks(t *testing.T) {
	testutil.SetupTestEnv(t)
	canaryValue := newCanary()
	misbehaving := testutil.NewMockServer(t, testutil.FixturesDir())
	misbehaving.On(http.MethodGet, apiPathAuthTokens, respond(http.StatusOK, []any{tokenBody("Ingest", canaryValue)}))
	misbehaving.OnPattern(http.MethodGet, tokenPathPattern, respond(http.StatusOK, tokenBody("Ingest", canaryValue)))
	misbehaving.OnPattern(http.MethodPut, tokenPathPattern, respond(http.StatusOK, tokenBody("Ingest (renamed)", canaryValue)))
	misbehaving.OnPattern(http.MethodDelete, tokenPathPattern, respond(http.StatusOK, map[string]any{}))
	creating := newCreateServer(t, canaryValue)
	file := writeYAML(t, tokenYAML)

	type run struct {
		server *testutil.MockServer
		args   []string
	}
	var runs []run
	for _, format := range []string{"", "table", "wide", "json", "yaml", "csv"} {
		runs = append(runs, run{misbehaving, []string{"list", "-o", format}})
	}
	for _, format := range []string{"", "table", "json", "yaml"} {
		runs = append(runs,
			run{misbehaving, []string{"get", tokenID, "-o", format}},
			run{creating, []string{"create", "-f", file, "-o", format}},
			run{creating, []string{"create", "-f", file, "-o", format, "--secret-file", "<new>"}},
		)
	}
	runs = append(runs,
		run{misbehaving, []string{"update", "-f", file}},
		run{misbehaving, []string{"update", "-f", file, "--dry-run"}},
		run{misbehaving, []string{"delete", tokenID, "--force"}},
	)

	for _, agentMode := range []bool{false, true} {
		for _, entry := range runs {
			name := fmt.Sprintf("agent=%v/%s", agentMode, strings.Join(entry.args, " "))
			t.Run(name, func(t *testing.T) {
				withAgentMode(t, agentMode)
				args := append([]string(nil), entry.args...)
				for i, arg := range args {
					if arg == "<new>" {
						args[i] = filepath.Join(t.TempDir(), "token.secret")
					}
				}
				stdout, stderr, err := runCmd(t, entry.server.URL, args...)
				errText := ""
				if err != nil {
					errText = err.Error()
				}
				assert.NotContains(t, stdout, canaryValue, "stdout")
				assert.NotContains(t, stderr, canaryValue, "stderr")
				assert.NotContains(t, errText, canaryValue, "error")
			})
		}
	}
}
