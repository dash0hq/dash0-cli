//go:build integration

package datasets

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/confirmation"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiPathDatasets = "/api/datasets"
	testAuthToken   = "auth_test_token"
	deletePrompt    = `Are you sure you want to delete dataset "staging"? Dashboards, check rules, synthetic checks and views are kept for 30 days; creating the same slug within that time restores them. Telemetry is removed as its retention expires. [y/N]: `

	telemetryFiltersWarning = "spec.telemetryFilters is not managed by the CLI"

	datasetWithFiltersYAML = `kind: Dash0Dataset
metadata:
  name: Production
  labels:
    dash0.com/id: production
spec:
  telemetryFilters:
    - context: log
      condition: "true"
`

	datasetYAML = `kind: Dash0Dataset
metadata:
  name: Production
  labels:
    dash0.com/id: production
spec: {}
`
)

var datasetIDPattern = regexp.MustCompile(`^/api/datasets/[^/]+$`)

// newExperimentalDatasetsCmd creates a root command with the --experimental persistent
// flag and the datasets subcommand attached, mirroring the real command tree.
func newExperimentalDatasetsCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewDatasetsCmd())
	return root
}

// runCmd runs the command tree with the connection flags appended and captures stdout and stderr.
func runCmd(t *testing.T, serverURL string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newExperimentalDatasetsCmd()
	cmd.SetArgs(append(args, "--api-url", serverURL, "--auth-token", testAuthToken))
	stderr = testutil.CaptureStderr(t, func() {
		stdout = testutil.CaptureStdout(t, func() {
			err = cmd.Execute()
		})
	})
	return stdout, stderr, err
}

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dataset.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

// requestsFor returns the recorded requests with the given method and path.
func requestsFor(server *testutil.MockServer, method, path string) []testutil.RecordedRequest {
	var out []testutil.RecordedRequest
	for _, r := range server.Requests() {
		if r.Method == method && r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

func decodeDataset(t *testing.T, req testutil.RecordedRequest) dash0api.DatasetDefinition {
	t.Helper()
	var d dash0api.DatasetDefinition
	require.NoError(t, json.Unmarshal(req.Body, &d))
	return d
}

func mockListSuccess(t *testing.T) *testutil.MockServer {
	t.Helper()
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDatasets, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsListSuccess,
		Validator:  testutil.RequireHeaders,
	})
	return server
}

func TestListDatasets_Success(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := mockListSuccess(t)

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "list")

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.Len(t, lines, 4) // header + 3 datasets
	assert.Regexp(t, `^NAME\s+ID$`, strings.TrimSpace(lines[0]))
	assert.Regexp(t, `^Default\s+default$`, strings.TrimSpace(lines[1]))
	assert.Regexp(t, `^Production\s+production$`, strings.TrimSpace(lines[2]))
	assert.Regexp(t, `^Staging\s+staging$`, strings.TrimSpace(lines[3]))
}

func TestListDatasets_Empty(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDatasets, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsListEmpty,
		Validator:  testutil.RequireHeaders,
	})

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "list")

	require.NoError(t, err)
	assert.Contains(t, output, "No datasets found")
}

func TestListDatasets_Wide(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := mockListSuccess(t)

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "list", "-o", "wide")

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.Len(t, lines, 4)
	assert.Regexp(t, `^NAME\s+ID\s+ORIGIN$`, strings.TrimSpace(lines[0]))
	assert.Regexp(t, `^Production\s+production\s+dash0-cli$`, strings.TrimSpace(lines[2]))
	assert.Regexp(t, `^Staging\s+staging\s+-$`, strings.TrimSpace(lines[3]))
}

func TestListDatasets_JSON(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := mockListSuccess(t)

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "list", "-o", "json")

	require.NoError(t, err)
	var parsed []dash0api.DatasetDefinition
	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	require.Len(t, parsed, 3)
	assert.Equal(t, dash0api.Dash0Dataset, parsed[1].Kind)
	assert.Equal(t, "production", dash0api.GetDatasetID(&parsed[1]))
	assert.Equal(t, "Production", dash0api.GetDatasetName(&parsed[1]))
}

func TestListDatasets_EmptyJSONIsArray(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDatasets, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsListEmpty,
		Validator:  testutil.RequireHeaders,
	})

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "list", "-o", "json")

	require.NoError(t, err)
	assert.Equal(t, "[]", strings.TrimSpace(output))
}

func TestListDatasets_YAML(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := mockListSuccess(t)

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "list", "-o", "yaml")

	require.NoError(t, err)
	assert.Equal(t, 3, strings.Count(output, "kind: Dash0Dataset"))
	assert.Equal(t, 2, strings.Count(output, "\n---\n"))
}

