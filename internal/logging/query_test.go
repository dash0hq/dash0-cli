package logging

import (
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newLogsQueryCmd() (*cobra.Command, *cobra.Command) {
	root := &cobra.Command{Use: "dash0"}
	root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
	logsCmd := NewLogsCmd()
	root.AddCommand(logsCmd)
	var queryCmd *cobra.Command
	for _, c := range logsCmd.Commands() {
		if c.Name() == "query" {
			queryCmd = c
			break
		}
	}
	return root, queryCmd
}

func TestQuerySkipHeaderFlag(t *testing.T) {
	_, queryCmd := newLogsQueryCmd()
	flag := queryCmd.Flags().Lookup("skip-header")
	require.NotNil(t, flag, "--skip-header flag should be registered on logs query")
	assert.Equal(t, "false", flag.DefValue)
}

func TestQueryColumnFlag(t *testing.T) {
	_, queryCmd := newLogsQueryCmd()
	flag := queryCmd.Flags().Lookup("column")
	require.NotNil(t, flag, "--column flag should be registered on logs query")
	assert.Equal(t, "[]", flag.DefValue)
}

func TestParseQueryFormat(t *testing.T) {
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
		{"wide", "", true},
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

func TestGetBodyString(t *testing.T) {
	strVal := "hello"
	intVal := "42"
	doubleVal := 3.14
	boolVal := true

	tests := []struct {
		name string
		body *dash0api.AnyValue
		want string
	}{
		{"nil", nil, ""},
		{"string", &dash0api.AnyValue{StringValue: &strVal}, "hello"},
		{"int", &dash0api.AnyValue{IntValue: &intVal}, "42"},
		{"double", &dash0api.AnyValue{DoubleValue: &doubleVal}, "3.14"},
		{"bool", &dash0api.AnyValue{BoolValue: &boolVal}, "true"},
		{"empty", &dash0api.AnyValue{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractBodyString(tt.body))
		})
	}
}

func TestFormatTimestamp(t *testing.T) {
	// 2024-01-25T12:00:00.000Z in nanoseconds
	assert.Equal(t, "2024-01-25T12:00:00.000Z", formatTimestamp("1706184000000000000"))
	// Invalid input returns as-is
	assert.Equal(t, "not-a-number", formatTimestamp("not-a-number"))
}

func TestSeverityRange(t *testing.T) {
	var num9 int32 = 9
	var num17 int32 = 17
	var num0 int32 = 0

	assert.Equal(t, "INFO", severityRange(&num9))
	assert.Equal(t, "ERROR", severityRange(&num17))
	assert.Equal(t, "UNKNOWN", severityRange(&num0))
	assert.Equal(t, "", severityRange(nil))
}
