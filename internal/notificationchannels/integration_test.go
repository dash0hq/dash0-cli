//go:build integration

package notificationchannels

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	apiPathNotificationChannels = "/api/notification-channels"
	testAuthToken               = "auth_test_token"
)

var notificationChannelIDPattern = regexp.MustCompile(`^/api/notification-channels/[^/]+$`)

// newExperimentalNotificationChannelsCmd creates a root command with the --experimental persistent
// flag and the notification-channels subcommand attached, mirroring the real command tree.
func newExperimentalNotificationChannelsCmd() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewNotificationChannelsCmd())
	return root
}

func TestListNotificationChannels_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathNotificationChannels, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureNotificationChannelsListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalNotificationChannelsCmd()
	cmd.SetArgs([]string{"-X", "notification-channels", "list", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "TYPE")
	assert.Contains(t, output, "ID")
	assert.Contains(t, output, "Slack Alerts")
	assert.Contains(t, output, "PagerDuty On-Call")
	assert.Contains(t, output, "Email Digest")
}

func TestListNotificationChannels_Empty(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathNotificationChannels, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureNotificationChannelsListEmpty,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalNotificationChannelsCmd()
	cmd.SetArgs([]string{"-X", "notification-channels", "list", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "No notification channels found.")
}

func TestListNotificationChannels_JSON(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathNotificationChannels, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureNotificationChannelsListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalNotificationChannelsCmd()
	cmd.SetArgs([]string{"-X", "notification-channels", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	var parsed []interface{}
	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Len(t, parsed, 3)
}

func TestListNotificationChannels_YAML(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathNotificationChannels, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureNotificationChannelsListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalNotificationChannelsCmd()
	cmd.SetArgs([]string{"-X", "notification-channels", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "kind: Dash0NotificationChannel")
	assert.Contains(t, output, "metadata:")
	assert.Contains(t, output, "spec:")
	assert.Contains(t, output, "---")
}

func TestListNotificationChannels_CSV(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathNotificationChannels, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureNotificationChannelsListSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalNotificationChannelsCmd()
	cmd.SetArgs([]string{"-X", "notification-channels", "list", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "csv"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.GreaterOrEqual(t, len(lines), 4) // header + 3 channels
	assert.Equal(t, "name,type,id,origin", lines[0])
	assert.Contains(t, lines[1], "Slack Alerts")
}

func TestListNotificationChannels_Unauthorized(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodGet, apiPathNotificationChannels, testutil.MockResponse{
		StatusCode: http.StatusUnauthorized,
		BodyFile:   testutil.FixtureNotificationChannelsUnauthorized,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalNotificationChannelsCmd()
	cmd.SetArgs([]string{"-X", "notification-channels", "list", "--api-url", server.URL, "--auth-token", "auth_invalid"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestListNotificationChannels_RequiresExperimental(t *testing.T) {
	testutil.SetupTestEnv(t)

	cmd := newExperimentalNotificationChannelsCmd()
	cmd.SetArgs([]string{"notification-channels", "list", "--api-url", "http://unused", "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental")
}

func TestGetNotificationChannel_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, notificationChannelIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureNotificationChannelsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalNotificationChannelsCmd()
	cmd.SetArgs([]string{"-X", "notification-channels", "get", "abc-123-def-456", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "Kind:  Dash0NotificationChannel")
	assert.Contains(t, output, "Name:  Slack Alerts")
	assert.Contains(t, output, "Type:  slack")
	assert.Contains(t, output, "ID:    abc-123-def-456")
}

func TestGetNotificationChannel_JSON(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, notificationChannelIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureNotificationChannelsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalNotificationChannelsCmd()
	cmd.SetArgs([]string{"-X", "notification-channels", "get", "abc-123-def-456", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "json"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.Equal(t, "Dash0NotificationChannel", parsed["kind"])
}

func TestGetNotificationChannel_YAML(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, notificationChannelIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureNotificationChannelsGetSuccess,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalNotificationChannelsCmd()
	cmd.SetArgs([]string{"-X", "notification-channels", "get", "abc-123-def-456", "--api-url", server.URL, "--auth-token", testAuthToken, "-o", "yaml"})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "kind: Dash0NotificationChannel")
	assert.Contains(t, output, "name: Slack Alerts")
}

func TestGetNotificationChannel_NotFound(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodGet, notificationChannelIDPattern, testutil.MockResponse{
		StatusCode: http.StatusNotFound,
		BodyFile:   testutil.FixtureNotificationChannelsNotFound,
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalNotificationChannelsCmd()
	cmd.SetArgs([]string{"-X", "notification-channels", "get", "nonexistent-id", "--api-url", server.URL, "--auth-token", testAuthToken})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestCreateNotificationChannel_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.On(http.MethodPost, apiPathNotificationChannels, testutil.MockResponse{
		StatusCode: http.StatusOK,
		BodyFile:   testutil.FixtureNotificationChannelsCreateSuccess,
		Validator:  testutil.RequireHeaders,
	})

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "channel.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: Dash0NotificationChannel
metadata:
  name: Slack Alerts
spec:
  type: slack
  config:
    url: https://hooks.slack.com/services/T00/B00/XXX
`), 0644)
	require.NoError(t, err)

	cmd := newExperimentalNotificationChannelsCmd()
	cmd.SetArgs([]string{"-X", "notification-channels", "create", "-f", yamlFile, "--api-url", server.URL, "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "Slack Alerts")
	assert.Contains(t, output, "created")
}

func TestCreateNotificationChannel_DryRun(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "channel.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: Dash0NotificationChannel
metadata:
  name: Slack Alerts
spec:
  type: slack
  config:
    url: https://hooks.slack.com/services/T00/B00/XXX
`), 0644)
	require.NoError(t, err)

	cmd := newExperimentalNotificationChannelsCmd()
	cmd.SetArgs([]string{"-X", "notification-channels", "create", "-f", yamlFile, "--dry-run", "--api-url", "http://unused", "--auth-token", testAuthToken})

	var cmdErr error
	output := testutil.CaptureStdout(t, func() {
		cmdErr = cmd.Execute()
	})

	require.NoError(t, cmdErr)
	assert.Contains(t, output, "Dry run")
}

func TestDeleteNotificationChannel_Success(t *testing.T) {
	testutil.SetupTestEnv(t)

	server := testutil.NewMockServer(t, testutil.FixturesDir())
	server.OnPattern(http.MethodDelete, notificationChannelIDPattern, testutil.MockResponse{
		StatusCode: http.StatusOK,
		Body:       map[string]any{},
		Validator:  testutil.RequireHeaders,
	})

	cmd := newExperimentalNotificationChannelsCmd()
	cmd.SetArgs([]string{"-X", "notification-channels", "delete", "abc-123-def-456", "--force", "--api-url", server.URL, "--auth-token", testAuthToken})

	var err error
	output := testutil.CaptureStdout(t, func() {
		err = cmd.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "deleted")
}

func TestUpdateNotificationChannel_IDMismatch(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "channel.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: Dash0NotificationChannel
metadata:
  name: Slack Alerts
  labels:
    dash0.com/id: file-id-1111-2222-3333
spec:
  type: slack
  config:
    url: https://hooks.slack.com/services/T00/B00/XXX
`), 0644)
	require.NoError(t, err)

	cmd := newExperimentalNotificationChannelsCmd()
	cmd.SetArgs([]string{"-X", "notification-channels", "update", "arg-id-aaaa-bbbb-cccc", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "does not match")
	assert.Contains(t, cmdErr.Error(), "arg-id-aaaa-bbbb-cccc")
	assert.Contains(t, cmdErr.Error(), "file-id-1111-2222-3333")
}

func TestUpdateNotificationChannel_NoIDAnywhere(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "channel.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: Dash0NotificationChannel
metadata:
  name: Slack Alerts
spec:
  type: slack
  config:
    url: https://hooks.slack.com/services/T00/B00/XXX
`), 0644)
	require.NoError(t, err)

	cmd := newExperimentalNotificationChannelsCmd()
	cmd.SetArgs([]string{"-X", "notification-channels", "update", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "no notification channel ID provided as argument, and the file does not contain an ID")
}