func TestListDatasets_CSV(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := mockListSuccess(t)

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "list", "-o", "csv")

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.Len(t, lines, 4)
	assert.Equal(t, "name,id,origin", lines[0])
	assert.Equal(t, "Production,production,dash0-cli", lines[2])
	assert.Equal(t, "Staging,staging,-", lines[3])
}

func TestListDatasets_SkipHeader(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := mockListSuccess(t)

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "list", "--skip-header")

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.Len(t, lines, 3)
	assert.Regexp(t, `^Default\s+default$`, strings.TrimSpace(lines[0]))
}

func TestListDatasets_Column(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := mockListSuccess(t)

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "list", "-o", "csv", "--column", "slug")

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Equal(t, []string{"id", "default", "production", "staging"}, lines)
}

func TestListDatasets_InvalidFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	_, _, err := runCmd(t, "http://unused", "-X", "datasets", "list", "-o", "xml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown output format")
}

func TestListDatasets_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathDatasets, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   testutil.FixtureDatasetsUnauthorized,
		Validator:  testutil.RequireHeaders,
	})

	_, _, err := runCmd(t, server.URL, "-X", "datasets", "list")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestDatasets_RequireExperimental(t *testing.T) {
	testutil.SetupTestEnv(t)
	yamlFile := writeYAML(t, datasetYAML)

	for name, args := range map[string][]string{
		"list":   {"datasets", "list"},
		"get":    {"datasets", "get", "production"},
		"create": {"datasets", "create", "-f", yamlFile},
		"update": {"datasets", "update", "-f", yamlFile},
		"delete": {"datasets", "delete", "production", "--force"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := runCmd(t, "http://unused", args...)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "experimental")
		})
	}
}

func TestGetDataset_Success(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "get", "production")

	require.NoError(t, err)
	assert.Regexp(t, `Kind:\s+Dash0Dataset`, output)
	assert.Regexp(t, `Name:\s+Production`, output)
	assert.Regexp(t, `ID:\s+production`, output)
	assert.Regexp(t, `Origin:\s+dash0-cli`, output)
	require.Len(t, requestsFor(server, http.MethodGet, "/api/datasets/production"), 1)
}

func TestGetDataset_JSON(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "get", "production", "-o", "json")

	require.NoError(t, err)
	var parsed dash0api.DatasetDefinition
	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Equal(t, "production", dash0api.GetDatasetID(&parsed))
	assert.Equal(t, "Production", parsed.Metadata.Name)
}

func TestGetDataset_YAML(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "get", "production", "-o", "yaml")

	require.NoError(t, err)
	assert.Contains(t, output, "kind: Dash0Dataset")
	assert.Contains(t, output, "dash0.com/id: production")
}

func TestGetDataset_InvalidFormatMakesNoAPICall(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	_, _, err := runCmd(t, server.URL, "-X", "datasets", "get", "production", "-o", "xml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown output format")
	assert.Empty(t, server.Requests())
}

func TestGetDataset_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureDatasetsNotFound,
		Validator:  testutil.RequireHeaders,
	})

	_, _, err := runCmd(t, server.URL, "-X", "datasets", "get", "missing")

	require.Error(t, err)
	assert.Contains(t, err.Error(), `dataset "missing" not found`)
}

func TestGetDataset_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   testutil.FixtureDatasetsUnauthorized,
		Validator:  testutil.RequireHeaders,
	})

	_, _, err := runCmd(t, server.URL, "-X", "datasets", "get", "production")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestCreateDataset_PostsWhenPreflightIs404(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureDatasetsNotFound,
		Validator:  testutil.RequireHeaders,
	})
	server.On(http.MethodPost, apiPathDatasets, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsCreateSuccess,
		Validator:  testutil.RequireHeaders,
	})

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "create", "-f", writeYAML(t, datasetYAML))

	require.NoError(t, err)
	assert.Contains(t, output, `Dataset "Production" created (id: production)`)

	posts := requestsFor(server, http.MethodPost, apiPathDatasets)
	require.Len(t, posts, 1)
	body := decodeDataset(t, posts[0])
	assert.Equal(t, dash0api.Dash0Dataset, body.Kind)
	assert.Equal(t, "Production", body.Metadata.Name)
	assert.Equal(t, "production", dash0api.GetDatasetID(&body))
	assert.Empty(t, requestsFor(server, http.MethodPut, "/api/datasets/production"))
}

