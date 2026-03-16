//go:build integration

package recordingrulegroups

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAuthToken = "auth_test_token"

func TestUpdateRecordingRuleGroup_IDMismatch(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "group.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: Dash0RecordingRuleGroup
metadata:
  name: http-metrics
  labels:
    dash0.com/origin: file-origin-1111-2222-3333-444444444444
spec:
  display:
    name: HTTP Metrics
  enabled: true
  interval: 1m
  rules:
    - record: http_request_rate
      expression: rate(http_requests_total[5m])
`), 0644)
	require.NoError(t, err)

	cmd := NewRecordingRuleGroupsCmd()
	cmd.SetArgs([]string{"update", "arg-origin-aaaa-bbbb-cccc-dddddddddddd", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "does not match")
	assert.Contains(t, cmdErr.Error(), "arg-origin-aaaa-bbbb-cccc-dddddddddddd")
	assert.Contains(t, cmdErr.Error(), "file-origin-1111-2222-3333-444444444444")
}

func TestUpdateRecordingRuleGroup_NoIDAnywhere(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "group.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: Dash0RecordingRuleGroup
metadata:
  name: http-metrics
spec:
  display:
    name: HTTP Metrics
  enabled: true
  interval: 1m
  rules:
    - record: http_request_rate
      expression: rate(http_requests_total[5m])
`), 0644)
	require.NoError(t, err)

	cmd := NewRecordingRuleGroupsCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "no recording rule group ID provided as argument, and the file does not contain an ID")
}
