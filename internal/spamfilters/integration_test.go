//go:build integration

package spamfilters

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSpamFiltersCmd creates a root command with the --experimental persistent
// flag and the spam-filters subcommand attached, mirroring the real command tree.
// The flag is registered so the backward-compatibility test can still pass -X.
func newSpamFiltersCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewSpamFiltersCmd())
	return root
}

const (
	apiPathSpamFilters    = "/api/spam-filters"
	testAuthToken         = "auth_test_token"
	fixtureListSuccess    = "spamfilters/list_success.json"
	fixtureListEmpty      = "spamfilters/list_empty.json"
	fixtureGetSuccess     = "spamfilters/get_success.json"
	fixtureGetSuccessV1A2 = "spamfilters/get_success_v1alpha2.json"
	fixtureGetDisabled    = "spamfilters/get_disabled.json"
	fixtureCreateSuccess  = "spamfilters/create_success.json"
	fixtureNotFound       = "spamfilters/error_not_found.json"
	fixtureUnauthorized   = "dashboards/error_unauthorized.json"
)

var spamFilterIDPattern = regexp.MustCompile(`^/api/spam-filters/[^/]+$`)

func TestListSpamFilters_JSONFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathSpamFilters, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSpamFiltersCmd()
	cmd.SetArgs([]string{"spam-filters", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json", "--limit", "2"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, `"kind": "Dash0SpamFilter"`)
	assert.Contains(t, output, `"metadata"`)
	assert.Contains(t, output, `"spec"`)
	assert.Contains(t, output, `"Drop noisy health checks"`)
}

func TestListSpamFilters_YAMLFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathSpamFilters, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSpamFiltersCmd()
	cmd.SetArgs([]string{"spam-filters", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml", "--limit", "2"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "kind: Dash0SpamFilter")
	assert.Contains(t, output, "metadata:")
	assert.Contains(t, output, "spec:")
	assert.Contains(t, output, "---")
}

func TestListSpamFilters_TableFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathSpamFilters, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSpamFiltersCmd()
	cmd.SetArgs([]string{"spam-filters", "list", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "ID")
	assert.Contains(t, output, "Drop noisy health checks")
	assert.Contains(t, output, "Drop debug spans")
}

func TestListSpamFilters_Empty(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathSpamFilters, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListEmpty,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSpamFiltersCmd()
	cmd.SetArgs([]string{"spam-filters", "list", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "No spam filters found.")
}

func TestGetSpamFilter_JSONFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, spamFilterIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSpamFiltersCmd()
	cmd.SetArgs([]string{"spam-filters", "get", "00000000-0000-0000-0000-000000000001", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, `"kind": "Dash0SpamFilter"`)
	assert.Contains(t, output, `"Drop noisy health checks"`)
}

func TestGetSpamFilter_YAMLFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, spamFilterIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSpamFiltersCmd()
	cmd.SetArgs([]string{"spam-filters", "get", "00000000-0000-0000-0000-000000000001", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "kind: Dash0SpamFilter")
	assert.Contains(t, output, "name: Drop noisy health checks")
}

func TestGetSpamFilter_DefaultFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, spamFilterIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSpamFiltersCmd()
	cmd.SetArgs([]string{"spam-filters", "get", "00000000-0000-0000-0000-000000000001", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "Kind: Dash0SpamFilter")
	assert.Contains(t, output, "API Version: v1alpha1")
	assert.Contains(t, output, "Name: Drop noisy health checks")
	assert.Contains(t, output, "Enabled: true\n")
	assert.Contains(t, output, "Filters: k8s.namespace.name is kube-system")
}

func TestDeleteSpamFilter_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, spamFilterIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSpamFiltersCmd()
	cmd.SetArgs([]string{"spam-filters", "delete", "00000000-0000-0000-0000-000000000001", "--api-url", server.URL, "--auth-token", testAuthToken, "--force"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "deleted")
}

func TestListSpamFilters_DatasetQueryParam(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathSpamFilters, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureListEmpty,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSpamFiltersCmd()
	cmd.SetArgs([]string{"spam-filters", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "--dataset", "my-dataset"})

	var err error
	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Equal(t, http.MethodGet, req.Method)
	assert.Equal(t, apiPathSpamFilters, req.Path)
	assert.Contains(t, req.Query, "dataset=my-dataset")
	assert.True(t, strings.HasPrefix(req.Header.Get("Authorization"), "Bearer "))
}

func TestCreateSpamFilter_DatasetQueryParam(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathSpamFilters, testutil.MockResponse{
		StatusCode: http.StatusCreated,
		BodyFile:   fixtureCreateSuccess,
		Validator:  testutil.RequireHeaders,
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "filter.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: Dash0SpamFilter
metadata:
  name: Drop noisy health checks
spec:
  contexts:
    - log
  filter:
    - key: "k8s.namespace.name"
      operator: "is"
      value: "kube-system"
`), 0644)
	require.NoError(t, err)

	cmd := newSpamFiltersCmd()
	cmd.SetArgs([]string{"spam-filters", "create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken, "--dataset", "my-dataset"})

	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Equal(t, http.MethodPost, req.Method)
	assert.Contains(t, req.Query, "dataset=my-dataset")
}

func TestUpdateSpamFilter_DatasetQueryParam(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, spamFilterIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})
	server.OnPattern(http.MethodPut, spamFilterIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "filter.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: Dash0SpamFilter
metadata:
  name: Drop noisy health checks
  labels:
    dash0.com/id: 00000000-0000-0000-0000-000000000001
spec:
  contexts:
    - log
  filter:
    - key: "k8s.namespace.name"
      operator: "is"
      value: "kube-system"
`), 0644)
	require.NoError(t, err)

	cmd := newSpamFiltersCmd()
	cmd.SetArgs([]string{"spam-filters", "update", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken, "--dataset", "my-dataset"})

	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Equal(t, http.MethodPut, req.Method)
	assert.Contains(t, req.Query, "dataset=my-dataset")
}

func TestGetSpamFilter_V1Alpha2_DefaultFormat(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, spamFilterIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccessV1A2,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSpamFiltersCmd()
	cmd.SetArgs([]string{"spam-filters", "get", "00000000-0000-0000-0000-0000000000a2", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "Kind: Dash0SpamFilter")
	assert.Contains(t, output, "API Version: v1alpha2")
	assert.Contains(t, output, "Name: Drop debug logs (v1alpha2)")
	// v1alpha2 prints a single "Context:" line, not "Contexts:".
	assert.Contains(t, output, "Context: log")
	assert.NotContains(t, output, "Contexts:")
	assert.Contains(t, output, "Filters: otel.log.severity.range is DEBUG")
}

func TestGetSpamFilter_V1Alpha2_YAMLPreservesShape(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, spamFilterIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   fixtureGetSuccessV1A2,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newSpamFiltersCmd()
	cmd.SetArgs([]string{"spam-filters", "get", "00000000-0000-0000-0000-0000000000a2", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "apiVersion: v1alpha2")
	// v1alpha2 spec uses a scalar `context`, not an array `contexts`.
	assert.Contains(t, output, "context: log")
	assert.NotContains(t, output, "contexts:")
}

func TestCreateSpamFilter_V1Alpha2_SendsV1Alpha2Body(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathSpamFilters, testutil.MockResponse{
		StatusCode: http.StatusCreated,
		BodyFile:   fixtureGetSuccessV1A2,
		Validator:  testutil.RequireHeaders,
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "filter.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: v1alpha2
kind: Dash0SpamFilter
metadata:
  name: Drop debug logs (v1alpha2)
spec:
  context: log
  filter:
    - key: otel.log.severity.range
      operator: is
      value: DEBUG
`), 0644)
	require.NoError(t, err)

	cmd := newSpamFiltersCmd()
	cmd.SetArgs([]string{"spam-filters", "create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	require.NoError(t, err)

	req := server.LastRequest()
	require.NotNil(t, req)
	assert.Equal(t, http.MethodPost, req.Method)

	// Parse the outbound body as v1alpha2 and assert on typed fields. This is
	// the convention required by docs/code-style.md and avoids the fragility
	// of substring matching on raw JSON.
	var sent dash0api.SpamFilterV1Alpha2
	require.NoError(t, json.Unmarshal(req.Body, &sent))
	assert.Equal(t, dash0api.SpamFilterApiVersionV1Alpha2V1alpha2, sent.ApiVersion)
	assert.Equal(t, dash0api.TelemetryFilterContextLog, sent.Spec.Context)
	require.Len(t, sent.Spec.Filter, 1)
	assert.Equal(t, "otel.log.severity.range", sent.Spec.Filter[0].Key)
}

func TestCreateSpamFilter_UnknownAPIVersion(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "filter.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: v9beta
kind: Dash0SpamFilter
metadata:
  name: bogus
spec: {}
`), 0644)
	require.NoError(t, err)

	cmd := newSpamFiltersCmd()
	cmd.SetArgs([]string{"spam-filters", "create", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()
	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), `unsupported spam filter apiVersion "v9beta"`)
	assert.Contains(t, cmdErr.Error(), `"v1alpha1"`)
	assert.Contains(t, cmdErr.Error(), `"v1alpha2"`)
}

// execute runs the spam-filters command tree with args and returns its stdout.
func execute(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newSpamFiltersCmd()
	cmd.SetArgs(args)
	var err error
	out := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})
	return out, err
}

// writeDoc writes a spam filter document to a temporary file and returns its path.
func writeDoc(t *testing.T, doc string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "filter.yaml")
	require.NoError(t, os.WriteFile(path, []byte(doc), 0644))
	return path
}

