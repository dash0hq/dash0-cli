package asset

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadMultiDocumentYAML_SingleDocument(t *testing.T) {
	yaml := `kind: Dashboard
metadata:
  name: test-dashboard
spec:
  display:
    name: Test Dashboard
`
	docs, err := ReadMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "Dashboard", docs[0].Kind)
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
	docs, err := ReadMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	require.Len(t, docs, 3)
	assert.Equal(t, "Dashboard", docs[0].Kind)
	assert.Equal(t, "CheckRule", docs[1].Kind)
	assert.Equal(t, "View", docs[2].Kind)
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
	docs, err := ReadMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	// Parser creates documents for each YAML document, including empty ones
	// The actual kind validation happens later in apply's runApply
	require.GreaterOrEqual(t, len(docs), 2)

	// Filter to find documents with valid kinds
	var validDocs []Document
	for _, doc := range docs {
		if doc.Kind != "" {
			validDocs = append(validDocs, doc)
		}
	}
	require.Len(t, validDocs, 2)
	assert.Equal(t, "Dashboard", validDocs[0].Kind)
	assert.Equal(t, "View", validDocs[1].Kind)
}

func TestReadMultiDocumentYAML_EmptyInput(t *testing.T) {
	_, err := ReadMultiDocumentYAML("-", strings.NewReader(""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no input provided")
}

func TestReadMultiDocumentYAML_InvalidYAML(t *testing.T) {
	yaml := `kind: Dashboard
  invalid yaml: [
    unclosed bracket
`
	_, err := ReadMultiDocumentYAML("-", strings.NewReader(yaml))
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
	docs, err := ReadMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	require.Len(t, docs, 1)

	// Verify the raw content contains expected fields
	raw := string(docs[0].Raw)
	assert.Contains(t, raw, "name: test-rule")
	assert.Contains(t, raw, "expression: up == 0")
	assert.Contains(t, raw, "severity: critical")
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
	docs, err := ReadMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "PrometheusRule", docs[0].Kind)
}

func TestPersesDashboardParsing(t *testing.T) {
	yaml := `apiVersion: perses.dev/v1alpha1
kind: PersesDashboard
metadata:
  name: test-perses-dashboard
  labels:
    dash0.com/id: test-perses-id
spec:
  display:
    name: Test Perses Dashboard
  duration: 5m
  panels: {}
`
	docs, err := ReadMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "PersesDashboard", docs[0].Kind)
	assert.Equal(t, "Test Perses Dashboard", docs[0].Name)
	assert.Equal(t, "test-perses-id", docs[0].ID)
}

func TestPersesDashboardParsing_V1Alpha2(t *testing.T) {
	yaml := `apiVersion: perses.dev/v1alpha2
kind: PersesDashboard
metadata:
  name: test-v1alpha2
spec:
  config:
    display:
      name: V1Alpha2 Dashboard
    duration: 10m
`
	docs, err := ReadMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "PersesDashboard", docs[0].Kind)
	assert.Equal(t, "V1Alpha2 Dashboard", docs[0].Name)
}

func TestReadMultiDocumentYAML_FromBuffer(t *testing.T) {
	yaml := `kind: Dashboard
metadata:
  name: buffer-test
`
	buf := bytes.NewBufferString(yaml)
	docs, err := ReadMultiDocumentYAML("-", buf)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "Dashboard", docs[0].Kind)
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
	docs, err := ReadMultiDocumentYAML("-", strings.NewReader(yaml))
	require.NoError(t, err)
	require.Len(t, docs, 3)
	assert.Equal(t, 1, docs[0].DocIndex)
	assert.Equal(t, 2, docs[1].DocIndex)
	assert.Equal(t, 3, docs[2].DocIndex)
	// All should have DocCount = 3
	for _, doc := range docs {
		assert.Equal(t, 3, doc.DocCount)
	}
}

