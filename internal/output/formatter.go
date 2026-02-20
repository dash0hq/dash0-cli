package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"sigs.k8s.io/yaml"
)

// Format represents the output format type
type Format string

const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
	FormatWide  Format = "wide"
)

// ParseFormat parses a format string into a Format type
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(s) {
	case "table", "":
		return FormatTable, nil
	case "json":
		return FormatJSON, nil
	case "yaml", "yml":
		return FormatYAML, nil
	case "wide":
		return FormatWide, nil
	default:
		return "", fmt.Errorf("unknown output format: %s (valid formats: table, json, yaml, wide)", s)
	}
}

// Formatter handles output formatting
type Formatter struct {
	format     Format
	writer     io.Writer
	skipHeader bool
}

// FormatterOption configures optional Formatter behavior.
type FormatterOption func(*Formatter)

// WithSkipHeader omits the header row from table output.
func WithSkipHeader(skip bool) FormatterOption {
	return func(f *Formatter) {
		f.skipHeader = skip
	}
}

// NewFormatter creates a formatter for the specified output format
func NewFormatter(format Format, w io.Writer, opts ...FormatterOption) *Formatter {
	f := &Formatter{
		format: format,
		writer: w,
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// Format returns the configured format
func (f *Formatter) Format() Format {
	return f.format
}

// PrintJSON outputs data as pretty-printed JSON
func (f *Formatter) PrintJSON(data interface{}) error {
	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// PrintYAML outputs data as YAML via JSON (sigs.k8s.io/yaml). This ensures
// json tags, omitempty, and custom MarshalJSON methods are respected.
func (f *Formatter) PrintYAML(data interface{}) error {
	out, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	_, err = f.writer.Write(out)
	return err
}

// Print outputs data in the configured format (JSON or YAML only)
// For table format, use the type-specific table printing functions
func (f *Formatter) Print(data interface{}) error {
	switch f.format {
	case FormatJSON:
		return f.PrintJSON(data)
	case FormatYAML:
		return f.PrintYAML(data)
	case FormatTable, FormatWide:
		return fmt.Errorf("use type-specific Print method for table format")
	default:
		return fmt.Errorf("unknown format: %s", f.format)
	}
}

// ValidateSkipHeader returns an error if --skip-header is used with a format
// that has no header row (e.g. JSON or YAML).
func ValidateSkipHeader(skipHeader bool, format string) error {
	if !skipHeader {
		return nil
	}
	lower := strings.ToLower(format)
	switch lower {
	case "table", "wide", "csv", "":
		return nil
	default:
		return fmt.Errorf("--skip-header is not supported with output format %q", format)
	}
}

// Column represents a table column configuration
type Column struct {
	Header string
	Width  int
	Value  func(item interface{}) string
}

// PrintTable prints a table with the given columns and data
func (f *Formatter) PrintTable(columns []Column, data []interface{}) error {
	if len(data) == 0 {
		fmt.Fprintln(f.writer, "No assets found.")
		return nil
	}

	lastCol := len(columns) - 1
	var format []string
	for i, col := range columns {
		if i == lastCol {
			format = append(format, "%s")
		} else {
			format = append(format, fmt.Sprintf("%%-%ds", col.Width))
		}
	}

	// Print header — the last column is not padded since nothing follows it
	if !f.skipHeader {
		var headerParts []string
		for i, col := range columns {
			if i == lastCol {
				headerParts = append(headerParts, col.Header)
			} else {
				headerParts = append(headerParts, fmt.Sprintf("%-*s", col.Width, col.Header))
			}
		}
		fmt.Fprintln(f.writer, strings.Join(headerParts, "  "))
	}

	// Print rows
	for _, item := range data {
		var rowParts []string
		for i, col := range columns {
			value := col.Value(item)
			// Truncate if necessary, but never truncate the last column
			if i != lastCol && len(value) > col.Width {
				value = value[:col.Width-3] + "..."
			}
			rowParts = append(rowParts, fmt.Sprintf(format[i], value))
		}
		fmt.Fprintln(f.writer, strings.Join(rowParts, "  "))
	}

	return nil
}

// Truncate truncates a string to the given max length, adding "..." if truncated
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
