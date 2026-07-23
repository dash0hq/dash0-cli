//go:build integration

package checkrules

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiPathCheckRules   = "/api/alerting/check-rules"
	fixtureListSuccess  = "checkrules/list_success.json"
	fixtureListEmpty    = "checkrules/list_empty.json"
	fixtureGetSuccess   = "checkrules/get_success.json"
	fixtureUnauthorized = "dashboards/error_unauthorized.json"
)

var checkRuleIDPattern = regexp.MustCompile(`^/api/alerting/check-rules/[a-f0-9-]+$`)

func TestListCheckRules_JSONFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathCheckRules, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodGet, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewCheckRulesCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json", "--limit", "2"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// JSON output should contain full check rule definitions
	assert.Contains(t, output, `"name"`)
	assert.Contains(t, output, `"expression"`)
	assert.Contains(t, output, `"Failing check rule 2"`)
}

func TestListCheckRules_YAMLFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathCheckRules, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodGet, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewCheckRulesCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml", "--limit", "2"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// YAML output should contain full check rule definitions
	assert.Contains(t, output, "name: Failing check rule 2")
	assert.Contains(t, output, "expression:")
}

// TestGetCheckRule_NotFound pins the observable behavior when the server
// returns 404 for a check rule GET: the error message must include "not found"
// and name the asset type, so `dash0 check-rules get <id>` surfaces an
// actionable message instead of a raw HTTP error.
func TestGetCheckRule_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureCheckRulesNotFound,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewCheckRulesCmd()
	cmd.SetArgs([]string{"get", "00000000-0000-0000-0000-000000000000", "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "check rule")
	assert.Contains(t, err.Error(), "not found")
}

// TestGetCheckRule_Unauthorized pins the clean-message contract for 401.
// The failure mode checklist requires that auth failures surface a message
// that identifies the credential problem, not a raw 401 body.
func TestGetCheckRule_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   fixtureUnauthorized,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewCheckRulesCmd()
	cmd.SetArgs([]string{"get", "00000000-0000-0000-0000-000000000000", "--api-url", server.URL, "--auth-token", "auth_invalid"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

// TestDeleteCheckRule_NotFound covers the imperative delete path when
// the server returns 404.
func TestDeleteCheckRule_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, checkRuleIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureCheckRulesNotFound,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewCheckRulesCmd()
	cmd.SetArgs([]string{"delete", "00000000-0000-0000-0000-000000000000", "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "check rule")
	assert.Contains(t, err.Error(), "not found")
}