func TestCreateDataset_PutsWhenPreflightHits(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPut, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsUpdateSuccess,
		Validator:  testutil.RequireHeaders,
	})

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "create", "-f", writeYAML(t, datasetYAML))

	require.NoError(t, err)
	assert.Contains(t, output, "updated")

	puts := requestsFor(server, http.MethodPut, "/api/datasets/production")
	require.Len(t, puts, 1)
	body := decodeDataset(t, puts[0])
	assert.Equal(t, "production", dash0api.GetDatasetID(&body))
	assert.Equal(t, "Production", body.Metadata.Name)
	assert.Empty(t, requestsFor(server, http.MethodPost, apiPathDatasets))
}

func TestCreateDataset_PreflightErrorSurfaces(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   testutil.FixtureDatasetsUnauthorized,
		Validator:  testutil.RequireHeaders,
	})

	_, _, err := runCmd(t, server.URL, "-X", "datasets", "create", "-f", writeYAML(t, datasetYAML))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
	assert.Empty(t, requestsFor(server, http.MethodPost, apiPathDatasets))
}

func TestCreateDataset_PermissionsFollowedByPut(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureDatasetsNotFound,
		Validator:  testutil.RequireHeaders,
	})
	server.On(http.MethodPost, apiPathDatasets, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsCreateSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPut, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsCreateSuccess,
		Validator:  testutil.RequireHeaders,
	})
	yamlFile := writeYAML(t, `kind: Dash0Dataset
metadata:
  name: Production
  labels:
    dash0.com/id: production
spec:
  permissions:
    - role: admin
      actions:
        - dataset:read
`)

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "create", "-f", yamlFile)

	require.NoError(t, err)
	assert.Contains(t, output, "created")
	require.Len(t, requestsFor(server, http.MethodPost, apiPathDatasets), 1)
	puts := requestsFor(server, http.MethodPut, "/api/datasets/production")
	require.Len(t, puts, 1)
	body := decodeDataset(t, puts[0])
	require.NotNil(t, body.Spec.Permissions)
	require.Len(t, *body.Spec.Permissions, 1)
	assert.Equal(t, []dash0api.DatasetAction{dash0api.DatasetRead}, (*body.Spec.Permissions)[0].Actions)
}

func TestCreateDataset_WarnsAboutTelemetryFilters(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureDatasetsNotFound,
		Validator:  testutil.RequireHeaders,
	})
	server.On(http.MethodPost, apiPathDatasets, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsCreateSuccess,
		Validator:  testutil.RequireHeaders,
	})

	output, stderr, err := runCmd(t, server.URL, "-X", "datasets", "create", "-f", writeYAML(t, datasetWithFiltersYAML))

	require.NoError(t, err)
	assert.Contains(t, stderr, telemetryFiltersWarning)
	assert.NotContains(t, output, telemetryFiltersWarning)
	posts := requestsFor(server, http.MethodPost, apiPathDatasets)
	require.Len(t, posts, 1)
	assert.Nil(t, decodeDataset(t, posts[0]).Spec.TelemetryFilters, "telemetryFilters must be stripped from the request")
}

func TestCreateDataset_NoTelemetryFiltersNoWarning(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureDatasetsNotFound,
		Validator:  testutil.RequireHeaders,
	})
	server.On(http.MethodPost, apiPathDatasets, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsCreateSuccess,
		Validator:  testutil.RequireHeaders,
	})

	_, stderr, err := runCmd(t, server.URL, "-X", "datasets", "create", "-f", writeYAML(t, datasetYAML))

	require.NoError(t, err)
	assert.NotContains(t, stderr, "telemetryFilters")
}

func TestCreateDataset_PermissionsPutFailureStillReportsCreated(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureDatasetsNotFound,
		Validator:  testutil.RequireHeaders,
	})
	server.On(http.MethodPost, apiPathDatasets, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsCreateSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPut, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusBadRequest,
		BodyFile:   testutil.FixtureDatasetsBadRequest,
		Validator:  testutil.RequireHeaders,
	})
	yamlFile := writeYAML(t, `kind: Dash0Dataset
metadata:
  name: Production
  labels:
    dash0.com/id: production
spec:
  permissions:
    - role: admin
      actions:
        - dataset:read
`)

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "create", "-f", yamlFile)

	require.Error(t, err)
	assert.Contains(t, output, `Dataset "Production" created (id: production)`)
	assert.Contains(t, err.Error(), "spec.permissions were not applied")
	assert.Contains(t, err.Error(), "rerun")
	require.Len(t, requestsFor(server, http.MethodPost, apiPathDatasets), 1)
	require.Len(t, requestsFor(server, http.MethodPut, "/api/datasets/production"), 1)
}

