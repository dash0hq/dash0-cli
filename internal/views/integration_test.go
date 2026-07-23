//go:build integration

package views

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiPathViews        = "/api/views"
	fixtureListSuccess  = "views/list_success.json"
	fixtureListEmpty    = "views/list_empty.json"
	fixtureGetSuccess   = "views/get_success.json"
	fixtureUnauthorized = "dashboards/error_unauthorized.json"
)

var viewIDPattern = regexp.MustCompile(`^/api/views/[a-f0-9-]+$`)

func TestListViews_JSONFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathViews, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewViewsCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json", "--limit", "2"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// JSON output should contain full view definitions
	assert.Contains(t, output, `"kind": "Dash0View"`)
	assert.Contains(t, output, `"metadata"`)
	assert.Contains(t, output, `"spec"`)
}

func TestListViews_YAMLFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathViews, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewViewsCmd()
	cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml", "--limit", "2"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	// YAML output should contain full view definitions as multi-document YAML
	assert.Contains(t, output, "kind: Dash0View")
	assert.Contains(t, output, "metadata:")
	assert.Contains(t, output, "spec:")
	// Multiple documents should be separated by ---
	assert.Contains(t, output, "---")
}

// TestGetView_NotFound pins the clean-message contract on 404 GET.
func TestGetView_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureViewsNotFound,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewViewsCmd()
	cmd.SetArgs([]string{"get", "00000000-0000-0000-0000-000000000000", "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "view")
	assert.Contains(t, err.Error(), "not found")
}

// TestGetView_Unauthorized pins the clean-message contract on 401.
func TestGetView_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   fixtureUnauthorized,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewViewsCmd()
	cmd.SetArgs([]string{"get", "00000000-0000-0000-0000-000000000000", "--api-url", server.URL, "--auth-token", "auth_invalid"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

// TestDeleteView_NotFound covers the imperative delete path when the server
// returns 404.
func TestDeleteView_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, viewIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureViewsNotFound,
		Validator:  testutil.RequireHeaders,
	})

	cmd := NewViewsCmd()
	cmd.SetArgs([]string{"delete", "00000000-0000-0000-0000-000000000000", "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "view")
	assert.Contains(t, err.Error(), "not found")
}
