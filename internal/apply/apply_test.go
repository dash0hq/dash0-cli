package apply

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeKind(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Dashboard", "dashboard"},
		{"dashboard", "dashboard"},
		{"DASHBOARD", "dashboard"},
		{"CheckRule", "checkrule"},
		{"check-rule", "checkrule"},
		{"check_rule", "checkrule"},
		{"Dash0Dashboard", "dashboard"},
		{"Dash0CheckRule", "checkrule"},
		{"PrometheusRule", "prometheusrule"},
		{"SyntheticCheck", "syntheticcheck"},
		{"synthetic-check", "syntheticcheck"},
		{"View", "view"},
		{"Dash0View", "view"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeKind(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsValidKind(t *testing.T) {
	validKinds := []string{
		"Dashboard",
		"dashboard",
		"CheckRule",
		"checkrule",
		"check-rule",
		"PrometheusRule",
		"prometheusrule",
		"SyntheticCheck",
		"syntheticcheck",
		"synthetic-check",
		"View",
		"view",
		"Dash0Dashboard",
		"Dash0View",
	}

	for _, kind := range validKinds {
		t.Run("valid_"+kind, func(t *testing.T) {
			assert.True(t, isValidKind(kind), "expected %q to be valid", kind)
		})
	}

	invalidKinds := []string{
		"Unknown",
		"Pod",
		"Deployment",
		"ConfigMap",
		"",
		"   ",
	}

	for _, kind := range invalidKinds {
		t.Run("invalid_"+kind, func(t *testing.T) {
			assert.False(t, isValidKind(kind), "expected %q to be invalid", kind)
		})
	}
}

func TestReadMultiDocumentYAML_SingleDocument(t *testing.T) {
	yaml := `kind: Dashboard
metadata:
  name: test-dashboard
spec:
  display:
    name: Test Dashboard
`
	docs, err := readMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "Dashboard", docs[0].kind)
}

func TestReadMultiDocumentYAML_MultipleDocuments(t *testing.T) {
	yaml := `kind: Dashboard
metadata:
  name: dashboard-1
---
kind: CheckRule
name: check-rule-1
---
kind: View
metadata:
  name: view-1
`
	docs, err := readMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	require.Len(t, docs, 3)
	assert.Equal(t, "Dashboard", docs[0].kind)
	assert.Equal(t, "CheckRule", docs[1].kind)
	assert.Equal(t, "View", docs[2].kind)
}

func TestReadMultiDocumentYAML_WithEmptyDocuments(t *testing.T) {
	// The parser will include documents without a kind field
	// Validation of the kind field happens in a separate step
	yaml := `---
kind: Dashboard
metadata:
  name: dashboard-1
---
---
kind: View
metadata:
  name: view-1
---
`
	docs, err := readMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	// Parser creates documents for each YAML document, including empty ones
	// The actual kind validation happens later in runApply
	require.GreaterOrEqual(t, len(docs), 2)

	// Filter to find documents with valid kinds
	var validDocs []assetDocument
	for _, doc := range docs {
		if doc.kind != "" {
			validDocs = append(validDocs, doc)
		}
	}
	require.Len(t, validDocs, 2)
	assert.Equal(t, "Dashboard", validDocs[0].kind)
	assert.Equal(t, "View", validDocs[1].kind)
}

func TestReadMultiDocumentYAML_EmptyInput(t *testing.T) {
	_, err := readMultiDocumentYAML("-", strings.NewReader(""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no input provided")
}

func TestReadMultiDocumentYAML_InvalidYAML(t *testing.T) {
	yaml := `kind: Dashboard
  invalid yaml: [
    unclosed bracket
`
	_, err := readMultiDocumentYAML("-", strings.NewReader(yaml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse YAML")
}

func TestReadMultiDocumentYAML_PreservesRawContent(t *testing.T) {
	yaml := `kind: CheckRule
name: test-rule
expression: up == 0
labels:
  severity: critical
`
	docs, err := readMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	require.Len(t, docs, 1)

	// Verify the raw content contains expected fields
	raw := string(docs[0].raw)
	assert.Contains(t, raw, "name: test-rule")
	assert.Contains(t, raw, "expression: up == 0")
	assert.Contains(t, raw, "severity: critical")
}

func TestApplyAction_String(t *testing.T) {
	assert.Equal(t, "created", string(actionCreated))
	assert.Equal(t, "updated", string(actionUpdated))
}

func TestConvertToCheckRule(t *testing.T) {
	rule := &asset.PrometheusAlertingRule{
		Alert: "HighErrorRate",
		Expr:  "sum(rate(errors[5m])) > 0.1",
		For:   "5m",
		Labels: map[string]string{
			"severity": "critical",
		},
		Annotations: map[string]string{
			"summary":     "High error rate detected",
			"description": "Error rate exceeds threshold",
		},
	}

	checkRule := asset.ConvertToCheckRule(rule, "1m", "test-id")

	assert.Equal(t, "HighErrorRate", checkRule.Name)
	assert.Equal(t, "sum(rate(errors[5m])) > 0.1", checkRule.Expression)
	assert.NotNil(t, checkRule.For)
	assert.Equal(t, "5m", string(*checkRule.For))
	assert.NotNil(t, checkRule.Interval)
	assert.Equal(t, "1m", string(*checkRule.Interval))
	assert.NotNil(t, checkRule.Id)
	assert.Equal(t, "test-id", *checkRule.Id)
	assert.NotNil(t, checkRule.Summary)
	assert.Equal(t, "High error rate detected", *checkRule.Summary)
	assert.NotNil(t, checkRule.Description)
	assert.Equal(t, "Error rate exceeds threshold", *checkRule.Description)
	require.NotNil(t, checkRule.Labels)
	assert.Equal(t, "critical", (*checkRule.Labels)["severity"])
}

func TestConvertToCheckRule_MinimalInput(t *testing.T) {
	rule := &asset.PrometheusAlertingRule{
		Alert: "SimpleAlert",
		Expr:  "up == 0",
	}

	checkRule := asset.ConvertToCheckRule(rule, "", "")

	assert.Equal(t, "SimpleAlert", checkRule.Name)
	assert.Equal(t, "up == 0", checkRule.Expression)
	assert.Nil(t, checkRule.For)
	assert.Nil(t, checkRule.Interval)
	assert.Nil(t, checkRule.Id)
	assert.Nil(t, checkRule.Labels)
}

func TestPrometheusRuleParsing(t *testing.T) {
	yaml := `apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: test-rules
  labels:
    dash0.com/id: test-prom-rule-id
spec:
  groups:
    - name: test-group
      interval: 1m
      rules:
        - alert: HighErrorRate
          expr: sum(rate(errors[5m])) > 0.1
          for: 5m
          labels:
            severity: critical
          annotations:
            summary: High error rate detected
`
	docs, err := readMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "PrometheusRule", docs[0].kind)
}

func TestReadMultiDocumentYAML_FromBuffer(t *testing.T) {
	yaml := `kind: Dashboard
metadata:
  name: buffer-test
`
	buf := bytes.NewBufferString(yaml)
	docs, err := readMultiDocumentYAML("-", buf)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "Dashboard", docs[0].kind)
}

func TestReadMultiDocumentYAML_SetsDocIndex(t *testing.T) {
	yaml := `kind: Dashboard
metadata:
  name: dashboard-1
---
kind: CheckRule
name: check-rule-1
---
kind: View
metadata:
  name: view-1
`
	docs, err := readMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	require.Len(t, docs, 3)
	assert.Equal(t, 1, docs[0].docIndex)
	assert.Equal(t, 2, docs[1].docIndex)
	assert.Equal(t, 3, docs[2].docIndex)
	// All should have docCount = 3
	for _, doc := range docs {
		assert.Equal(t, 3, doc.docCount)
	}
}

func TestLocation_SingleFileMultiDoc(t *testing.T) {
	doc := assetDocument{docIndex: 2, docCount: 3}
	assert.Equal(t, "document 2", doc.location())
}

func TestLocation_SingleFileSingleDoc(t *testing.T) {
	doc := assetDocument{docIndex: 1, docCount: 1}
	assert.Equal(t, "document 1", doc.location())
}

func TestLocation_DirectoryMultiDoc(t *testing.T) {
	doc := assetDocument{filePath: "dashboards/prod.yaml", docIndex: 2, docCount: 3}
	assert.Equal(t, "dashboards/prod.yaml: document 2", doc.location())
}

func TestLocation_DirectorySingleDoc(t *testing.T) {
	doc := assetDocument{filePath: "dashboards/prod.yaml", docIndex: 1, docCount: 1}
	assert.Equal(t, "dashboards/prod.yaml", doc.location())
}

func TestDiscoverFiles_Basic(t *testing.T) {
	dir := t.TempDir()
	// Create files
	os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("kind: Dashboard"), 0644)
	os.WriteFile(filepath.Join(dir, "b.yml"), []byte("kind: View"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("not yaml"), 0644)

	files, err := discoverFiles(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"a.yaml", "b.yml"}, files)
}

func TestDiscoverFiles_NestedDirs(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0755)
	os.WriteFile(filepath.Join(dir, "root.yaml"), []byte("kind: Dashboard"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "mid.yml"), []byte("kind: View"), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "deep", "leaf.yaml"), []byte("kind: CheckRule"), 0644)

	files, err := discoverFiles(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"root.yaml",
		filepath.Join("sub", "deep", "leaf.yaml"),
		filepath.Join("sub", "mid.yml"),
	}, files)
}

