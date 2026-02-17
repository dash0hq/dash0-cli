package experimental

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCmd() (*cobra.Command, *cobra.Command) {
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")

	child := &cobra.Command{Use: "child"}
	root.AddCommand(child)
	return root, child
}

func TestRequireExperimental_FlagNotSet(t *testing.T) {
	_, child := newTestCmd()
	err := RequireExperimental(child)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental command")
	assert.Contains(t, err.Error(), "--experimental")
	assert.Contains(t, err.Error(), "-X")
}

func TestRequireExperimental_FlagSet(t *testing.T) {
	root, _ := newTestCmd()
	// Execute the root command with the flag set so cobra propagates it to the child.
	var childErr error
	for _, c := range root.Commands() {
		if c.Name() == "child" {
			c.RunE = func(cmd *cobra.Command, args []string) error {
				childErr = RequireExperimental(cmd)
				return childErr
			}
		}
	}
	root.SetArgs([]string{"--experimental", "child"})
	require.NoError(t, root.Execute())
	require.NoError(t, childErr)
}

func TestRequireExperimental_FlagNotRegistered(t *testing.T) {
	cmd := &cobra.Command{Use: "standalone"}
	err := RequireExperimental(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "experimental command")
}
