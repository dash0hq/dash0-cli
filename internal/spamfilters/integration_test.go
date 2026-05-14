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

// newExperimentalSpamFiltersCmd creates a root command with the --experimental persistent
// flag and the spam-filters subcommand attached, mirroring the real command tree.
func newExperimentalSpamFiltersCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewSpamFiltersCmd())
	return root
}

const (
	apiPathSpamFilters       = "/api/spam-filters"
	testAuthToken            = "auth_test_token"
	fixtureListSuccess       = "spamfilters/list_success.json"
	fixtureListEmpty         = "spamfilters/list_empty.json"
	fixtureGetSuccess        = "spamfilters/get_success.json"
	fixtureGetSuccessV1A2    = "spamfilters/get_success_v1alpha2.json"
	fixtureCreateSuccess     = "spamfilters/create_success.json"
	fixtureUnauthorized      = "dashboards/error_unauthorized.json"
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

	cmd := newExperimentalSpamFiltersCmd()
	cmd.SetArgs([]string{"-X", "spam-filters", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json", "--limit", "2"})

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

	cmd := newExperimentalSpamFiltersCmd()
	cmd.SetArgs([]string{"-X", "spam-filters", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml", "--limit", "2"})

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

	cmd := newExperimentalSpamFiltersCmd()
	cmd.SetArgs([]string{"-X", "spam-filters", "list", "--api-url", server.URL, "--auth-token", testAuthToken})

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

	cmd := newExperimentalSpamFiltersCmd()
	cmd.SetArgs([]string{"-X", "spam-filters", "list", "--api-url", server.URL, "--auth-token", testAuthToken})

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

	cmd := newExperimentalSpamFiltersCmd()
	cmd.SetArgs([]string{"-X", "spam-filters", "get", "00000000-0000-0000-0000-000000000001", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json"})

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

	cmd := newExperimentalSpamFiltersCmd()
	cmd.SetArgs([]string{"-X", "spam-filters", "get", "00000000-0000-0000-0000-000000000001", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml"})

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

	cmd := newExperimentalSpamFiltersCmd()
	cmd.SetArgs([]string{"-X", "spam-filters", "get", "00000000-0000-0000-0000-000000000001", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "Kind: Dash0SpamFilter")
	assert.Contains(t, output, "API Version: v1alpha1")
	assert.Contains(t, output, "Name: Drop noisy health checks")
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

	cmd := newExperimentalSpamFiltersCmd()
	cmd.SetArgs([]string{"-X", "spam-filters", "delete", "00000000-0000-0000-0000-000000000001", "--api-url", server.URL, "--auth-token", testAuthToken, "--force"})

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

	cmd := newExperimentalSpamFiltersCmd()
	cmd.SetArgs([]string{"-X", "spam-filters", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "--dataset", "my-dataset"})

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

	cmd := newExperimentalSpamFiltersCmd()
	cmd.SetArgs([]string{"-X", "spam-filters", "create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken, "--dataset", "my-dataset"})

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

	cmd := newExperimentalSpamFiltersCmd()
	cmd.SetArgs([]string{"-X", "spam-filters", "update", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken, "--dataset", "my-dataset"})

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

	cmd := newExperimentalSpamFiltersCmd()
	cmd.SetArgs([]string{"-X", "spam-filters", "get", "00000000-0000-0000-0000-0000000000a2", "--api-url", server.URL, "--auth-token", testAuthToken})

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

	cmd := newExperimentalSpamFiltersCmd()
	cmd.SetArgs([]string{"-X", "spam-filters", "get", "00000000-0000-0000-0000-0000000000a2", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml"})

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

	cmd := newExperimentalSpamFiltersCmd()
	cmd.SetArgs([]string{"-X", "spam-filters", "create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

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
	assert.Equal(t, dash0api.V1alpha2, sent.ApiVersion)
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

	cmd := newExperimentalSpamFiltersCmd()
	cmd.SetArgs([]string{"-X", "spam-filters", "create", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()
	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), `unsupported spam filter apiVersion "v9beta"`)
	assert.Contains(t, cmdErr.Error(), `"v1alpha1"`)
	assert.Contains(t, cmdErr.Error(), `"v1alpha2"`)
}
