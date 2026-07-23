//go:build integration

package syntheticchecks

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiPathSyntheticChecks = "/api/synthetic-checks"
	fixtureListSuccess     = "syntheticchecks/list_success.json"
	fixtureListEmpty       = "syntheticchecks/list_empty.json"
	fixtureGetSuccess      = "syntheticchecks/get_success.json"
	fixtureUnauthorized    = "dashboards/error_unauthorized.json"
)

var syntheticCheckIDPattern = regexp.MustCompile(`^/api/synthetic-checks/[a-f0-9-]+$`)

func TestListSyntheticChecks_JSONFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathSyntheticChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodGet, syntheticCheckIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewSyntheticChecksCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json", "--limit", "2"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// JSON output should contain full synthetic check definitions
	assert.Contains(t, output, `"kind": "Dash0SyntheticCheck"`)
	assert.Contains(t, output, `"metadata"`)
	assert.Contains(t, output, `"spec"`)
}

func TestListSyntheticChecks_YAMLFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathSyntheticChecks, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodGet, syntheticCheckIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewSyntheticChecksCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml", "--limit", "2"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// YAML output should contain full synthetic check definitions as multi-document YAML
	assert.Contains(t, output, "kind: Dash0SyntheticCheck")
	assert.Contains(t, output, "metadata:")
	assert.Contains(t, output, "spec:")
	// Multiple documents should be separated by ---
	assert.Contains(t, output, "---")
}

// TestGetSyntheticCheck_NotFound pins the clean-message contract on 404 GET.
func TestGetSyntheticCheck_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, syntheticCheckIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureSyntheticChecksNotFound,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewSyntheticChecksCmd()
	cmd.SetArgs([]string{"get", "00000000-0000-0000-0000-000000000000", "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "synthetic check")
	assert.Contains(t, err.Error(), "not found")
}

// TestGetSyntheticCheck_Unauthorized pins the clean-message contract on 401.
func TestGetSyntheticCheck_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, syntheticCheckIDPattern, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   fixtureUnauthorized,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewSyntheticChecksCmd()
	cmd.SetArgs([]string{"get", "00000000-0000-0000-0000-000000000000", "--api-url", server.URL, "--auth-token", "auth_invalid"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

// TestDeleteSyntheticCheck_NotFound covers the imperative delete path when
// the server returns 404; the CLI must still exit non-zero with the same
// clean message.
func TestDeleteSyntheticCheck_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, syntheticCheckIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureSyntheticChecksNotFound,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewSyntheticChecksCmd()
	cmd.SetArgs([]string{"delete", "00000000-0000-0000-0000-000000000000", "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "synthetic check")
	assert.Contains(t, err.Error(), "not found")
}