func TestCreateDataset_SlugRequired(t *testing.T) {
	testutil.SetupTestEnv(t)
	yamlFile := writeYAML(t, `kind: Dash0Dataset
metadata:
  name: Production
spec: {}
`)

	_, _, err := runCmd(t, "http://unused", "-X", "datasets", "create", "-f", yamlFile)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "dash0.com/id")
}

func TestCreateDataset_DryRun(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "create", "-f", writeYAML(t, datasetYAML), "--dry-run")

	require.NoError(t, err)
	assert.Contains(t, output, "Dry run")
	assert.Empty(t, server.Requests())
}

func TestUpdateDataset_NotFoundSendsNoPut(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureDatasetsNotFound,
		Validator:  testutil.RequireHeaders,
	})

	_, _, err := runCmd(t, server.URL, "-X", "datasets", "update", "-f", writeYAML(t, datasetYAML))

	require.Error(t, err)
	assert.Contains(t, err.Error(), `dataset "production" not found`)
	assert.Empty(t, requestsFor(server, http.MethodPut, "/api/datasets/production"))
	assert.Empty(t, requestsFor(server, http.MethodPost, apiPathDatasets))
}

func TestUpdateDataset_Success(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPut, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsUpdateSuccess,
		Validator:  testutil.RequireHeaders,
	})
	// The input carries server-managed fields, as an export from get -o yaml does.
	yamlFile := writeYAML(t, `kind: Dash0Dataset
metadata:
  name: Production (renamed)
  labels:
    dash0.com/id: production
    dash0.com/source: ui
    dash0.com/version: "7"
  annotations:
    dash0.com/created-at: "2020-01-01T00:00:00Z"
    dash0.com/updated-at: "2020-01-02T00:00:00Z"
spec: {}
`)

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "update", "-f", yamlFile)

	require.NoError(t, err)
	assert.Contains(t, output, "--- Dataset (before)")
	assert.Contains(t, output, "+++ Dataset (after)")
	assert.Contains(t, output, "-  name: Production")
	assert.Contains(t, output, "+  name: Production (renamed)")

	puts := requestsFor(server, http.MethodPut, "/api/datasets/production")
	require.Len(t, puts, 1)
	body := decodeDataset(t, puts[0])
	assert.Equal(t, "Production (renamed)", body.Metadata.Name)
	assert.Nil(t, body.Metadata.Annotations, "server-managed annotations must be stripped")
	require.NotNil(t, body.Metadata.Labels)
	assert.Nil(t, body.Metadata.Labels.Dash0Comversion, "server-managed version label must be stripped")
	assert.Nil(t, body.Metadata.Labels.Dash0Comsource, "server-managed source label must be stripped")
	assert.Equal(t, "production", dash0api.GetDatasetID(&body), "the slug label must survive")
}

func TestUpdateDataset_WarnsAboutTelemetryFilters(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPut, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsUpdateSuccess,
		Validator:  testutil.RequireHeaders,
	})

	_, stderr, err := runCmd(t, server.URL, "-X", "datasets", "update", "-f", writeYAML(t, datasetWithFiltersYAML))

	require.NoError(t, err)
	assert.Contains(t, stderr, telemetryFiltersWarning)
	puts := requestsFor(server, http.MethodPut, "/api/datasets/production")
	require.Len(t, puts, 1)
	assert.Nil(t, decodeDataset(t, puts[0]).Spec.TelemetryFilters, "telemetryFilters must be stripped from the request")
}

func TestUpdateDataset_PositionalIDTargetsPathWhenFileHasNoID(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPut, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsUpdateSuccess,
		Validator:  testutil.RequireHeaders,
	})

	// The file names no slug, so the positional id is the only key and must pick the URL path.
	yamlFile := writeYAML(t, "kind: Dash0Dataset\nmetadata:\n  name: Production\nspec: {}\n")

	_, _, err := runCmd(t, server.URL, "-X", "datasets", "update", "production", "-f", yamlFile)

	require.NoError(t, err)
	assert.Len(t, requestsFor(server, http.MethodGet, "/api/datasets/production"), 1)
	assert.Len(t, requestsFor(server, http.MethodPut, "/api/datasets/production"), 1)
}

func TestUpdateDataset_DryRun(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureDatasetsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	yamlFile := writeYAML(t, strings.Replace(datasetYAML, "name: Production", "name: Production (renamed)", 1))

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "update", "-f", yamlFile, "--dry-run")

	require.NoError(t, err)
	assert.Contains(t, output, "+  name: Production (renamed)")
	assert.Empty(t, requestsFor(server, http.MethodPut, "/api/datasets/production"))
}

