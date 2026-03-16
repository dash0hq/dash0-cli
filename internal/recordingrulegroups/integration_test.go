//go:build integration

package recordingrulegroups

import (
	"net/http"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiPathRecordingRuleGroups = "/api/recording-rule-groups"
	fixtureListSuccess         = "recordingrulegroups/list_success.json"
	fixtureListEmpty           = "recordingrulegroups/list_empty.json"
	fixtureGetSuccess          = "recordingrulegroups/get_success.json"
	fixtureUnauthorized        = "dashboards/error_unauthorized.json"
)

func TestListRecordingRuleGroups_JSONFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathRecordingRuleGroups, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewRecordingRuleGroupsCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json", "--limit", "2"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// JSON output should contain full recording rule group definitions
	assert.Contains(t, output, `"kind": "Dash0RecordingRuleGroup"`)
	assert.Contains(t, output, `"metadata"`)
	assert.Contains(t, output, `"spec"`)
	assert.Contains(t, output, `"HTTP Metrics"`)
}

func TestListRecordingRuleGroups_YAMLFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathRecordingRuleGroups, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewRecordingRuleGroupsCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml", "--limit", "2"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// YAML output should contain full recording rule group definitions as multi-document YAML
	assert.Contains(t, output, "kind: Dash0RecordingRuleGroup")
	assert.Contains(t, output, "metadata:")
	assert.Contains(t, output, "spec:")
	assert.Contains(t, output, "HTTP Metrics")
	// Multiple documents should be separated by ---
	assert.Contains(t, output, "---")
}