func TestDiscoverFiles_SkipsHidden(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".hidden"), 0755)
	os.WriteFile(filepath.Join(dir, ".hidden", "secret.yaml"), []byte("kind: Dashboard"), 0644)
	os.WriteFile(filepath.Join(dir, ".dotfile.yaml"), []byte("kind: View"), 0644)
	os.WriteFile(filepath.Join(dir, "visible.yaml"), []byte("kind: CheckRule"), 0644)

	files, err := discoverFiles(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"visible.yaml"}, files)
}

func TestDiscoverFiles_Sorted(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "z.yaml"), []byte("kind: Dashboard"), 0644)
	os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("kind: View"), 0644)
	os.WriteFile(filepath.Join(dir, "m.yaml"), []byte("kind: CheckRule"), 0644)

	files, err := discoverFiles(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"a.yaml", "m.yaml", "z.yaml"}, files)
}

func TestDiscoverFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	_, err := discoverFiles(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no .yaml or .yml files found")
}

func TestDiscoverFiles_CaseInsensitiveExtensions(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "upper.YAML"), []byte("kind: Dashboard"), 0644)
	os.WriteFile(filepath.Join(dir, "mixed.Yml"), []byte("kind: View"), 0644)

	files, err := discoverFiles(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"mixed.Yml", "upper.YAML"}, files)
}

