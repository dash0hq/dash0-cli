package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
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
	format Format
	writer io.Writer
}

// NewFormatter creates a formatter for the specified output format
func NewFormatter(format Format, w io.Writer) *Formatter {
	return &Formatter{
		format: format,
		writer: w,
	}
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

// PrintYAML outputs data as YAML
func (f *Formatter) PrintYAML(data interface{}) error {
	encoder := yaml.NewEncoder(f.writer)
	encoder.SetIndent(2)
	defer encoder.Close()
	return encoder.Encode(data)
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

	// Print header
	var headerParts []string
	var format []string
	for _, col := range columns {
		headerParts = append(headerParts, fmt.Sprintf("%-*s", col.Width, col.Header))
		format = append(format, fmt.Sprintf("%%-%ds", col.Width))
	}
	fmt.Fprintln(f.writer, strings.Join(headerParts, "  "))

	// Print rows
	for _, item := range data {
		var rowParts []string
		for i, col := range columns {
			value := col.Value(item)
			// Truncate if necessary
			if len(value) > col.Width {
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
