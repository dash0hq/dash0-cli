package asset

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"gopkg.in/yaml.v3"
	sigsyaml "sigs.k8s.io/yaml"
)

// Document represents a parsed YAML document with its kind, extracted
// name/identifier, and raw content. Shared between apply (which
// creates/updates/deletes from it) and diff (which only reads and previews),
// so both commands discover and parse -f's input identically.
type Document struct {
	Kind     string
	Name     string // human-readable name extracted from the document
	ID       string // asset ID extracted from the document (location varies by kind)
	Raw      []byte
	FilePath string // relative path when loaded from a directory, empty for stdin/single-file
	DocIndex int    // 1-based index within the file
	DocCount int    // total number of documents in the file
}

// Location returns a human-readable string describing where this document came from.
func (d Document) Location() string {
	if d.FilePath == "" {
		return fmt.Sprintf("document %d", d.DocIndex)
	}
	if d.DocCount == 1 {
		return d.FilePath
	}
	return fmt.Sprintf("%s: document %d", d.FilePath, d.DocIndex)
}

// Pluralize returns "1 thing" or "N things" depending on count.
// This logic is currently naive and assumes all plurals are formed by adding "s".
// This happens to work for all our asset kinds, errors, documents and other situations
// we use this logic.
func Pluralize(count int, singular string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %ss", count, singular)
}

// FormatNameAndID returns a display string with name and optional ID.
func FormatNameAndID(name, id string) string {
	if name != "" && id != "" {
		return fmt.Sprintf("%q (%s)", name, id)
	}
	if name != "" {
		return fmt.Sprintf("%q", name)
	}
	if id != "" {
		return fmt.Sprintf("(%s)", id)
	}
	return ""
}

// ParseDocumentHeader extracts the kind, human-readable name, and ID from
// raw YAML bytes. It unmarshals into typed structs and delegates to the
// Extract*/Get* functions in this package so every caller (apply, diff, the
// per-asset CRUD commands) uses the same extraction logic.
func ParseDocumentHeader(data []byte) (kind, name, id string, err error) {
	kind, err = dash0yaml.DetectKind(data)
	if err != nil {
		return "", "", "", err
	}

	switch NormalizeKind(kind) {
	case "dashboard":
		var dashboard dash0api.DashboardDefinition
		if err := sigsyaml.Unmarshal(data, &dashboard); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = dash0api.GetDashboardName(&dashboard)
		if name == "" {
			name = dashboard.Metadata.Name
		}
		id = dash0api.GetDashboardID(&dashboard)

	case "checkrule":
		var rule dash0api.PrometheusAlertRule
		if err := sigsyaml.Unmarshal(data, &rule); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = dash0api.GetCheckRuleName(&rule)
		id = dash0api.GetCheckRuleID(&rule)

	case "view":
		var view dash0api.ViewDefinition
		if err := sigsyaml.Unmarshal(data, &view); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = dash0api.GetViewName(&view)
		id = dash0api.GetViewID(&view)

	case "syntheticcheck":
		var check dash0api.SyntheticCheckDefinition
		if err := sigsyaml.Unmarshal(data, &check); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = dash0api.GetSyntheticCheckName(&check)
		id = dash0api.GetSyntheticCheckID(&check)

	case "prometheusrule":
		// We only need metadata (name + ID) here; the Metadata struct has no
		// time.Duration fields, so a partial unmarshal via sigsyaml is safe.
		var partial struct {
			Metadata dash0api.PrometheusRulesMetadata `json:"metadata"`
		}
		if err := sigsyaml.Unmarshal(data, &partial); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = partial.Metadata.Name
		id = partial.Metadata.Labels[dash0api.LabelID]

	case "persesdashboard":
		var perses dash0api.PersesDashboard
		if err := sigsyaml.Unmarshal(data, &perses); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = dash0api.GetPersesDashboardName(&perses)
		id = dash0api.GetPersesDashboardID(&perses)

	case "spamfilter":
		// Metadata fields (name + dash0.com/id label) are identical across
		// v1alpha1 and v1alpha2, so we use the v1alpha1 type for the header
		// peek regardless of the document's apiVersion. The apiVersion-aware
		// dispatch happens in the caller (apply's applySpamFilter).
		var filter dash0api.SpamFilter
		if err := sigsyaml.Unmarshal(data, &filter); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = dash0api.GetSpamFilterName(&filter)
		id = dash0api.GetSpamFilterID(&filter)

	case "notificationchannel":
		var channel dash0api.NotificationChannelDefinition
		if err := sigsyaml.Unmarshal(data, &channel); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = dash0api.GetNotificationChannelName(&channel)
		id = dash0api.GetNotificationChannelID(&channel)
		if id == "" {
			// Notification channels use origin as the upsert key. Surface it
			// as the ID in dry-run/listing output so users can see which
			// existing channel the document will replace.
			id = dash0api.GetNotificationChannelOrigin(&channel)
		}

	case "team":
		var team dash0api.TeamDefinitionV1Alpha1
		if err := sigsyaml.Unmarshal(data, &team); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = dash0api.GetTeamDisplayName(&team)
		if name == "" {
			name = dash0api.GetTeamName(&team)
		}
		id = dash0api.GetTeamID(&team)
		if id == "" {
			// Teams use origin as the upsert key.
			id = dash0api.GetTeamOrigin(&team)
		}

	default:
		var raw map[string]any
		if err := sigsyaml.Unmarshal(data, &raw); err != nil {
			return "", "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name = yamlStringFromMap(raw, "metadata", "name")
	}

	return kind, name, id, nil
}

// yamlStringFromMap traverses a nested map[string]any by the given keys and
// returns the leaf value as a string, or "" if any key is missing or the
// value is not a string.
func yamlStringFromMap(m map[string]any, keys ...string) string {
	for i, key := range keys {
		val, ok := m[key]
		if !ok {
			return ""
		}
		if i == len(keys)-1 {
			s, _ := val.(string)
			return s
		}
		m, ok = val.(map[string]any)
		if !ok {
			return ""
		}
	}
	return ""
}

// ReadMultiDocumentYAML splits a YAML stream into individual documents.
// This is the only place that requires gopkg.in/yaml.v3 directly —
// sigs.k8s.io/yaml doesn't provide a streaming decoder for multi-document YAML.
func ReadMultiDocumentYAML(filePath string, stdin io.Reader) ([]Document, error) {
	var data []byte
	var err error

	if filePath == "-" {
		data, err = io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read from stdin: %w", err)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("no input provided on stdin")
		}
	} else {
		data, err = os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
	}

	return ParseMultiDocumentYAML(data)
}

