//go:build integration

package spamfilters

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/testutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newExperimentalSpamFiltersCmdForUpdate creates a root command with the --experimental persistent
// flag and the spam-filters subcommand attached, mirroring the real command tree.
func newExperimentalSpamFiltersCmdForUpdate() *cobra.Command {
	root := &cobra.Command{Use: "dash0", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	root.AddCommand(NewSpamFiltersCmd())
	return root
}

func TestUpdateSpamFilter_IDMismatch(t *testing.T) {
	testutil.SetupTestEnv(t)

	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "filter.yaml")
	err := os.WriteFile(yamlFile, []byte(`kind: Dash0SpamFilter
metadata:
  name: Drop noisy health checks
  labels:
    dash0.com/id: file-id-1111-2222-3333-444444444444
spec:
  contexts:
    - log
  filter:
    - key: "k8s.namespace.name"
      value:
        stringValue:
          operator: "equals"
          comparisonValue: "kube-system"
`), 0644)
	require.NoError(t, err)

	cmd := newExperimentalSpamFiltersCmdForUpdate()
	cmd.SetArgs([]string{"-X", "spam-filters", "update", "arg-id-aaaa-bbbb-cccc-dddddddddddd", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "does not match")
	assert.Contains(t, cmdErr.Error(), "arg-id-aaaa-bbbb-cccc-dddddddddddd")
	assert.Contains(t, cmdErr.Error(), "file-id-1111-2222-3333-444444444444")
}

func TestUpdateSpamFilter_NoIDAnywhere(t *testing.T) {
	testutil.SetupTestEnv(t)

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
      value:
        stringValue:
          operator: "equals"
          comparisonValue: "kube-system"
`), 0644)
	require.NoError(t, err)

	cmd := newExperimentalSpamFiltersCmdForUpdate()
	cmd.SetArgs([]string{"-X", "spam-filters", "update", "-f", yamlFile, "--api-url", "http://unused", "--auth-token", testAuthToken})

	cmdErr := cmd.Execute()

	require.Error(t, cmdErr)
	assert.Contains(t, cmdErr.Error(), "no spam filter ID provided as argument, and the file does not contain an ID")
}