func TestLocation_SingleFileMultiDoc(t *testing.T) {
	doc := Document{DocIndex: 2, DocCount: 3}
	assert.Equal(t, "document 2", doc.Location())
}

func TestLocation_SingleFileSingleDoc(t *testing.T) {
	doc := Document{DocIndex: 1, DocCount: 1}
	assert.Equal(t, "document 1", doc.Location())
}

func TestLocation_DirectoryMultiDoc(t *testing.T) {
	doc := Document{FilePath: "dashboards/prod.yaml", DocIndex: 2, DocCount: 3}
	assert.Equal(t, "dashboards/prod.yaml: document 2", doc.Location())
}

func TestLocation_DirectorySingleDoc(t *testing.T) {
	doc := Document{FilePath: "dashboards/prod.yaml", DocIndex: 1, DocCount: 1}
	assert.Equal(t, "dashboards/prod.yaml", doc.Location())
}

func TestDiscoverFiles_Basic(t *testing.T) {
	dir := t.TempDir()
	// Create files
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("kind: Dashboard"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.yml"), []byte("kind: View"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.txt"), []byte("not yaml"), 0644))

	files, err := DiscoverFiles(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"a.yaml", "b.yml"}, files)
}

func TestDiscoverFiles_NestedDirs(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "root.yaml"), []byte("kind: Dashboard"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "mid.yml"), []byte("kind: View"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "deep", "leaf.yaml"), []byte("kind: CheckRule"), 0644))

	files, err := DiscoverFiles(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"root.yaml",
		filepath.Join("sub", "deep", "leaf.yaml"),
		filepath.Join("sub", "mid.yml"),
	}, files)
}

func TestDiscoverFiles_SkipsHidden(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".hidden"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".hidden", "secret.yaml"), []byte("kind: Dashboard"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".dotfile.yaml"), []byte("kind: View"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "visible.yaml"), []byte("kind: CheckRule"), 0644))

	files, err := DiscoverFiles(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"visible.yaml"}, files)
}

// TestDiscoverFiles_DotPrefixedTargetItselfIsNotHidden pins a deliberate
// behavior: a dot-prefixed directory explicitly passed via -f (e.g.
// -f .dash0-assets/) is a deliberate user choice, not something to skip —
// only path components *inside* it are checked against the hidden-name
// rule.
func TestDiscoverFiles_DotPrefixedTargetItselfIsNotHidden(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, ".dash0-assets")
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dashboard.yaml"), []byte("kind: Dashboard"), 0644))

	files, err := DiscoverFiles(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"dashboard.yaml"}, files)
}

func TestDiscoverFiles_Sorted(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "z.yaml"), []byte("kind: Dashboard"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("kind: View"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "m.yaml"), []byte("kind: CheckRule"), 0644))

	files, err := DiscoverFiles(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"a.yaml", "m.yaml", "z.yaml"}, files)
}

func TestDiscoverFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	_, err := DiscoverFiles(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no .yaml or .yml files found")
}

func TestDiscoverFiles_CaseInsensitiveExtensions(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "upper.YAML"), []byte("kind: Dashboard"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mixed.Yml"), []byte("kind: View"), 0644))

	files, err := DiscoverFiles(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"mixed.Yml", "upper.YAML"}, files)
}

func TestReadDirectory_SetsFilePath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dashboard.yaml"), []byte("kind: Dashboard\nmetadata:\n  name: test\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "view.yaml"), []byte("kind: View\nmetadata:\n  name: test\n"), 0644))

	docs, err := ReadDirectory(dir)
	require.NoError(t, err)
	require.Len(t, docs, 2)
	assert.Equal(t, "dashboard.yaml", docs[0].FilePath)
	assert.Equal(t, "Dashboard", docs[0].Kind)
	assert.Equal(t, "view.yaml", docs[1].FilePath)
	assert.Equal(t, "View", docs[1].Kind)
}

