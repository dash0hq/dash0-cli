package agentmode

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintJSONHelp(t *testing.T) {
	root := &cobra.Command{
		Use:   "test",
		Short: "Test command",
	}
	root.Flags().StringP("output", "o", "table", "Output format")

	sub := &cobra.Command{
		Use:     "sub",
		Short:   "Sub command",
		Aliases: []string{"s"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	sub.Flags().Bool("force", false, "Skip prompt")
	root.AddCommand(sub)

	var buf bytes.Buffer
	require.NoError(t, PrintJSONHelp(&buf, root))

	var result commandHelp
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Equal(t, "test", result.Name)
	assert.Equal(t, "Test command", result.Description)
	// Local flags include the ones we registered plus cobra's built-in help flag
	var outputFlag *flagHelp
	for i := range result.Flags {
		if result.Flags[i].Name == "output" {
			outputFlag = &result.Flags[i]
		}
	}
	require.NotNil(t, outputFlag, "expected to find 'output' flag")
	assert.Equal(t, "o", outputFlag.Shorthand)
	require.Len(t, result.Subcommands, 1)
	assert.Equal(t, "sub", result.Subcommands[0].Name)
	assert.Equal(t, []string{"s"}, result.Subcommands[0].Aliases)
	require.Len(t, result.Subcommands[0].Flags, 1)
	assert.Equal(t, "force", result.Subcommands[0].Flags[0].Name)
}
