package output

import (
	"bytes"
	"testing"

	"github.com/dash0hq/dash0-cli/internal"
	"github.com/stretchr/testify/assert"
)

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input    string
		expected Format
		hasError bool
	}{
		{"table", FormatTable, false},
		{"TABLE", FormatTable, false},
		{"", FormatTable, false},
		{"json", FormatJSON, false},
		{"JSON", FormatJSON, false},
		{"yaml", FormatYAML, false},
		{"yml", FormatYAML, false},
		{"wide", FormatWide, false},
		{"invalid", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result, err := ParseFormat(tc.input)
			if tc.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestFormatter_PrintJSON(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatJSON, &buf)

	data := map[string]string{"name": "test", "id": "123"}
	err := f.PrintJSON(data)
	assert.NoError(t, err)

	expected := "{\n  \"id\": \"123\",\n  \"name\": \"test\"\n}\n"
	assert.Equal(t, expected, buf.String())
}

func TestFormatter_PrintYAML(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatYAML, &buf)

	data := map[string]string{"name": "test", "id": "123"}
	err := f.PrintYAML(data)
	assert.NoError(t, err)

	// YAML output may vary slightly, just check it contains the values
	assert.Contains(t, buf.String(), "name: test")
	assert.Contains(t, buf.String(), "id: \"123\"")
}

func TestFormatter_Print(t *testing.T) {
	data := map[string]string{"name": "test"}

	t.Run("JSON", func(t *testing.T) {
		var buf bytes.Buffer
		f := NewFormatter(FormatJSON, &buf)
		err := f.Print(data)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "\"name\": \"test\"")
	})

	t.Run("YAML", func(t *testing.T) {
		var buf bytes.Buffer
		f := NewFormatter(FormatYAML, &buf)
		err := f.Print(data)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "name: test")
	})

	t.Run("Table returns error", func(t *testing.T) {
		var buf bytes.Buffer
		f := NewFormatter(FormatTable, &buf)
		err := f.Print(data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "use type-specific Print method")
	})
}

type testItem struct {
	Name string
	ID   string
}

func TestFormatter_PrintTable(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)

	columns := []Column{
		{Header: "ID", Width: 10, Value: func(item interface{}) string {
			return item.(testItem).ID
		}},
		{Header: internal.HEADER_NAME, Width: 20, Value: func(item interface{}) string {
			return item.(testItem).Name
		}},
	}

	data := []interface{}{
		testItem{ID: "123", Name: "Dashboard One"},
		testItem{ID: "456", Name: "Dashboard Two"},
	}

	err := f.PrintTable(columns, data)
	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "ID")
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "123")
	assert.Contains(t, output, "Dashboard One")
	assert.Contains(t, output, "456")
	assert.Contains(t, output, "Dashboard Two")
}

func TestFormatter_PrintTable_SkipHeader(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf, WithSkipHeader(true))

	columns := []Column{
		{Header: "ID", Width: 10, Value: func(item interface{}) string {
			return item.(testItem).ID
		}},
		{Header: internal.HEADER_NAME, Width: 20, Value: func(item interface{}) string {
			return item.(testItem).Name
		}},
	}

	data := []interface{}{
		testItem{ID: "123", Name: "Dashboard One"},
		testItem{ID: "456", Name: "Dashboard Two"},
	}

	err := f.PrintTable(columns, data)
	assert.NoError(t, err)

	output := buf.String()
	assert.NotContains(t, output, "ID")
	assert.NotContains(t, output, "NAME")
	assert.Contains(t, output, "123")
	assert.Contains(t, output, "Dashboard One")
	assert.Contains(t, output, "456")
	assert.Contains(t, output, "Dashboard Two")
}

func TestFormatter_PrintTable_Empty(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)

	err := f.PrintTable([]Column{}, []interface{}{})
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "No assets found")
}

func TestValidateSkipHeader(t *testing.T) {
	t.Run("skip-header false never errors", func(t *testing.T) {
		for _, format := range []string{"table", "wide", "json", "yaml", "csv", ""} {
			assert.NoError(t, ValidateSkipHeader(false, format))
		}
	})

	t.Run("skip-header true with compatible formats", func(t *testing.T) {
		for _, format := range []string{"table", "wide", "csv", ""} {
			assert.NoError(t, ValidateSkipHeader(true, format), "format %q should be compatible", format)
		}
	})

	t.Run("skip-header true with incompatible formats", func(t *testing.T) {
		for _, format := range []string{"json", "yaml"} {
			err := ValidateSkipHeader(true, format)
			assert.Error(t, err, "format %q should be incompatible", format)
			assert.Contains(t, err.Error(), "--skip-header is not supported")
		}
	})
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a long string", 10, "this is..."},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := Truncate(tc.input, tc.maxLen)
			assert.Equal(t, tc.expected, result)
		})
	}
}
