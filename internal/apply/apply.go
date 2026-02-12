package apply

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	sigsyaml "sigs.k8s.io/yaml"
)

// Flags for the apply command
type applyFlags struct {
	ApiUrl    string
	AuthToken string
	Dataset   string
	File      string
	DryRun    bool
}

// NewApplyCmd creates the top-level apply command
func NewApplyCmd() *cobra.Command {
	var flags applyFlags

	cmd := &cobra.Command{
		Use:   "apply -f <file|directory>",
		Short: "Apply asset definitions from a file or directory",
		Long: `Apply asset definitions from a YAML file or a directory containing YAML files.
Files must have the .yaml or .yml file extension and may contain
multiple documents separated by "---".

Each document must have a "kind" field specifying the asset type.
Use '-f -' to read documents from stdin.

When a directory is specified, all .yaml and .yml files are discovered
recursively. Hidden files and directories (starting with '.') are skipped.
All documents are validated before any are applied; if any document fails
validation, no changes are made.

Supported asset types:
  - Dashboard
  - CheckRule (or PrometheusRule CRD)
  - SyntheticCheck
  - View

If an asset exists, it will be updated. If it doesn't exist, it will be created.`,
		Example: `  # Apply a single asset
  dash0 apply -f dashboard.yaml

  # Apply multiple assets from a single file
  dash0 apply -f assets.yaml

  # Apply all assets from a directory (recursive)
  dash0 apply -f dashboards/

  # Apply from stdin
  cat assets.yaml | dash0 apply -f -

  # Validate without applying
  dash0 apply -f assets.yaml --dry-run

  # Validate a directory without applying
  dash0 apply -f dashboards/ --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.File == "" {
				return fmt.Errorf("file is required; use -f to specify the file (use '-' for stdin)")
			}
			cmd.SilenceUsage = true
			return runApply(cmd.Context(), &flags)
		},
	}

	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Path to a file or directory containing asset definitions (use '-' for stdin)")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Validate the file without applying changes")
	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API URL for the Dash0 API (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token for the Dash0 API (overrides active profile)")
	cmd.Flags().StringVarP(&flags.Dataset, "dataset", "d", "", "Dataset to operate on")

	return cmd
}

// assetDocument represents a parsed YAML document with its kind
type assetDocument struct {
	kind     string
	name     string // human-readable name extracted from the document
	id       string // asset ID extracted from the document (location varies by kind)
	raw      []byte
	filePath string // relative path when loaded from a directory, empty for stdin/single-file
	docIndex int    // 1-based index within the file
	docCount int    // total number of documents in the file
}

// location returns a human-readable string describing where this document came from.
func (d assetDocument) location() string {
	if d.filePath == "" {
		return fmt.Sprintf("document %d", d.docIndex)
	}
	if d.docCount == 1 {
		return d.filePath
	}
	return fmt.Sprintf("%s: document %d", d.filePath, d.docIndex)
}

// applyAction indicates whether an asset was created or updated
type applyAction string

const (
	actionCreated applyAction = "created"
	actionUpdated applyAction = "updated"
)

// applyResult holds the outcome of applying a single asset.
type applyResult struct {
	kind   string
	name   string
	id     string
	action applyAction
}

func runApply(ctx context.Context, flags *applyFlags) error {
	var documents []assetDocument
	var fromDirectory bool
	var err error

	if flags.File == "-" {
		// Read from stdin
		documents, err = readMultiDocumentYAML("-", os.Stdin)
		if err != nil {
			return validationError(err.Error())
		}
	} else {
		info, statErr := os.Stat(flags.File)
		if statErr != nil {
			return fmt.Errorf("failed to read input: %w", statErr)
		}
		if info.IsDir() {
			fromDirectory = true
			documents, err = readDirectory(flags.File)
			if err != nil {
				return validationError(err.Error())
			}
		} else {
			documents, err = readMultiDocumentYAML(flags.File, nil)
			if err != nil {
				return validationError(err.Error())
			}
		}
	}

	if len(documents) == 0 {
		return validationError("no documents found in input")
	}

	// Validate all documents, collecting all errors
	var validationErrors []string
	for _, doc := range documents {
		if doc.kind == "" {
			validationErrors = append(validationErrors, fmt.Sprintf("%s: missing 'kind' field", doc.location()))
		} else if !isValidKind(doc.kind) {
			validationErrors = append(validationErrors, fmt.Sprintf("%s: unsupported kind %q (supported: Dashboard, CheckRule, PrometheusRule, SyntheticCheck, View)", doc.location(), doc.kind))
		}
	}
	if len(validationErrors) > 0 {
		return validationError(validationErrors...)
	}

	if flags.DryRun {
		return printDryRun(documents, fromDirectory)
	}

	// Create API client
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	// Apply each document
	var applied []string
	for _, doc := range documents {
		results, applyErr := applyDocument(ctx, apiClient, doc, flags.Dataset)
		for _, r := range results {
			displayKind := asset.KindDisplayName(r.kind)
			label := formatNameAndId(r.name, r.id)
			applied = append(applied, fmt.Sprintf("%s %s", displayKind, label))
			if fromDirectory {
				fmt.Printf("%s: %s %s %s\n", doc.filePath, displayKind, label, r.action)
			} else {
				fmt.Printf("%s %s %s\n", displayKind, label, r.action)
			}
		}
		if applyErr != nil {
			if len(applied) > 0 {
				fmt.Println("Applied before error:")
				for _, a := range applied {
					fmt.Printf("  - %s\n", a)
				}
			}
			return fmt.Errorf("%s (%s): %w", doc.location(), doc.kind, applyErr)
		}
	}

	return nil
}

func printDryRun(documents []assetDocument, fromDirectory bool) error {
	if !fromDirectory {
		fmt.Printf("Dry run: %s validated successfully\n", pluralize(len(documents), "document"))
		for i, doc := range documents {
			fmt.Printf("  %d. %s %s\n", i+1, asset.KindDisplayName(doc.kind), formatNameAndId(doc.name, doc.id))
		}
		return nil
	}

	// Count unique files
	fileSet := make(map[string]bool)
	for _, doc := range documents {
		fileSet[doc.filePath] = true
	}
	fmt.Printf("Dry run: %s from %s validated successfully\n", pluralize(len(documents), "document"), pluralize(len(fileSet), "file"))

	// Group by file, preserving order
	var currentFile string
	docInFile := 0
	for _, doc := range documents {
		if doc.filePath != currentFile {
			currentFile = doc.filePath
			docInFile = 0
			fmt.Printf("  %s\n", doc.filePath)
		}
		docInFile++
		fmt.Printf("    %d. %s %s\n", docInFile, asset.KindDisplayName(doc.kind), formatNameAndId(doc.name, doc.id))
	}
	return nil
}

// validationError formats one or more validation issues into a consistent
// "validation failed with N error/errors:" message.
func validationError(issues ...string) error {
	return fmt.Errorf("validation failed with %s:\n  %s", pluralize(len(issues), "error"), strings.Join(issues, "\n  "))
}

// pluralize returns "1 thing" or "N things" depending on count.
// This logic is currently naive and assumes all plurals are formed by adding "s".
// This happens to work for all our asset kinds, errors, documents and other situations
// we use this logic.
func pluralize(count int, singular string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %ss", count, singular)
}

// formatNameAndId returns a display string with name and optional ID.
func formatNameAndId(name, id string) string {
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

// parseDocumentHeader extracts the kind, human-readable name, and ID from
// raw YAML bytes. The name and ID locations vary by asset kind:
//
//	Kind            | Name source          | ID source
//	----------------|----------------------|----------------------------
//	Dashboard       | spec.display.name    | metadata.name (the UUID)
//	CheckRule       | top-level name       | top-level id
//	View            | metadata.name        | metadata.labels["dash0.com/id"]
//	SyntheticCheck  | metadata.name        | metadata.labels["dash0.com/id"]
//	PrometheusRule  | metadata.name        | metadata.labels["dash0.com/id"]
func parseDocumentHeader(data []byte) (kind, name, id string, err error) {
	var raw map[string]any
	if err := sigsyaml.Unmarshal(data, &raw); err != nil {
		return "", "", "", fmt.Errorf("failed to decode document: %w", err)
	}

	kind = asset.DetectKind(data)

	switch normalizeKind(kind) {
	case "dashboard":
		name = yamlStringFromMap(raw, "spec", "display", "name")
		id = yamlStringFromMap(raw, "metadata", "name")

	case "checkrule":
		name, _ = raw["name"].(string)
		id, _ = raw["id"].(string)

	case "view", "syntheticcheck", "prometheusrule":
		name = yamlStringFromMap(raw, "metadata", "name")
		id = yamlStringFromMap(raw, "metadata", "labels", "dash0.com/id")

	default:
		name = yamlStringFromMap(raw, "metadata", "name")
	}

	return kind, name, id, nil
}

// yamlString traverses a nested map[string]any by the given keys and returns
// the leaf value as a string, or "" if any key is missing or the value is not
// a string.
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

// readMultiDocumentYAML splits a YAML stream into individual documents.
// This is the only place that requires gopkg.in/yaml.v3 directly —
// sigs.k8s.io/yaml doesn't provide a streaming decoder for multi-document YAML.
func readMultiDocumentYAML(filePath string, stdin io.Reader) ([]assetDocument, error) {
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

	var documents []assetDocument
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

		kind, name, id, err := parseDocumentHeader(buf.Bytes())
		if err != nil {
			return nil, err
		}

		documents = append(documents, assetDocument{
			kind:     kind,
			name:     name,
			id:       id,
			raw:      buf.Bytes(),
			docIndex: len(documents) + 1,
		})
	}

	// Handle single-document files without YAML document markers
	if len(documents) == 0 && len(data) > 0 {
		kind, name, id, _ := parseDocumentHeader(data)
		if kind != "" {
			documents = append(documents, assetDocument{
				kind:     kind,
				name:     name,
				id:       id,
				raw:      data,
				docIndex: 1,
			})
		}
	}

	// Set docCount on all documents
	for i := range documents {
		documents[i].docCount = len(documents)
	}

	return documents, nil
}

// discoverFiles recursively finds all .yaml/.yml files under dirPath,
// skipping hidden entries (names starting with '.').
// Returns paths relative to dirPath, sorted lexicographically.
func discoverFiles(dirPath string) ([]string, error) {
	var files []string
	hasNestedDirs := false
	err := filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		// Skip hidden files and directories
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if path != dirPath {
				hasNestedDirs = true
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".yaml" || ext == ".yml" {
			rel, err := filepath.Rel(dirPath, path)
			if err != nil {
				return err
			}
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan directory: %w", err)
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

// readDirectory reads all YAML files from a directory recursively.
func readDirectory(dirPath string) ([]assetDocument, error) {
	files, err := discoverFiles(dirPath)
	if err != nil {
		return nil, err
	}

	var allDocs []assetDocument
	for _, relPath := range files {
		fullPath := filepath.Join(dirPath, relPath)
		docs, err := readMultiDocumentYAML(fullPath, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", relPath, err)
		}
		for i := range docs {
			docs[i].filePath = relPath
		}
		allDocs = append(allDocs, docs...)
	}

	return allDocs, nil
}

func isValidKind(kind string) bool {
	switch normalizeKind(kind) {
	case "dashboard", "checkrule", "syntheticcheck", "view", "prometheusrule":
		return true
	default:
		return false
	}
}

func normalizeKind(kind string) string {
	// Normalize common variations
	k := strings.ToLower(strings.ReplaceAll(kind, "-", ""))
	k = strings.ReplaceAll(k, "_", "")
	k = strings.TrimPrefix(k, "dash0")
	return k
}

func applyDocument(ctx context.Context, apiClient dash0api.Client, doc assetDocument, dataset string) ([]applyResult, error) {
	datasetPtr := client.DatasetPtr(dataset)

	switch normalizeKind(doc.kind) {
	case "dashboard":
		var dashboard dash0api.DashboardDefinition
		if err := sigsyaml.Unmarshal(doc.raw, &dashboard); err != nil {
			return nil, fmt.Errorf("failed to parse Dashboard: %w", err)
		}
		result, err := asset.ImportDashboard(ctx, apiClient, &dashboard, datasetPtr)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{
				AssetType: "dashboard",
				AssetName: asset.ExtractDashboardDisplayName(&dashboard),
			})
		}
		return []applyResult{{kind: doc.kind, name: result.Name, id: result.ID, action: applyAction(result.Action)}}, nil

	case "checkrule":
		var rule dash0api.PrometheusAlertRule
		if err := sigsyaml.Unmarshal(doc.raw, &rule); err != nil {
			return nil, fmt.Errorf("failed to parse CheckRule: %w", err)
		}
		result, err := asset.ImportCheckRule(ctx, apiClient, &rule, datasetPtr)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{
				AssetType: "check rule",
				AssetName: rule.Name,
			})
		}
		return []applyResult{{kind: doc.kind, name: result.Name, id: result.ID, action: applyAction(result.Action)}}, nil

	case "prometheusrule":
		var promRule asset.PrometheusRule
		if err := sigsyaml.Unmarshal(doc.raw, &promRule); err != nil {
			return nil, fmt.Errorf("failed to parse PrometheusRule: %w", err)
		}
		return applyPrometheusRule(ctx, apiClient, &promRule, datasetPtr)

	case "syntheticcheck":
		var check dash0api.SyntheticCheckDefinition
		if err := sigsyaml.Unmarshal(doc.raw, &check); err != nil {
			return nil, fmt.Errorf("failed to parse SyntheticCheck: %w", err)
		}
		result, err := asset.ImportSyntheticCheck(ctx, apiClient, &check, datasetPtr)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{
				AssetType: "synthetic check",
				AssetName: check.Metadata.Name,
			})
		}
		return []applyResult{{kind: doc.kind, name: result.Name, id: result.ID, action: applyAction(result.Action)}}, nil

	case "view":
		var view dash0api.ViewDefinition
		if err := sigsyaml.Unmarshal(doc.raw, &view); err != nil {
			return nil, fmt.Errorf("failed to parse View: %w", err)
		}
		result, err := asset.ImportView(ctx, apiClient, &view, datasetPtr)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{
				AssetType: "view",
				AssetName: view.Metadata.Name,
			})
		}
		return []applyResult{{kind: doc.kind, name: result.Name, id: result.ID, action: applyAction(result.Action)}}, nil

	default:
		return nil, fmt.Errorf("unsupported kind: %s", doc.kind)
	}
}

// applyPrometheusRule extracts rules from a PrometheusRule CRD and applies each as a CheckRule.
// Returns one applyResult per rule, each with its own action. On partial failure,
// returns the successfully applied results along with the error.
func applyPrometheusRule(ctx context.Context, apiClient dash0api.Client, promRule *asset.PrometheusRule, datasetPtr *string) ([]applyResult, error) {
	importResults, err := asset.ImportPrometheusRule(ctx, apiClient, promRule, datasetPtr)

	var results []applyResult
	for _, r := range importResults {
		results = append(results, applyResult{
			kind:   "CheckRule",
			name:   r.Name,
			id:     r.ID,
			action: applyAction(r.Action),
		})
	}

	if err != nil {
		return results, client.HandleAPIError(err, client.ErrorContext{
			AssetType: "check rule",
		})
	}

	return results, nil
}