func TestReadDirectory_SetsFilePath(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "dashboard.yaml"), []byte("kind: Dashboard\nmetadata:\n  name: test\n"), 0644)
	os.WriteFile(filepath.Join(dir, "view.yaml"), []byte("kind: View\nmetadata:\n  name: test\n"), 0644)

	docs, err := readDirectory(dir)
	require.NoError(t, err)
	require.Len(t, docs, 2)
	assert.Equal(t, "dashboard.yaml", docs[0].filePath)
	assert.Equal(t, "Dashboard", docs[0].kind)
	assert.Equal(t, "view.yaml", docs[1].filePath)
	assert.Equal(t, "View", docs[1].kind)
}

func TestReadDirectory_MultiDocFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "multi.yaml"), []byte("kind: Dashboard\nmetadata:\n  name: d1\n---\nkind: View\nmetadata:\n  name: v1\n"), 0644)

	docs, err := readDirectory(dir)
	require.NoError(t, err)
	require.Len(t, docs, 2)
	assert.Equal(t, "multi.yaml", docs[0].filePath)
	assert.Equal(t, "multi.yaml", docs[1].filePath)
	assert.Equal(t, 1, docs[0].docIndex)
	assert.Equal(t, 2, docs[1].docIndex)
	assert.Equal(t, 2, docs[0].docCount)
	assert.Equal(t, 2, docs[1].docCount)
}

func TestReadMultiDocumentYAML_ExtractsNameAndId(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		expectedName string
		expectedId   string
	}{
		{
			name: "Dashboard: display name and metadata.name as ID",
			yaml: `kind: Dashboard
metadata:
  name: a1b2c3d4-5678-90ab-cdef-1234567890ab
spec:
  display:
    name: Production Overview
`,
			expectedName: "Production Overview",
			expectedId:   "a1b2c3d4-5678-90ab-cdef-1234567890ab",
		},
		{
			name: "CheckRule: top-level name and id",
			yaml: `kind: CheckRule
id: b2c3d4e5-6789-01bc-def0-234567890abc
name: High Error Rate
expression: rate(errors[5m]) > 0.1
`,
			expectedName: "High Error Rate",
			expectedId:   "b2c3d4e5-6789-01bc-def0-234567890abc",
		},
		{
			name: "View: metadata.name as name, label as ID",
			yaml: `kind: View
metadata:
  name: Error Logs
  labels:
    dash0.com/id: c3d4e5f6-7890-12cd-ef01-34567890abcd
spec:
  query: "severity >= ERROR"
`,
			expectedName: "Error Logs",
			expectedId:   "c3d4e5f6-7890-12cd-ef01-34567890abcd",
		},
		{
			name: "SyntheticCheck: metadata.name as name, label as ID",
			yaml: `kind: SyntheticCheck
metadata:
  name: API Health Check
  labels:
    dash0.com/id: d4e5f6a7-8901-23de-f012-4567890abcde
`,
			expectedName: "API Health Check",
			expectedId:   "d4e5f6a7-8901-23de-f012-4567890abcde",
		},
		{
			name: "PrometheusRule: metadata.name as name, label as ID",
			yaml: `kind: PrometheusRule
metadata:
  name: test-rules
  labels:
    dash0.com/id: prom-rule-id
`,
			expectedName: "test-rules",
			expectedId:   "prom-rule-id",
		},
		{
			name: "CheckRule without ID",
			yaml: `kind: CheckRule
name: No ID Rule
expression: up == 0
`,
			expectedName: "No ID Rule",
			expectedId:   "",
		},
		{
			name: "View without labels",
			yaml: `kind: View
metadata:
  name: Simple View
`,
			expectedName: "Simple View",
			expectedId:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs, err := readMultiDocumentYAML("-", strings.NewReader(tt.yaml))
			require.NoError(t, err)
			require.Len(t, docs, 1)
			assert.Equal(t, tt.expectedName, docs[0].name)
			assert.Equal(t, tt.expectedId, docs[0].id)
		})
	}
}

