//go:build integration

package dashboards

import (
	"net/http"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListDashboards_NameLookupFailures(t *testing.T) {
	for _, format := range []string{"table", "wide", "csv", "json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			testutil.SetupTestEnv(t)
			t.Setenv("DASH0_MAX_RETRIES", "0")
			ids := []string{
				"0c3893ac-3d26-11ef-943e-eedf0419e619",
				"0e1e559b-a2e7-4504-b665-6121b84a28c2",
				"15dd7855-e156-4dee-bc37-43be799d6ea5",
			}
			server := testutil.NewMockServer(t, testutil.FixturesDir())
			server.On(http.MethodGet, apiPathDashboards, testutil.MockResponse{
				StatusCode: http.StatusOK,
				BodyFile:   fixtureListSuccess,
				Validator:  testutil.RequireHeaders,
			})
			server.On(http.MethodGet, apiPathDashboards+"/"+ids[0], testutil.MockResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       map[string]string{"message": "server returned 500"},
				Validator:  testutil.RequireHeaders,
			})
			server.On(http.MethodGet, apiPathDashboards+"/"+ids[1], testutil.MockResponse{
				StatusCode: http.StatusOK,
				BodyFile:   fixtureGetSuccess,
				Validator:  testutil.RequireHeaders,
			})
			server.On(http.MethodGet, apiPathDashboards+"/"+ids[2], testutil.MockResponse{
				StatusCode: http.StatusForbidden,
				Body:       map[string]string{"message": "permission denied"},
				Validator:  testutil.RequireHeaders,
			})
			cmd := NewDashboardsCmd()
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken, "--limit", "3", "-o", format})
			var err error
			var stdout string
			stderr := testutil.CaptureStderr(t, func() {
				stdout = testutil.CaptureStdout(t, func() {
					err = cmd.Execute()
				})
			})
			if format == "json" || format == "yaml" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), ids[0])
				assert.Empty(t, stdout)
				assert.NotContains(t, stderr, "warning:")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, 2, strings.Count(stdout, "<error>"))
			assert.Contains(t, stdout, "New dashboard")
			for _, id := range ids {
				assert.Contains(t, stdout, id)
			}
			assert.NotContains(t, stdout, "warning:")
			assert.Equal(t, 2, strings.Count(stderr, "warning: failed to resolve dashboard "))
			assert.Contains(t, stderr, "warning: failed to resolve dashboard "+ids[0]+":")
			assert.Contains(t, stderr, "server returned 500")
			assert.Contains(t, stderr, "warning: failed to resolve dashboard "+ids[2]+":")
			assert.Contains(t, stderr, "permission denied")
		})
	}
}
