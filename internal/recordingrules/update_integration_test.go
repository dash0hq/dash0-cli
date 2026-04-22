//go:build integration

package recordingrules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAuthToken = "auth_test_token"

func TestUpdateRecordingRule_IDMismatch(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "rule.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: CPU Usage Average
  labels:
    dash0.com/id: file-id-1111-2222-3333-444444444444
spec:
  groups:
    - name: cpu-averages
      rules:
        - record: instance:cpu_usage:avg5m
          expr: avg without(cpu) (rate(node_cpu_seconds_total{mode!="idle"}[5m]))
`), 0644)
	require.NoError(t, err)

	cmd := NewRecordingRulesCmd()
	cmd.SetArgs([]string{"update", "arg-id-aaaa-bbbb-cccc-dddddddddddd", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "does not match")
	assert.Contains(t, cmdErr.Error(), "arg-id-aaaa-bbbb-cccc-dddddddddddd")
	assert.Contains(t, cmdErr.Error(), "file-id-1111-2222-3333-444444444444")
}

func TestUpdateRecordingRule_NoIDAnywhere(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "rule.yaml")
	err := os.WriteFile(yamlFile, []byte(`apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: CPU Usage Average
spec:
  groups:
    - name: cpu-averages
      rules:
        - record: instance:cpu_usage:avg5m
          expr: avg without(cpu) (rate(node_cpu_seconds_total{mode!="idle"}[5m]))
`), 0644)
	require.NoError(t, err)

	cmd := NewRecordingRulesCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "no recording rule ID provided as argument, and the file does not contain an ID")
}
