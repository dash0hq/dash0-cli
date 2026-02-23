package query

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/otlp"
	"github.com/dash0hq/dash0-cli/internal/output"
)

// ColumnSpec represents a user-provided column specification from --column.
type ColumnSpec struct {
	Key string // user-provided key (alias or attribute key)
}

// ColumnDef defines a single output column.
type ColumnDef struct {
	Key     string              // canonical attribute key, e.g. "otel.log.time"
	Aliases []string            // short aliases, e.g. ["timestamp", "time"]; case-insensitive
	Header  string              // table header, e.g. "TIMESTAMP"
	Width   int                 // max width for table (upper bound); 0 = unlimited (last col)
	ColorFn func(string, int) string // optional color+pad formatter: (value, width) → styled string
}

// ParseColumns parses a list of --column flag values into a list of ColumnSpec.
// Each value is a single column specification: an alias or attribute key.
func ParseColumns(specs []string) ([]ColumnSpec, error) {
	var result []ColumnSpec
	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		result = append(result, ColumnSpec{Key: spec})
	}
	return result, nil
}

// ResolveColumns resolves user-provided column specs against the default column
// definitions. Each spec is matched by alias (case-insensitive) or by canonical
// key (case-sensitive). Unknown keys become arbitrary attribute columns.
func ResolveColumns(specs []ColumnSpec, defaults []ColumnDef) []ColumnDef {
	var result []ColumnDef
	for _, spec := range specs {
		def, matchedAlias, found := findColumnDef(spec.Key, defaults)
		if !found {
			// Arbitrary attribute column: header = key as-is
			result = append(result, ColumnDef{
				Key:    spec.Key,
				Header: spec.Key,
				Width:  widthForHeader(spec.Key, 30),
			})
			continue
		}

		col := def
		if matchedAlias != "" {
			// Matched by alias: header = uppercased alias
			col.Header = strings.ToUpper(matchedAlias)
		} else {
			// Matched by canonical key: header = key as-is
			col.Header = col.Key
		}
		col.Width = widthForHeader(col.Header, col.Width)
		result = append(result, col)
	}
	return result
}

// widthForHeader returns a width that is at least as wide as the header text.
// A width of 0 means unlimited (last column) and is never adjusted.
func widthForHeader(header string, width int) int {
	if width > 0 && len(header) > width {
		return len(header)
	}
	return width
}

// findColumnDef finds a ColumnDef matching the given key. It first checks
// aliases (case-insensitive), then canonical keys (case-sensitive).
// Returns the matched definition, the alias that matched (empty if matched by
// canonical key), and whether a match was found.
func findColumnDef(key string, defaults []ColumnDef) (ColumnDef, string, bool) {
	keyLower := strings.ToLower(key)
	// Check aliases first (case-insensitive)
	for _, d := range defaults {
		for _, alias := range d.Aliases {
			if strings.ToLower(alias) == keyLower {
				return d, key, true
			}
		}
	}
	// Check canonical key (case-sensitive)
	for _, d := range defaults {
		if d.Key == key {
			return d, "", true
		}
	}
	return ColumnDef{}, "", false
}

// ValidateColumnFormat rejects --column with JSON output.
func ValidateColumnFormat(columns []string, outputFormat string) error {
	if len(columns) == 0 {
		return nil
	}
	lower := strings.ToLower(outputFormat)
	if lower == "json" {
		return fmt.Errorf("--column is not supported with JSON output; use jq to reshape JSON output")
	}
	return nil
}

// RenderTable buffers all rows, computes optimal column widths (using Width as
// an upper bound), and renders the complete table. For each non-zero-width
// column, the effective width is min(maxWidth, max(headerLen, maxValueLen)).
// When skipHeader is true, header length is excluded from the computation.
// Width 0 (unlimited, typically the last column) stays unlimited.
func RenderTable(w io.Writer, cols []ColumnDef, rows []map[string]string, skipHeader bool) {
	effectiveWidths := computeEffectiveWidths(cols, rows, skipHeader)

	lastIdx := len(cols) - 1

	if !skipHeader {
		var parts []string
		for i, col := range cols {
			if i == lastIdx {
				parts = append(parts, col.Header)
			} else {
				parts = append(parts, fmt.Sprintf("%-*s", effectiveWidths[i], col.Header))
			}
		}
		fmt.Fprintln(w, strings.Join(parts, "  "))
	}

	for _, values := range rows {
		var parts []string
		for i, col := range cols {
			val := values[col.Key]
			if i == lastIdx {
				if col.ColorFn != nil {
					val = col.ColorFn(val, 0)
				}
				parts = append(parts, val)
			} else {
				ew := effectiveWidths[i]
				if col.Width > 0 {
					val = output.Truncate(val, col.Width)
				}
				if col.ColorFn != nil {
					val = col.ColorFn(val, ew)
				} else {
					val = fmt.Sprintf("%-*s", ew, val)
				}
				parts = append(parts, val)
			}
		}
		fmt.Fprintln(w, strings.Join(parts, "  "))
	}
}

// computeEffectiveWidths computes the optimal width for each column based on
// header length (unless skipHeader) and actual data values, capped at the
// column's max width.
func computeEffectiveWidths(cols []ColumnDef, rows []map[string]string, skipHeader bool) []int {
	widths := make([]int, len(cols))
	for i, col := range cols {
		if col.Width == 0 {
			continue
		}
		var ew int
		if !skipHeader {
			ew = len(col.Header)
		}
		for _, row := range rows {
			val := row[col.Key]
			truncated := output.Truncate(val, col.Width)
			if len(truncated) > ew {
				ew = len(truncated)
			}
		}
		if ew > col.Width {
			ew = col.Width
		}
		widths[i] = ew
	}
	return widths
}

// CSVHeader returns the CSV header row for the given columns using canonical keys.
func CSVHeader(cols []ColumnDef) []string {
	row := make([]string, len(cols))
	for i, col := range cols {
		row[i] = col.Key
	}
	return row
}

// CSVRow returns a CSV data row for the given columns and values.
func CSVRow(cols []ColumnDef, values map[string]string) []string {
	row := make([]string, len(cols))
	for i, col := range cols {
		row[i] = values[col.Key]
	}
	return row
}

// WriteCSVHeader writes the CSV header row.
func WriteCSVHeader(w *csv.Writer, cols []ColumnDef) error {
	return w.Write(CSVHeader(cols))
}

// WriteCSVRow writes a CSV data row.
func WriteCSVRow(w *csv.Writer, cols []ColumnDef, values map[string]string) error {
	return w.Write(CSVRow(cols, values))
}

// BuildValues merges predefined values with attribute lookups for arbitrary columns.
func BuildValues(predefined map[string]string, cols []ColumnDef, rawAttrs []dash0api.KeyValue) map[string]string {
	result := make(map[string]string, len(cols))
	for k, v := range predefined {
		result[k] = v
	}
	for _, col := range cols {
		if _, ok := result[col.Key]; !ok {
			result[col.Key] = otlp.FindAttribute(rawAttrs, col.Key)
		}
	}
	return result
}
