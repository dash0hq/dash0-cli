package logs

import (
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseQueryFormat(t *testing.T) {
	tests := []struct {
		input   string
		want    queryFormat
		wantErr bool
	}{
		{"", queryFormatTable, false},
		{"table", queryFormatTable, false},
		{"TABLE", queryFormatTable, false},
		{"otlp-json", queryFormatOtlpJSON, false},
		{"OTLP-JSON", queryFormatOtlpJSON, false},
		{"csv", queryFormatCSV, false},
		{"CSV", queryFormatCSV, false},
		{"yaml", "", true},
		{"json", "", true},
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

func TestExtractBodyString(t *testing.T) {
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

func TestSeverityName(t *testing.T) {
	text := "INFO"
	var num9 int32 = 9
	var num17 int32 = 17
	var num0 int32 = 0

	// Text takes precedence
	assert.Equal(t, "INFO", severityName(&num9, &text))
	// Number only
	assert.Equal(t, "INFO", severityName(&num9, nil))
	assert.Equal(t, "ERROR", severityName(&num17, nil))
	assert.Equal(t, "UNSPECIFIED", severityName(&num0, nil))
	// Both nil
	assert.Equal(t, "", severityName(nil, nil))
	// Empty text falls back to number
	empty := ""
	assert.Equal(t, "ERROR", severityName(&num17, &empty))
}
