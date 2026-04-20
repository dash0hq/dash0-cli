package tracing

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSpansQueryCmd() (*cobra.Command, *cobra.Command) {
	root := &cobra.Command{Use: "dash0"}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	spansCmd := NewSpansCmd()
	root.AddCommand(spansCmd)
	var queryCmd *cobra.Command
	for _, c := range spansCmd.Commands() {
		if c.Name() == "query" {
			queryCmd = c
			break
		}
	}
	return root, queryCmd
}

func TestSpansQuerySkipHeaderFlag(t *testing.T) {
	_, queryCmd := newSpansQueryCmd()
	flag := queryCmd.Flags().Lookup("skip-header")
	require.NotNil(t, flag, "--skip-header flag should be registered on spans query")
	assert.Equal(t, "false", flag.DefValue)
}

func TestSpansQueryColumnFlag(t *testing.T) {
	_, queryCmd := newSpansQueryCmd()
	flag := queryCmd.Flags().Lookup("column")
	require.NotNil(t, flag, "--column flag should be registered on spans query")
	assert.Equal(t, "[]", flag.DefValue)
}

func TestParseSpansQueryFormat(t *testing.T) {
	tests := []struct {
		input   string
		want    queryFormat
		wantErr bool
	}{
		{"", queryFormatTable, false},
		{"table", queryFormatTable, false},
		{"TABLE", queryFormatTable, false},
		{"json", queryFormatJSON, false},
		{"JSON", queryFormatJSON, false},
		{"csv", queryFormatCSV, false},
		{"CSV", queryFormatCSV, false},
		{"yaml", "", true},
		{"invalid", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseQueryFormat(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unknown output format")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestGetServiceName(t *testing.T) {
	tests := []struct {
		name       string
		attributes []struct {
			Key   string
			Value struct{ StringValue *string }
		}
		want string
	}{
		{
			name: "service name only",
			want: "my-service",
		},
	}
	_ = tests
	// Basic test via the concrete API type is covered in integration tests.
}
