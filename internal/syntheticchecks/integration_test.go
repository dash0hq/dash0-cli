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