func TestReadDirectory_MultiDocFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "multi.yaml"), []byte("kind: Dashboard\nmetadata:\n  name: d1\n---\nkind: View\nmetadata:\n  name: v1\n"), 0644))

	docs, err := ReadDirectory(dir)
	require.NoError(t, err)
	require.Len(t, docs, 2)
	assert.Equal(t, "multi.yaml", docs[0].FilePath)
	assert.Equal(t, "multi.yaml", docs[1].FilePath)
	assert.Equal(t, 1, docs[0].DocIndex)
	assert.Equal(t, 2, docs[1].DocIndex)
	assert.Equal(t, 2, docs[0].DocCount)
	assert.Equal(t, 2, docs[1].DocCount)
}

func TestReadMultiDocumentYAML_ExtractsNameAndId(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		expectedName string
		expectedId   string
	}{
		{
			name: "Dashboard: display name and dash0Extensions.id",
			yaml: `kind: Dashboard
metadata:
  name: Production Overview
  dash0Extensions:
    id: a1b2c3d4-5678-90ab-cdef-1234567890ab
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
			name: "PersesDashboard: display name and dash0.com/id label",
			yaml: `apiVersion: perses.dev/v1alpha1
kind: PersesDashboard
metadata:
  name: my-perses-dashboard
  labels:
    dash0.com/id: perses-dashboard-id
spec:
  display:
    name: My Perses Dashboard
`,
			expectedName: "My Perses Dashboard",
			expectedId:   "perses-dashboard-id",
		},
		{
			name: "PersesDashboard: metadata.name as fallback, no ID",
			yaml: `apiVersion: perses.dev/v1alpha1
kind: PersesDashboard
metadata:
  name: fallback-perses-name
spec:
  duration: 5m
`,
			expectedName: "fallback-perses-name",
			expectedId:   "",
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
			docs, err := ReadMultiDocumentYAML("-", strings.NewReader(tt.yaml))
			require.NoError(t, err)
			require.Len(t, docs, 1)
			assert.Equal(t, tt.expectedName, docs[0].Name)
			assert.Equal(t, tt.expectedId, docs[0].ID)
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
			name:         "Dashboard: display name and dash0Extensions.id",
			yaml:         "kind: Dashboard\nmetadata:\n  name: My Dashboard\n  dash0Extensions:\n    id: uuid-123\nspec:\n  display:\n    name: My Dashboard\n",
			expectedKind: "Dashboard",
			expectedName: "My Dashboard",
			expectedId:   "uuid-123",
		},
		{
			name:         "Dashboard: metadata.name as display name fallback, no ID without dash0Extensions",
			yaml:         "kind: Dashboard\nmetadata:\n  name: My Dashboard\n",
			expectedKind: "Dashboard",
			expectedName: "My Dashboard",
			expectedId:   "",
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
			name:         "PersesDashboard: display name and label ID",
			yaml:         "apiVersion: perses.dev/v1alpha1\nkind: PersesDashboard\nmetadata:\n  name: perses-name\n  labels:\n    dash0.com/id: perses-id\nspec:\n  display:\n    name: Perses Display Name\n",
			expectedKind: "PersesDashboard",
			expectedName: "Perses Display Name",
			expectedId:   "perses-id",
		},
		{
			name:         "PersesDashboard: metadata.name fallback without display name",
			yaml:         "apiVersion: perses.dev/v1alpha1\nkind: PersesDashboard\nmetadata:\n  name: perses-fallback\nspec:\n  duration: 5m\n",
			expectedKind: "PersesDashboard",
			expectedName: "perses-fallback",
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
			kind, name, id, err := ParseDocumentHeader([]byte(tt.yaml))
			require.NoError(t, err)
			assert.Equal(t, tt.expectedKind, kind)
			assert.Equal(t, tt.expectedName, name)
			assert.Equal(t, tt.expectedId, id)
		})
	}
}

func TestFormatNameAndID(t *testing.T) {
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
			result := FormatNameAndID(tt.docName, tt.id)
			assert.Equal(t, tt.expected, result)
		})
	}
}
