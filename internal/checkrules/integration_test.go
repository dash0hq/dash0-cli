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
