package diff

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withExperimentalFlag registers a local --experimental/-X bool flag on cmd,
// standing in for the persistent flag main.go registers on the real root
// command. Standalone tests that construct NewDiffCmd() directly (with no
// root parent) need this for experimental.RequireExperimental's
// cmd.Flags().GetBool("experimental") lookup to succeed instead of silently
// treating the flag as unregistered (=> always disabled).
func withExperimentalFlag(cmd *cobra.Command) {
	cmd.Flags().BoolP("experimental", "X", false, "Enable experimental features")
}

func TestDiff_RequiresExperimentalFlag(t *testing.T) {
	cmd := NewDiffCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"-f", "does-not-need-to-exist.yaml"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental command")
	assert.Contains(t, err.Error(), "--experimental")
}

func TestDiff_RequiresFile(t *testing.T) {
	cmd := NewDiffCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"--experimental"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file is required")
}

func TestDiff_RejectsSinceWithStdin(t *testing.T) {
	cmd := NewDiffCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"-f", "-", "--since", "HEAD~1", "--experimental"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stdin")
}

func TestDiff_RejectsUnexpectedArguments(t *testing.T) {
	cmd := NewDiffCmd()
	withExperimentalFlag(cmd)
	cmd.SetArgs([]string{"-f", "assets.yaml", "extra-arg", "--experimental"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected arguments")
}

func TestPendingDifferencesError_Error(t *testing.T) {
	assert.Equal(t, "1 difference pending", (&PendingDifferencesError{Count: 1}).Error())
	assert.Equal(t, "3 differences pending", (&PendingDifferencesError{Count: 3}).Error())
}
