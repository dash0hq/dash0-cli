package asset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadDefinitionFile_YAML(t *testing.T) {
	// Create a temporary YAML file
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "test.yaml")
	yamlContent := `name: test-dashboard
id: "123"
`
	err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
	assert.NoError(t, err)

	var result map[string]string
	err = ReadDefinitionFile(yamlFile, &result)
	assert.NoError(t, err)
	assert.Equal(t, "test-dashboard", result["name"])
	assert.Equal(t, "123", result["id"])
}

func TestReadDefinitionFile_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	jsonFile := filepath.Join(tmpDir, "test.json")
	jsonContent := `{"name": "test-dashboard", "id": "123"}`
	err := os.WriteFile(jsonFile, []byte(jsonContent), 0644)
	assert.NoError(t, err)

	var result map[string]string
	err = ReadDefinitionFile(jsonFile, &result)
	assert.NoError(t, err)
	assert.Equal(t, "test-dashboard", result["name"])
	assert.Equal(t, "123", result["id"])
}

func TestReadDefinitionFile_AutoDetect(t *testing.T) {
	tmpDir := t.TempDir()

	// Test auto-detect with YAML content but no extension
	noExtFile := filepath.Join(tmpDir, "test")
	yamlContent := `name: test
`
	err := os.WriteFile(noExtFile, []byte(yamlContent), 0644)
	assert.NoError(t, err)

	var result map[string]string
	err = ReadDefinitionFile(noExtFile, &result)
	assert.NoError(t, err)
	assert.Equal(t, "test", result["name"])
}

func TestReadDefinitionFile_NotFound(t *testing.T) {
	var result map[string]string
	err := ReadDefinitionFile("/nonexistent/file.yaml", &result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read file")
}

func TestReadDefinitionFile_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "invalid.yaml")
	err := os.WriteFile(invalidFile, []byte("invalid: yaml: content: ["), 0644)
	assert.NoError(t, err)

	var result map[string]string
	err = ReadDefinitionFile(invalidFile, &result)
	assert.Error(t, err)
}

func TestWriteDefinitionFile_YAML(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "output.yaml")

	data := map[string]string{"name": "test", "id": "456"}
	err := WriteDefinitionFile(yamlFile, data)
	assert.NoError(t, err)

	// Read back and verify
	content, err := os.ReadFile(yamlFile)
	assert.NoError(t, err)
	assert.Contains(t, string(content), "name: test")
}

func TestWriteDefinitionFile_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	jsonFile := filepath.Join(tmpDir, "output.json")

	data := map[string]string{"name": "test", "id": "456"}
	err := WriteDefinitionFile(jsonFile, data)
	assert.NoError(t, err)

	// Read back and verify
	content, err := os.ReadFile(jsonFile)
	assert.NoError(t, err)
	assert.Contains(t, string(content), "\"name\": \"test\"")
}

func TestReadDefinition_FromStdin_YAML(t *testing.T) {
	yamlContent := `name: test-dashboard
id: "123"
`
	stdin := strings.NewReader(yamlContent)

	var result map[string]string
	err := ReadDefinition("-", &result, stdin)
	assert.NoError(t, err)
	assert.Equal(t, "test-dashboard", result["name"])
	assert.Equal(t, "123", result["id"])
}

func TestReadDefinition_FromStdin_JSON(t *testing.T) {
	jsonContent := `{"name": "test-dashboard", "id": "123"}`
	stdin := strings.NewReader(jsonContent)

	var result map[string]string
	err := ReadDefinition("-", &result, stdin)
	assert.NoError(t, err)
	assert.Equal(t, "test-dashboard", result["name"])
	assert.Equal(t, "123", result["id"])
}

func TestReadDefinition_FromStdin_Empty(t *testing.T) {
	stdin := strings.NewReader("")

	var result map[string]string
	err := ReadDefinition("-", &result, stdin)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no input provided on stdin")
}

func TestReadDefinition_FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "test.yaml")
	yamlContent := `name: test-dashboard
id: "123"
`
	err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
	assert.NoError(t, err)

	var result map[string]string
	// stdin should be ignored when file path is provided
	err = ReadDefinition(yamlFile, &result, strings.NewReader("ignored"))
	assert.NoError(t, err)
	assert.Equal(t, "test-dashboard", result["name"])
	assert.Equal(t, "123", result["id"])
}