func TestUpdateDataset_IDMismatch(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	_, _, err := runCmd(t, server.URL, "-X", "datasets", "update", "other", "-f", writeYAML(t, datasetYAML))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match")
	assert.Contains(t, err.Error(), "other")
	assert.Contains(t, err.Error(), "production")
	assert.Empty(t, server.Requests(), "a mismatched id must not reach the API")
}

func TestUpdateDataset_NoIDAnywhere(t *testing.T) {
	testutil.SetupTestEnv(t)
	yamlFile := writeYAML(t, "kind: Dash0Dataset\nmetadata:\n  name: Production\nspec: {}\n")

	_, _, err := runCmd(t, "http://unused", "-X", "datasets", "update", "-f", yamlFile)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no dataset id given")
}

func TestDeleteDataset_Force(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "delete", "staging", "--force")

	require.NoError(t, err)
	assert.Equal(t, "Dataset \"staging\" deleted\n", output)
	assert.Len(t, requestsFor(server, http.MethodDelete, "/api/datasets/staging"), 1)
}

func TestDeleteDataset_AgentModeRequiresForce(t *testing.T) {
	testutil.SetupTestEnv(t)
	prev := agentmode.Enabled
	agentmode.Enabled = true
	t.Cleanup(func() { agentmode.Enabled = prev })
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	_, _, err := runCmd(t, server.URL, "-X", "datasets", "delete", "staging")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires --force when run in agent mode")
	assert.Contains(t, err.Error(), "datasets delete staging --force")
	assert.Empty(t, server.Requests())
}

func TestDeleteDataset_AgentModeWithForce(t *testing.T) {
	testutil.SetupTestEnv(t)
	prev := agentmode.Enabled
	agentmode.Enabled = true
	t.Cleanup(func() { agentmode.Enabled = prev })
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "delete", "staging", "--force")

	require.NoError(t, err)
	assert.Equal(t, "Dataset \"staging\" deleted\n", output)
	assert.Len(t, requestsFor(server, http.MethodDelete, "/api/datasets/staging"), 1)
}

func TestDeleteDataset_PromptConfirmed(t *testing.T) {
	testutil.SetupTestEnv(t)
	restore := confirmation.SetReaderForTest(strings.NewReader("y\n"))
	t.Cleanup(restore)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "delete", "staging")

	require.NoError(t, err)
	assert.Equal(t, deletePrompt+"Dataset \"staging\" deleted\n", output)
	assert.Len(t, requestsFor(server, http.MethodDelete, "/api/datasets/staging"), 1)
}

func TestDeleteDataset_PromptDeclined(t *testing.T) {
	testutil.SetupTestEnv(t)
	restore := confirmation.SetReaderForTest(strings.NewReader("n\n"))
	t.Cleanup(restore)
	server := testutil.NewMockServer(t, testutil.FixturesDir())

	output, _, err := runCmd(t, server.URL, "-X", "datasets", "delete", "staging")

	require.NoError(t, err)
	assert.Equal(t, deletePrompt+"Deletion cancelled\n", output)
	assert.Empty(t, server.Requests(), "a declined deletion must not call the API")
}

func TestDeleteDataset_ForceAlreadyDeleted(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureDatasetsNotFound,
		Validator:  testutil.RequireHeaders,
	})

	output, stderr, err := runCmd(t, server.URL, "-X", "datasets", "delete", "staging", "--force")

	require.NoError(t, err)
	assert.Empty(t, output)
	assert.Contains(t, stderr, `Dataset "staging" was already deleted`)
}

func TestDeleteDataset_NotFoundWithoutForce(t *testing.T) {
	testutil.SetupTestEnv(t)
	restore := confirmation.SetReaderForTest(strings.NewReader("y\n"))
	t.Cleanup(restore)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureDatasetsNotFound,
		Validator:  testutil.RequireHeaders,
	})

	_, _, err := runCmd(t, server.URL, "-X", "datasets", "delete", "staging")

	require.Error(t, err)
	assert.Contains(t, err.Error(), `dataset "staging" not found`)
}

func TestDeleteDataset_DefaultRejected(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusBadRequest,
		BodyFile:   testutil.FixtureDatasetsBadRequest,
		Validator:  testutil.RequireHeaders,
	})

	_, _, err := runCmd(t, server.URL, "-X", "datasets", "delete", "default", "--force")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid request")
	assert.Contains(t, err.Error(), "the default dataset cannot be deleted")
}

func TestDeleteDataset_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, datasetIDPattern, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   testutil.FixtureDatasetsUnauthorized,
		Validator:  testutil.RequireHeaders,
	})

	_, _, err := runCmd(t, server.URL, "-X", "datasets", "delete", "staging", "--force")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}
