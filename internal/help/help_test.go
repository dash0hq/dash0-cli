package help

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

// Usage strings routinely contain `<placeholder>` fragments (e.g.
// `dash0 skill show <topic>`). Agent-mode help is terminal/agent output,
// not HTML, so the JSON encoder must not turn those into `<topic>`.
func TestPrintJSONHelpDoesNotHTMLEscape(t *testing.T) {
	root := &cobra.Command{
		Use:     "show <topic>",
		Short:   "Prints entry for a <topic> — & other bits",
		Example: "  dash0 skill show <topic>",
	}

	var buf bytes.Buffer
	require.NoError(t, PrintJSONHelp(&buf, root))

	raw := buf.String()
	assert.NotContains(t, raw, "\\u003c", "encoder should not HTML-escape `<`")
	assert.NotContains(t, raw, "\\u003e", "encoder should not HTML-escape `>`")
	assert.NotContains(t, raw, "\\u0026", "encoder should not HTML-escape `&`")
	assert.Contains(t, raw, "<topic>")
	assert.Contains(t, raw, "& other bits")
}