// The commands no longer require --experimental, but scripts written while
// they did still pass it.
func TestSpamFilters_BackwardCompatWithExperimentalFlag(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathSpamFilters, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListSuccess, Validator: testutil.RequireHeaders})

	out, err := execute(t, "-X", "spam-filters", "list", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "Drop noisy health checks")
}

// The list endpoint returns every filter in the apiVersion it was stored in,
// so a v1alpha2 filter keeps its scalar spec.context.
func TestListSpamFilters_ReturnsEachFilterInItsOwnAPIVersion(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathSpamFilters, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListSuccess, Validator: testutil.RequireHeaders})

	out, err := execute(t, "spam-filters", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json")

	require.NoError(t, err)
	var items []json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(out), &items))
	require.Len(t, items, 3)

	var v1alpha1 dash0api.SpamFilter
	require.NoError(t, json.Unmarshal(items[0], &v1alpha1))
	assert.Equal(t, []dash0api.TelemetryFilterContext{dash0api.TelemetryFilterContextLog}, v1alpha1.Spec.Contexts)

	var v1alpha2 dash0api.SpamFilterV1Alpha2
	require.NoError(t, json.Unmarshal(items[2], &v1alpha2))
	assert.Equal(t, dash0api.SpamFilterApiVersionV1Alpha2V1alpha2, v1alpha2.ApiVersion)
	assert.Equal(t, dash0api.TelemetryFilterContextLog, v1alpha2.Spec.Context)
}

