package query

import (
	"bytes"
	"encoding/csv"
	"testing"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseColumns(t *testing.T) {
	t.Run("simple list", func(t *testing.T) {
		cols, err := ParseColumns([]string{"timestamp", "severity", "body"})
		require.NoError(t, err)
		require.Len(t, cols, 3)
		assert.Equal(t, "timestamp", cols[0].Key)
		assert.Equal(t, "severity", cols[1].Key)
		assert.Equal(t, "body", cols[2].Key)
	})

	t.Run("alias with spaces", func(t *testing.T) {
		cols, err := ParseColumns([]string{"start time", "duration"})
		require.NoError(t, err)
		require.Len(t, cols, 2)
		assert.Equal(t, "start time", cols[0].Key)
		assert.Equal(t, "duration", cols[1].Key)
	})

	t.Run("nil spec", func(t *testing.T) {
		cols, err := ParseColumns(nil)
		require.NoError(t, err)
		assert.Nil(t, cols)
	})

	t.Run("empty slice", func(t *testing.T) {
		cols, err := ParseColumns([]string{})
		require.NoError(t, err)
		assert.Nil(t, cols)
	})

	t.Run("whitespace entries skipped", func(t *testing.T) {
		cols, err := ParseColumns([]string{"  ", "body"})
		require.NoError(t, err)
		require.Len(t, cols, 1)
		assert.Equal(t, "body", cols[0].Key)
	})

}

func TestResolveColumns(t *testing.T) {
	defaults := []ColumnDef{
		{Key: "otel.log.time", Aliases: []string{"timestamp", "time"}, Header: "TIMESTAMP", Width: 28},
		{Key: "otel.log.severity.range", Aliases: []string{"severity"}, Header: "SEVERITY", Width: 10},
		{Key: "otel.log.body", Aliases: []string{"body"}, Header: "BODY", Width: 0},
	}

	t.Run("all predefined by alias", func(t *testing.T) {
		specs := []ColumnSpec{
			{Key: "timestamp"},
			{Key: "severity"},
			{Key: "body"},
		}
		cols := ResolveColumns(specs, defaults)
		require.Len(t, cols, 3)
		assert.Equal(t, "otel.log.time", cols[0].Key)
		assert.Equal(t, "TIMESTAMP", cols[0].Header)
		assert.Equal(t, "otel.log.severity.range", cols[1].Key)
		assert.Equal(t, "SEVERITY", cols[1].Header)
		assert.Equal(t, "otel.log.body", cols[2].Key)
		assert.Equal(t, "BODY", cols[2].Header)
	})

	t.Run("by canonical key uses key as header", func(t *testing.T) {
		specs := []ColumnSpec{{Key: "otel.log.time"}}
		cols := ResolveColumns(specs, defaults)
		require.Len(t, cols, 1)
		assert.Equal(t, "otel.log.time", cols[0].Key)
		assert.Equal(t, "otel.log.time", cols[0].Header)
		assert.Equal(t, 28, cols[0].Width, "width stays at predefined when header fits")
	})

	t.Run("width grows when header is longer than predefined width", func(t *testing.T) {
		specs := []ColumnSpec{{Key: "otel.log.severity.range"}}
		cols := ResolveColumns(specs, defaults)
		require.Len(t, cols, 1)
		assert.Equal(t, "otel.log.severity.range", cols[0].Header)
		assert.Equal(t, len("otel.log.severity.range"), cols[0].Width,
			"width should grow to fit header")
	})

	t.Run("alias case-insensitive", func(t *testing.T) {
		specs := []ColumnSpec{{Key: "TIMESTAMP"}}
		cols := ResolveColumns(specs, defaults)
		require.Len(t, cols, 1)
		assert.Equal(t, "otel.log.time", cols[0].Key)
		assert.Equal(t, "TIMESTAMP", cols[0].Header)
	})

	t.Run("short alias uses uppercased alias as header", func(t *testing.T) {
		specs := []ColumnSpec{{Key: "time"}}
		cols := ResolveColumns(specs, defaults)
		require.Len(t, cols, 1)
		assert.Equal(t, "otel.log.time", cols[0].Key)
		assert.Equal(t, "TIME", cols[0].Header)
	})

	t.Run("unknown keys become arbitrary columns", func(t *testing.T) {
		specs := []ColumnSpec{{Key: "http.method"}}
		cols := ResolveColumns(specs, defaults)
		require.Len(t, cols, 1)
		assert.Equal(t, "http.method", cols[0].Key)
		assert.Equal(t, "http.method", cols[0].Header)
		assert.Equal(t, 30, cols[0].Width)
	})

}

func TestValidateColumnFormat(t *testing.T) {
	t.Run("reject json", func(t *testing.T) {
		err := ValidateColumnFormat([]string{"timestamp"}, "json")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not supported with JSON")
	})

	t.Run("reject JSON uppercase", func(t *testing.T) {
		err := ValidateColumnFormat([]string{"timestamp"}, "JSON")
		require.Error(t, err)
	})

	t.Run("allow table", func(t *testing.T) {
		err := ValidateColumnFormat([]string{"timestamp"}, "table")
		require.NoError(t, err)
	})

	t.Run("allow csv", func(t *testing.T) {
		err := ValidateColumnFormat([]string{"timestamp"}, "csv")
		require.NoError(t, err)
	})

	t.Run("allow empty format", func(t *testing.T) {
		err := ValidateColumnFormat([]string{"timestamp"}, "")
		require.NoError(t, err)
	})

	t.Run("empty columns always valid", func(t *testing.T) {
		err := ValidateColumnFormat(nil, "json")
		require.NoError(t, err)
	})
}

func TestRenderTable(t *testing.T) {
	t.Run("basic rendering", func(t *testing.T) {
		cols := []ColumnDef{
			{Key: "a", Header: "COL A", Width: 10},
			{Key: "b", Header: "COL B", Width: 0},
		}
		rows := []map[string]string{
			{"a": "short", "b": "the rest"},
		}
		var buf bytes.Buffer
		RenderTable(&buf, cols, rows, false)
		assert.Equal(t, "COL A  COL B\nshort  the rest\n", buf.String())
	})

	t.Run("skip header", func(t *testing.T) {
		cols := []ColumnDef{
			{Key: "a", Header: "COL A", Width: 10},
			{Key: "b", Header: "COL B", Width: 0},
		}
		rows := []map[string]string{
			{"a": "val", "b": "rest"},
		}
		var buf bytes.Buffer
		RenderTable(&buf, cols, rows, true)
		// Header is skipped, so effective width = max(len("val")) = 3, not 5
		assert.Equal(t, "val  rest\n", buf.String())
	})

	t.Run("truncation at max width", func(t *testing.T) {
		cols := []ColumnDef{
			{Key: "a", Header: "A", Width: 8},
			{Key: "b", Header: "B", Width: 0},
		}
		rows := []map[string]string{
			{"a": "a very long value", "b": "ok"},
		}
		var buf bytes.Buffer
		RenderTable(&buf, cols, rows, false)
		assert.Contains(t, buf.String(), "a ver...")
	})

	t.Run("columns shrink to fit data", func(t *testing.T) {
		cols := []ColumnDef{
			{Key: "a", Header: "NAME", Width: 30},
			{Key: "b", Header: "VAL", Width: 0},
		}
		rows := []map[string]string{
			{"a": "short", "b": "x"},
			{"a": "medium123", "b": "y"},
		}
		var buf bytes.Buffer
		RenderTable(&buf, cols, rows, false)
		// Effective width should be max(len("NAME"), len("medium123")) = 9, not 30.
		// Header "NAME" is padded to 9, values are padded to 9.
		lines := bytes.Split(buf.Bytes(), []byte("\n"))
		header := string(lines[0])
		assert.Equal(t, "NAME       VAL", header)
		assert.Equal(t, "short      x", string(lines[1]))
		assert.Equal(t, "medium123  y", string(lines[2]))
	})

	t.Run("columns do not exceed max width", func(t *testing.T) {
		cols := []ColumnDef{
			{Key: "a", Header: "A", Width: 8},
			{Key: "b", Header: "B", Width: 0},
		}
		rows := []map[string]string{
			{"a": "this is way too long for the column", "b": "ok"},
		}
		var buf bytes.Buffer
		RenderTable(&buf, cols, rows, false)
		lines := bytes.Split(buf.Bytes(), []byte("\n"))
		// Effective width should be 8 (the max), since the truncated value "this ..." is 8 chars.
		header := string(lines[0])
		assert.Equal(t, "A         B", header)
		assert.Contains(t, string(lines[1]), "this ...")
	})

	t.Run("empty rows", func(t *testing.T) {
		cols := []ColumnDef{
			{Key: "a", Header: "COL A", Width: 10},
			{Key: "b", Header: "COL B", Width: 0},
		}
		var buf bytes.Buffer
		RenderTable(&buf, cols, nil, false)
		assert.Equal(t, "COL A  COL B\n", buf.String())
	})

	t.Run("header wider than all values", func(t *testing.T) {
		cols := []ColumnDef{
			{Key: "a", Header: "LONG HEADER", Width: 30},
			{Key: "b", Header: "B", Width: 0},
		}
		rows := []map[string]string{
			{"a": "hi", "b": "x"},
		}
		var buf bytes.Buffer
		RenderTable(&buf, cols, rows, false)
		lines := bytes.Split(buf.Bytes(), []byte("\n"))
		// Effective width = max(len("LONG HEADER"), len("hi")) = 11
		assert.Equal(t, "LONG HEADER  B", string(lines[0]))
		assert.Equal(t, "hi           x", string(lines[1]))
	})
}

func TestCSVOutput(t *testing.T) {
	cols := []ColumnDef{
		{Key: "otel.log.time", Header: "TIMESTAMP"},
		{Key: "otel.log.body", Header: "BODY"},
	}
	values := map[string]string{"otel.log.time": "2024-01-01", "otel.log.body": "hello"}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	require.NoError(t, WriteCSVHeader(w, cols))
	require.NoError(t, WriteCSVRow(w, cols, values))
	w.Flush()

	assert.Equal(t, "otel.log.time,otel.log.body\n2024-01-01,hello\n", buf.String())
}

func TestBuildValues(t *testing.T) {
	strVal := "my-service"
	predefined := map[string]string{
		"otel.log.time": "2024-01-01",
		"otel.log.body": "hello",
	}
	rawAttrs := []dash0api.KeyValue{
		{Key: "service.name", Value: dash0api.AnyValue{StringValue: &strVal}},
	}
	cols := []ColumnDef{
		{Key: "otel.log.time"},
		{Key: "otel.log.body"},
		{Key: "service.name"},
	}

	values := BuildValues(predefined, cols, rawAttrs)
	assert.Equal(t, "2024-01-01", values["otel.log.time"])
	assert.Equal(t, "hello", values["otel.log.body"])
	assert.Equal(t, "my-service", values["service.name"])
}

func TestBuildValuesMissingAttribute(t *testing.T) {
	predefined := map[string]string{"otel.log.time": "2024-01-01"}
	cols := []ColumnDef{
		{Key: "otel.log.time"},
		{Key: "missing.attr"},
	}

	values := BuildValues(predefined, cols, nil)
	assert.Equal(t, "2024-01-01", values["otel.log.time"])
	assert.Equal(t, "", values["missing.attr"])
}
