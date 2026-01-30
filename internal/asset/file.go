package asset

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ReadDefinition reads a YAML or JSON definition from a file or stdin.
// If path is "-" or empty, it reads from stdin (assumes YAML format).
// Otherwise, it reads from the file at the given path.
func ReadDefinition(path string, target interface{}, stdin io.Reader) error {
	if path == "-" || path == "" {
		return readFromStdin(stdin, target)
	}
	return ReadDefinitionFile(path, target)
}

// ReadDefinitionFile reads a YAML or JSON file and unmarshals into the target.
// It auto-detects the format based on file extension, falling back to YAML first.
func ReadDefinitionFile(path string, target interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", path, err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, target); err != nil {
			return fmt.Errorf("failed to parse YAML from %s: %w", path, err)
		}
	case ".json":
		if err := json.Unmarshal(data, target); err != nil {
			return fmt.Errorf("failed to parse JSON from %s: %w", path, err)
		}
	default:
		// Try YAML first, then JSON
		if err := yaml.Unmarshal(data, target); err != nil {
			if jsonErr := json.Unmarshal(data, target); jsonErr != nil {
				return fmt.Errorf("failed to parse file %s (tried YAML and JSON): yaml error: %v, json error: %v", path, err, jsonErr)
			}
		}
	}

	return nil
}

// readFromStdin reads YAML or JSON from stdin and unmarshals into the target.
// It tries YAML first, then JSON (same behavior as files without extension).
func readFromStdin(stdin io.Reader, target interface{}) error {
	data, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("failed to read from stdin: %w", err)
	}

	if len(data) == 0 {
		return fmt.Errorf("no input provided on stdin")
	}

	// Try YAML first, then JSON
	if err := yaml.Unmarshal(data, target); err != nil {
		if jsonErr := json.Unmarshal(data, target); jsonErr != nil {
			return fmt.Errorf("failed to parse stdin (tried YAML and JSON): yaml error: %v, json error: %v", err, jsonErr)
		}
	}

	return nil
}

// WriteDefinitionFile writes data to a YAML or JSON file.
// It auto-detects the format based on file extension, defaulting to YAML.
func WriteDefinitionFile(path string, data interface{}) error {
	ext := strings.ToLower(filepath.Ext(path))

	var output []byte
	var err error

	switch ext {
	case ".json":
		output, err = json.MarshalIndent(data, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		output = append(output, '\n')
	default: // Default to YAML with 2-space indent (Kubernetes standard)
		output, err = marshalYAML(data)
		if err != nil {
			return fmt.Errorf("failed to marshal YAML: %w", err)
		}
	}

	if err := os.WriteFile(path, output, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}

	return nil
}

// WriteToStdout writes data to stdout in the specified format.
// Format can be "yaml", "yml", or "json".
func WriteToStdout(format string, data interface{}) error {
	switch strings.ToLower(format) {
	case "json":
		output, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(output))
	default: // Default to YAML with 2-space indent (Kubernetes standard)
		output, err := marshalYAML(data)
		if err != nil {
			return fmt.Errorf("failed to marshal YAML: %w", err)
		}
		fmt.Print(string(output))
	}
	return nil
}

// marshalYAML marshals data to YAML with 2-space indentation (Kubernetes standard)
func marshalYAML(data interface{}) ([]byte, error) {
	var buf strings.Builder
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(data); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}