func TestListSpamFilters_TableFormatShowsEnabled(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathSpamFilters, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListSuccess, Validator: testutil.RequireHeaders})

	out, err := execute(t, "spam-filters", "list", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 4)
	assert.Equal(t, []string{"NAME", "ID", "CONTEXT", "ENABLED"}, strings.Fields(lines[0]))
	assert.Equal(t, "true", lastField(lines[1]), "a filter with dash0.com/enabled: \"true\" is enabled")
	assert.Equal(t, "false", lastField(lines[3]), "a filter with dash0.com/enabled: \"false\" is disabled")
	assert.NotContains(t, out, "ORIGIN", "origin is a wide-only column")
}

func TestListSpamFilters_CSVFormatShowsEnabled(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathSpamFilters, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureListSuccess, Validator: testutil.RequireHeaders})

	out, err := execute(t, "spam-filters", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "csv")

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 4)
	assert.Equal(t, "name,id,context,enabled,apiversion,filters,dataset,origin", lines[0])
	assert.Equal(t, "Drop debug logs,00000000-0000-0000-0000-0000000000a2,log,false,v1alpha2,otel.log.severity.range is DEBUG,default,dash0-cli_drop-debug-logs", lines[3])
}

func TestGetSpamFilter_DefaultFormatShowsDisabled(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, spamFilterIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetDisabled, Validator: testutil.RequireHeaders})

	out, err := execute(t, "spam-filters", "get", "dash0-cli_drop-noisy-health-checks", "--api-url", server.URL, "--auth-token", testAuthToken)

	require.NoError(t, err)
	assert.Contains(t, out, "Enabled: false\n")
}

func TestGetSpamFilter_YAMLKeepsDisabledAnnotation(t *testing.T) {
	testutil.SetupTestEnv(t)
	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, spamFilterIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: fixtureGetDisabled, Validator: testutil.RequireHeaders})

	out, err := execute(t, "spam-filters", "get", "dash0-cli_drop-noisy-health-checks", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml")

	require.NoError(t, err)
	filter, err := decodeV1Alpha1([]byte(out))
	require.NoError(t, err)
	require.NotNil(t, filter.Metadata.Annotations)
	require.NotNil(t, filter.Metadata.Annotations.Dash0Comenabled)
	assert.Equal(t, dash0api.False, *filter.Metadata.Annotations.Dash0Comenabled)
}

