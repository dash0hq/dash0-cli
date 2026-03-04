//go:build integration

package syntheticchecks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAuthToken = "auth_test_token"

func TestUpdateSyntheticCheck_IDMismatch(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "check.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: SyntheticCheck
metadata:
  name: API Health Check
  labels:
    dash0.com/id: file-id-1111-2222-3333-444444444444
spec:
  url: https://api.example.com/health
  interval: 60s
`), 0644)
	require.NoError(t, err)

	cmd := NewSyntheticChecksCmd()
	cmd.SetArgs([]string{"update", "arg-id-aaaa-bbbb-cccc-dddddddddddd", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "does not match")
	assert.Contains(t, cmdErr.Error(), "arg-id-aaaa-bbbb-cccc-dddddddddddd")
	assert.Contains(t, cmdErr.Error(), "file-id-1111-2222-3333-444444444444")
}

func TestUpdateSyntheticCheck_NoIDAnywhere(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "check.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: SyntheticCheck
metadata:
  name: API Health Check
spec:
  url: https://api.example.com/health
  interval: 60s
`), 0644)
	require.NoError(t, err)

	cmd := NewSyntheticChecksCmd()
	cmd.SetArgs([]string{"update", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "no synthetic check ID provided as argument, and the file does not contain an ID")
}