func TestParseDocumentHeader(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		expectedKind string
		expectedName string
		expectedId   string
	}{
		{
			name:         "Dashboard: display name and metadata.name as ID",
			yaml:         "kind: Dashboard\nmetadata:\n  name: uuid-123\nspec:\n  display:\n    name: My Dashboard\n",
			expectedKind: "Dashboard",
			expectedName: "My Dashboard",
			expectedId:   "uuid-123",
		},
		{
			name:         "CheckRule: top-level name and id",
			yaml:         "kind: CheckRule\nid: rule-id\nname: My Rule\nexpression: up == 0\n",
			expectedKind: "CheckRule",
			expectedName: "My Rule",
			expectedId:   "rule-id",
		},
		{
			name:         "View: metadata.name and label ID",
			yaml:         "kind: View\nmetadata:\n  name: view-name\n  labels:\n    dash0.com/id: view-id\n",
			expectedKind: "View",
			expectedName: "view-name",
			expectedId:   "view-id",
		},
		{
			name:         "SyntheticCheck: metadata.name and label ID",
			yaml:         "kind: SyntheticCheck\nmetadata:\n  name: check-name\n  labels:\n    dash0.com/id: check-id\n",
			expectedKind: "SyntheticCheck",
			expectedName: "check-name",
			expectedId:   "check-id",
		},
		{
			name:         "PrometheusRule: metadata.name and label ID",
			yaml:         "kind: PrometheusRule\nmetadata:\n  name: prom-name\n  labels:\n    dash0.com/id: prom-id\n",
			expectedKind: "PrometheusRule",
			expectedName: "prom-name",
			expectedId:   "prom-id",
		},
		{
			name:         "View without labels has no ID",
			yaml:         "kind: View\nmetadata:\n  name: view-name\n",
			expectedKind: "View",
			expectedName: "view-name",
			expectedId:   "",
		},
		{
			name:         "CheckRule without id field",
			yaml:         "kind: CheckRule\nname: some-name\nexpression: up == 0\n",
			expectedKind: "CheckRule",
			expectedName: "some-name",
			expectedId:   "",
		},
		{
			name:         "CheckRule inferred from name+expression when kind is missing",
			yaml:         "name: Exported Rule\nexpression: up == 0\nenabled: true\n",
			expectedKind: "CheckRule",
			expectedName: "Exported Rule",
			expectedId:   "",
		},
		{
			name:         "Unknown kind extracts no ID",
			yaml:         "kind: Unknown\nid: id\nmetadata:\n  name: name\n  labels:\n    dash0.com/id: label-id\n",
			expectedKind: "Unknown",
			expectedName: "name",
			expectedId:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, name, id, err := parseDocumentHeader([]byte(tt.yaml))
			require.NoError(t, err)
			assert.Equal(t, tt.expectedKind, kind)
			assert.Equal(t, tt.expectedName, name)
			assert.Equal(t, tt.expectedId, id)
		})
	}
}

func TestFormatNameAndId(t *testing.T) {
	tests := []struct {
		name     string
		docName  string
		id       string
		expected string
	}{
		{"both name and id", "My Dashboard", "uuid-123", `"My Dashboard" (uuid-123)`},
		{"name only", "My Dashboard", "", `"My Dashboard"`},
		{"id only", "", "uuid-123", "(uuid-123)"},
		{"neither", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatNameAndId(tt.docName, tt.id)
			assert.Equal(t, tt.expected, result)
		})
	}
}