const (
	disabledV1Alpha1Doc = `apiVersion: v1alpha1
kind: Dash0SpamFilter
metadata:
  name: Drop noisy health checks
  labels:
    dash0.com/origin: dash0-cli_drop-noisy-health-checks
  annotations:
    dash0.com/enabled: "false"
spec:
  contexts:
    - log
  filter:
    - key: k8s.namespace.name
      operator: is
      value: kube-system
`
	disabledV1Alpha2Doc = `apiVersion: v1alpha2
kind: Dash0SpamFilter
metadata:
  name: Drop debug logs
  labels:
    dash0.com/origin: dash0-cli_drop-debug-logs
  annotations:
    dash0.com/enabled: "false"
spec:
  context: log
  filter:
    - key: otel.log.severity.range
      operator: is
      value: DEBUG
`
)

// sentEnabledAnnotation returns the dash0.com/enabled annotation of the spam
// filter in a request body, or nil when the body has none. Both apiVersions
// share the metadata shape, so decoding as v1alpha1 reads either.
func sentEnabledAnnotation(t *testing.T, body []byte) *dash0api.SpamFilterAnnotationsDash0Comenabled {
	t.Helper()
	var sent dash0api.SpamFilter
	require.NoError(t, json.Unmarshal(body, &sent))
	if sent.Metadata.Annotations == nil {
		return nil
	}
	return sent.Metadata.Annotations.Dash0Comenabled
}

// A disabled filter must stay disabled: the API reads a write without the
// dash0.com/enabled annotation as enabled.
func TestCreateSpamFilter_KeepsDisabledAnnotation(t *testing.T) {
	for _, tc := range []struct {
		name, doc, origin, response string
	}{
		{"v1alpha1", disabledV1Alpha1Doc, "dash0-cli_drop-noisy-health-checks", fixtureGetDisabled},
		{"v1alpha2", disabledV1Alpha2Doc, "dash0-cli_drop-debug-logs", fixtureGetSuccessV1A2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.SetupTestEnv(t)
			server := testutil.NewMockServer(t, testutil.FixturesDir())
			server.OnPattern(http.MethodGet, spamFilterIDPattern, testutil.MockResponse{StatusCode: http.StatusNotFound, BodyFile: fixtureNotFound, Validator: testutil.RequireHeaders})
			server.OnPattern(http.MethodPut, spamFilterIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: tc.response, Validator: testutil.RequireHeaders})

			_, err := execute(t, "spam-filters", "create", "-f", writeDoc(t, tc.doc), "--api-url", server.URL, "--auth-token", testAuthToken)

			require.NoError(t, err)
			req := server.LastRequest()
			require.NotNil(t, req)
			require.Equal(t, http.MethodPut, req.Method)
			assert.Equal(t, apiPathSpamFilters+"/"+tc.origin, req.Path)
			enabled := sentEnabledAnnotation(t, req.Body)
			require.NotNil(t, enabled, "the request must carry dash0.com/enabled")
			assert.Equal(t, dash0api.False, *enabled)
		})
	}
}

func TestUpdateSpamFilter_KeepsDisabledAnnotation(t *testing.T) {
	for _, tc := range []struct {
		name, doc, getFixture string
	}{
		{"v1alpha1", disabledV1Alpha1Doc, fixtureGetSuccess},
		{"v1alpha2", disabledV1Alpha2Doc, fixtureGetSuccessV1A2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.SetupTestEnv(t)
			server := testutil.NewMockServer(t, testutil.FixturesDir())
			server.OnPattern(http.MethodGet, spamFilterIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: tc.getFixture, Validator: testutil.RequireHeaders})
			server.OnPattern(http.MethodPut, spamFilterIDPattern, testutil.MockResponse{StatusCode: http.StatusOK, BodyFile: tc.getFixture, Validator: testutil.RequireHeaders})

			_, err := execute(t, "spam-filters", "update", "-f", writeDoc(t, tc.doc), "--api-url", server.URL, "--auth-token", testAuthToken)

			require.NoError(t, err)
			req := server.LastRequest()
			require.NotNil(t, req)
			require.Equal(t, http.MethodPut, req.Method)
			enabled := sentEnabledAnnotation(t, req.Body)
			require.NotNil(t, enabled, "the request must carry dash0.com/enabled")
			assert.Equal(t, dash0api.False, *enabled)
		})
	}
}

func lastField(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}
