//go:build integration

package apply

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

func TestApply_WarnsOnLossyMetadataBeforeWriting(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		name := "write"
		if dryRun {
			name = "dry-run"
		}
		t.Run(name, func(t *testing.T) {
			testutil.SetupTestEnv(t)
			server := testutil.NewMockServer(t, testutil.FixturesDir())
			const path = "/api/teams"
			writes := 0
			server.On(http.MethodPost, path, testutil.MockResponse{
				StatusCode: http.StatusOK, BodyFile: testutil.FixtureTeamsCreateSuccess,
				Validator: func(r *http.Request) error {
					writes++
					return testutil.RequireHeaders(r)
				},
			})
			file := filepath.Join(t.TempDir(), "teams.yaml")
			require.NoError(t, os.WriteFile(file, []byte(`kind: Dash0Team
metadata: {name: backend, labels: {team-lead: alice}}
spec:
  display: {name: Backend, color: {from: "#000000", to: "#ffffff"}}
  members: []
---
kind: Dash0Team
metadata: {name: frontend, annotations: {example.com/note: value}}
spec:
  display: {name: Frontend, color: {from: "#000000", to: "#ffffff"}}
  members: []
`), 0600))
			args := []string{"-f", file, "--api-url", server.URL, "--auth-token", testAuthToken}
			if dryRun {
				args = append(args, "--dry-run")
			}
			cmd := NewApplyCmd()
			cmd.SetArgs(args)
			var cmdErr error
			var stdout string
			stderr := testutil.CaptureStderr(t, func() {
				stdout = testutil.CaptureStdout(t, func() { cmdErr = cmd.Execute() })
			})
			require.NoError(t, cmdErr)
			assert.Equal(t, 1, strings.Count(stderr, `document 1: metadata.labels["team-lead"]`))
			assert.Equal(t, 1, strings.Count(stderr, `document 2: metadata.annotations["example.com/note"]`))
			assert.NotContains(t, stdout, "warning:")
			if dryRun {
				assert.Zero(t, writes)
			} else {
				assert.Equal(t, 2, writes)
			}
		})
	}
}
