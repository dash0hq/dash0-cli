//go:build integration

package teams

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTeamFileWrites_WarnOnLossyMetadata(t *testing.T) {
	for _, command := range []string{"create", "update"} {
		for _, dryRun := range []bool{false, true} {
			name := command
			if dryRun {
				name += "-dry-run"
			}
			t.Run(name, func(t *testing.T) {
				testutil.SetupTestEnv(t)
				server := testutil.NewMockServer(t, testutil.FixturesDir())
				const path = "/api/teams/backend"
				server.On(http.MethodGet, path, testutil.MockResponse{
					StatusCode: http.StatusOK, BodyFile: testutil.FixtureTeamsGetSuccess, Validator: testutil.RequireHeaders,
				})
				server.On(http.MethodPut, path, testutil.MockResponse{
					StatusCode: http.StatusOK, BodyFile: testutil.FixtureTeamsCreateSuccess, Validator: testutil.RequireHeaders,
				})
				file := filepath.Join(t.TempDir(), "team.yaml")
				require.NoError(t, os.WriteFile(file, []byte(`kind: Dash0Team
metadata:
  name: backend
  labels:
    dash0.com/origin: backend
    team-lead: alice
  annotations:
    example.com/note: private-value
spec:
  display: {name: Backend, color: {from: "#000000", to: "#ffffff"}}
  members: []
`), 0600))
				args := []string{"-X", "teams", command, "-f", file, "--api-url", server.URL, "--auth-token", testAuthToken}
				if dryRun {
					args = append(args, "--dry-run")
				}
				cmd := newExperimentalTeamsCmd()
				cmd.SetArgs(args)
				var cmdErr error
				var stdout string
				stderr := testutil.CaptureStderr(t, func() {
					stdout = testutil.CaptureStdout(t, func() { cmdErr = cmd.Execute() })
				})
				require.NoError(t, cmdErr)
				assert.Equal(t, 1, strings.Count(stderr, `metadata.labels["team-lead"]`))
				assert.Equal(t, 1, strings.Count(stderr, `metadata.annotations["example.com/note"]`))
				assert.Contains(t, stderr, file)
				assert.NotContains(t, stdout, "warning:")
				assert.NotContains(t, stderr, "private-value")
				writes := countRequests(server.Requests(), http.MethodPut, path)
				if dryRun {
					assert.Zero(t, writes)
				} else {
					assert.Equal(t, 1, writes)
				}
			})
		}
	}
}
