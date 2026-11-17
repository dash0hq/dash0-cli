//go:build integration

package dashboards

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateDashboard_OriginAndExtensionsIDDiffer_FailsBeforeAnyRequest(t *testing.T) {
	for _, tt := range []struct {
		name  string
		extra []string
	}{
		{"create", nil},
		{"create --dry-run", []string{"--dry-run"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetupTestEnv(t)

			yamlFile := filepath.Join(t.TempDir(), "dashboard.yaml")
			require.NoError(t, os.WriteFile(yamlFile, []byte(`kind: Dashboard
metadata:
  name: checkout
  labels:
    dash0.com/origin: dash0-cli_checkout
  dash0Extensions:
    id: other-id
spec:
  display:
    name: Checkout
`), 0644))

			server := testutil.NewMockServer(t, testutil.FixturesDir())
			cmd := NewDashboardsCmd()
			cmd.SetArgs(append([]string{"create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken}, tt.extra...))

			var err error
			testutil.CaptureStdout(t, func() { err = cmd.Execute() })

			require.Error(t, err)
			assert.Contains(t, err.Error(), `metadata.labels["dash0.com/origin"] ("dash0-cli_checkout") and metadata.dash0Extensions.id ("other-id") must be equal; set only metadata.labels["dash0.com/origin"], and move "other-id" to metadata.labels["dash0.com/id"] if it is the id of an existing dashboard`)
			assert.Empty(t, server.Requests())
		})
	}
}
