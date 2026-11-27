//go:build integration

package samplingrules

import (
	"net/http"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSamplingRuleFileWrites_WarnOnLossyMetadata(t *testing.T) {
	for _, command := range []string{"create", "update"} {
		for _, dryRun := range []bool{false, true} {
			name := command
			if dryRun {
				name += "-dry-run"
			}
			t.Run(name, func(t *testing.T) {
				testutil.SetupTestEnv(t)
				server := testutil.NewMockServer(t, testutil.FixturesDir())
				path := apiPath + "/" + testOrigin
				server.On(http.MethodGet, path, testutil.MockResponse{
					StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders,
				})
				server.On(http.MethodPut, path, testutil.MockResponse{
					StatusCode: http.StatusOK, BodyFile: fixtureGetSuccess, Validator: testutil.RequireHeaders,
				})
				raw := strings.Replace(docWithOrigin, "  labels:\n", "  annotations:\n    example.com/note: private-value\n  labels:\n    team-lead: alice\n", 1)
				file := writeDoc(t, raw)
				args := []string{"sampling-rules", command, "-f", file, "--api-url", server.URL, "--auth-token", testAuthToken}
				if dryRun {
					args = append(args, "--dry-run")
				}
				stdout, stderr, cmdErr := executeWithStderr(t, args...)
				require.NoError(t, cmdErr)
				assert.Equal(t, 1, strings.Count(stderr, `metadata.labels["team-lead"]`))
				assert.Equal(t, 1, strings.Count(stderr, `metadata.annotations["example.com/note"]`))
				assert.Contains(t, stderr, file)
				assert.NotContains(t, stderr, "private-value")
				assert.NotContains(t, stderr, "alice")
				assert.NotContains(t, stdout, "warning:")
				assert.Zero(t, countMethod(server.Requests(), http.MethodPost))
				if dryRun {
					assert.Zero(t, countMethod(server.Requests(), http.MethodPut))
				} else {
					require.Equal(t, 1, countMethod(server.Requests(), http.MethodPut))
					body := decodeRule(t, findRequest(t, server.Requests(), http.MethodPut, path).Body)
					assert.Equal(t, "checkout-errors", body.Metadata.Name)
					assert.True(t, body.Spec.Enabled)
				}
			})
		}
	}
}