// ParseMultiDocumentYAML splits data on YAML document boundaries and parses
// each into a Document. Factored out of ReadMultiDocumentYAML so callers
// that already have file content in memory (e.g. --since's git-history name
// lookups, which read a blob via git rather than a path on disk) can reuse
// the same parsing without a round trip through the filesystem.
func ParseMultiDocumentYAML(data []byte) ([]Document, error) {
	var documents []Document
	decoder := yaml.NewDecoder(bytes.NewReader(data))

	for {
		var node yaml.Node
		err := decoder.Decode(&node)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse YAML:\n    %w", err)
		}

		// Skip empty documents
		if node.Kind == 0 {
			continue
		}

		// Re-encode the node to get the raw bytes for this document
		var buf bytes.Buffer
		encoder := yaml.NewEncoder(&buf)
		encoder.SetIndent(2)
		if err := encoder.Encode(&node); err != nil {
			return nil, fmt.Errorf("failed to re-encode document: %w", err)
		}
		encoder.Close()

		kind, name, id, err := ParseDocumentHeader(buf.Bytes())
		if err != nil {
			return nil, err
		}

		documents = append(documents, Document{
			Kind:     kind,
			Name:     name,
			ID:       id,
			Raw:      buf.Bytes(),
			DocIndex: len(documents) + 1,
		})
	}

	// Handle single-document files without YAML document markers
	if len(documents) == 0 && len(data) > 0 {
		kind, name, id, _ := ParseDocumentHeader(data)
		if kind != "" {
			documents = append(documents, Document{
				Kind:     kind,
				Name:     name,
				ID:       id,
				Raw:      data,
				DocIndex: 1,
			})
		}
	}

	// Set DocCount on all documents
	for i := range documents {
		documents[i].DocCount = len(documents)
	}

	return documents, nil
}

// DiscoverFiles recursively finds all .yaml/.yml files under dirPath,
// skipping hidden entries (names starting with '.').
// Returns paths relative to dirPath, sorted lexicographically.
func DiscoverFiles(dirPath string) ([]string, error) {
	var paths []string
	var hasNestedDirs bool
	if err := filepath.WalkDir(dirPath, FindNonHiddenYAMLFiles(dirPath, &paths, &hasNestedDirs)); err != nil {
		return nil, fmt.Errorf("failed to scan directory: %w", err)
	}

	var files []string
	for _, path := range paths {
		rel, err := filepath.Rel(dirPath, path)
		if err != nil {
			return nil, err
		}
		files = append(files, rel)
	}
	if len(files) == 0 {
		if hasNestedDirs {
			return nil, fmt.Errorf("no .yaml or .yml files found in %s and nested directories", dirPath)
		}
		return nil, fmt.Errorf("no .yaml or .yml files found in %s", dirPath)
	}
	sort.Strings(files)
	return files, nil
}

// ReadDirectory reads all YAML files from a directory recursively.
func ReadDirectory(dirPath string) ([]Document, error) {
	files, err := DiscoverFiles(dirPath)
	if err != nil {
		return nil, err
	}

	var allDocs []Document
	for _, relPath := range files {
		fullPath := filepath.Join(dirPath, relPath)
		docs, err := ReadMultiDocumentYAML(fullPath, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", relPath, err)
		}
		for i := range docs {
			docs[i].FilePath = relPath
		}
		allDocs = append(allDocs, docs...)
	}

	return allDocs, nil
}
