package apply

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	dash0 "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/client"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
		Use:   "apply -f <file>",
		Short: "Apply asset definitions from a file",
		Long: `Apply asset definitions from a YAML or JSON file.
The file may contain multiple documents separated by "---".
Each document must have a "kind" field specifying the asset type.
Use '-f -' to read from stdin.

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

  # Apply from stdin
  cat assets.yaml | dash0 apply -f -

  # Validate without applying
  dash0 apply -f assets.yaml --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.File == "" {
				return fmt.Errorf("file is required; use -f to specify the file (use '-' for stdin)")
			}
			return runApply(cmd.Context(), &flags)
		},
	}

	cmd.Flags().StringVarP(&flags.File, "file", "f", "", "Path to the file containing asset definitions (use '-' for stdin)")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false, "Validate the file without applying changes")
	cmd.Flags().StringVar(&flags.ApiUrl, "api-url", "", "API URL for the Dash0 API (overrides active profile)")
	cmd.Flags().StringVar(&flags.AuthToken, "auth-token", "", "Auth token for the Dash0 API (overrides active profile)")
	cmd.Flags().StringVarP(&flags.Dataset, "dataset", "d", "", "Dataset to operate on")

	return cmd
}

// assetDocument represents a parsed YAML document with its kind
type assetDocument struct {
	Kind string `yaml:"kind"`
	raw  []byte
}

// applyAction indicates whether an asset was created or updated
type applyAction string

const (
	actionCreated applyAction = "created"
	actionUpdated applyAction = "updated"
)

// PrometheusRule represents the Prometheus Operator PrometheusRule CRD
type PrometheusRule struct {
	APIVersion string                 `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                 `yaml:"kind" json:"kind"`
	Metadata   PrometheusRuleMetadata `yaml:"metadata" json:"metadata"`
	Spec       PrometheusRuleSpec     `yaml:"spec" json:"spec"`
}

// PrometheusRuleMetadata contains metadata for a PrometheusRule
type PrometheusRuleMetadata struct {
	Name        string            `yaml:"name,omitempty" json:"name,omitempty"`
	Namespace   string            `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`
}

// PrometheusRuleSpec contains the spec for a PrometheusRule
type PrometheusRuleSpec struct {
	Groups []PrometheusRuleGroup `yaml:"groups" json:"groups"`
}

// PrometheusRuleGroup represents a group of alerting rules
type PrometheusRuleGroup struct {
	Name     string            `yaml:"name" json:"name"`
	Interval string            `yaml:"interval,omitempty" json:"interval,omitempty"`
	Rules    []PrometheusRule_ `yaml:"rules" json:"rules"`
}

// PrometheusRule_ represents an individual alerting rule within a group
type PrometheusRule_ struct {
	Alert       string            `yaml:"alert,omitempty" json:"alert,omitempty"`
	Expr        string            `yaml:"expr" json:"expr"`
	For         string            `yaml:"for,omitempty" json:"for,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`
}

func runApply(ctx context.Context, flags *applyFlags) error {
	// Read and parse the file or stdin
	documents, err := readMultiDocumentYAML(flags.File, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	if len(documents) == 0 {
		return fmt.Errorf("no documents found in file")
	}

	// Validate all documents first
	for i, doc := range documents {
		if doc.Kind == "" {
			return fmt.Errorf("document %d: missing 'kind' field", i+1)
		}
		if !isValidKind(doc.Kind) {
			return fmt.Errorf("document %d: unsupported kind %q (supported: Dashboard, CheckRule, PrometheusRule, SyntheticCheck, View)", i+1, doc.Kind)
		}
	}

	if flags.DryRun {
		fmt.Printf("Dry run: %d document(s) validated successfully\n", len(documents))
		for i, doc := range documents {
			fmt.Printf("  %d. %s\n", i+1, doc.Kind)
		}
		return nil
	}

	// Create API client
	apiClient, err := client.NewClientFromContext(ctx, flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	// Apply each document
	var applied []string
	for i, doc := range documents {
		name, action, err := applyDocument(ctx, apiClient, doc, flags.Dataset)
		if err != nil {
			// Report what was applied before the error
			if len(applied) > 0 {
				fmt.Println("Applied before error:")
				for _, a := range applied {
					fmt.Printf("  - %s\n", a)
				}
			}
			return fmt.Errorf("document %d (%s): %w", i+1, doc.Kind, err)
		}
		applied = append(applied, fmt.Sprintf("%s %q", doc.Kind, name))
		fmt.Printf("%s %q %s\n", doc.Kind, name, action)
	}

	return nil
}

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
			return nil, fmt.Errorf("failed to parse YAML: %w", err)
		}

		// Skip empty documents
		if node.Kind == 0 {
			continue
		}

		// Extract the kind field
		var kindDoc struct {
			Kind string `yaml:"kind"`
		}
		if err := node.Decode(&kindDoc); err != nil {
			return nil, fmt.Errorf("failed to decode document: %w", err)
		}

		// Re-encode the node to get the raw bytes for this document
		var buf bytes.Buffer
		encoder := yaml.NewEncoder(&buf)
		encoder.SetIndent(2)
		if err := encoder.Encode(&node); err != nil {
			return nil, fmt.Errorf("failed to re-encode document: %w", err)
		}
		encoder.Close()

		documents = append(documents, assetDocument{
			Kind: kindDoc.Kind,
			raw:  buf.Bytes(),
		})
	}

	// Handle single-document files without YAML document markers
	if len(documents) == 0 && len(data) > 0 {
		// Try parsing as a single document
		var kindDoc struct {
			Kind string `yaml:"kind"`
		}
		if err := yaml.Unmarshal(data, &kindDoc); err == nil && kindDoc.Kind != "" {
			documents = append(documents, assetDocument{
				Kind: kindDoc.Kind,
				raw:  data,
			})
		}
	}

	return documents, nil
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

func applyDocument(ctx context.Context, apiClient dash0.Client, doc assetDocument, dataset string) (string, applyAction, error) {
	datasetPtr := client.DatasetPtr(dataset)

	switch normalizeKind(doc.Kind) {
	case "dashboard":
		var dashboard dash0.DashboardDefinition
		if err := yaml.Unmarshal(doc.raw, &dashboard); err != nil {
			return "", "", fmt.Errorf("failed to parse Dashboard: %w", err)
		}
		// Set origin to dash0-cli (using Id field since Origin is deprecated)
		if dashboard.Metadata.Dash0Extensions == nil {
			dashboard.Metadata.Dash0Extensions = &dash0.DashboardMetadataExtensions{}
		}
		origin := "dash0-cli"
		dashboard.Metadata.Dash0Extensions.Id = &origin

		// Check if dashboard exists
		action := actionCreated
		if dashboard.Metadata.Dash0Extensions.Id != nil && *dashboard.Metadata.Dash0Extensions.Id != "" {
			_, err := apiClient.GetDashboard(ctx, *dashboard.Metadata.Dash0Extensions.Id, datasetPtr)
			if err == nil {
				action = actionUpdated
			}
		}

		result, err := apiClient.ImportDashboard(ctx, &dashboard, datasetPtr)
		if err != nil {
			return "", "", client.HandleAPIError(err, client.ErrorContext{
				AssetType: "dashboard",
				AssetName: extractDashboardDisplayName(&dashboard),
			})
		}
		return result.Metadata.Name, action, nil

	case "checkrule":
		var rule dash0.PrometheusAlertRule
		if err := yaml.Unmarshal(doc.raw, &rule); err != nil {
			return "", "", fmt.Errorf("failed to parse CheckRule: %w", err)
		}
		// Set origin to dash0-cli
		if rule.Labels == nil {
			labels := make(map[string]string)
			rule.Labels = &labels
		}
		(*rule.Labels)["dash0.com/origin"] = "dash0-cli"

		// Check if check rule exists
		action := actionCreated
		if rule.Id != nil && *rule.Id != "" {
			_, err := apiClient.GetCheckRule(ctx, *rule.Id, datasetPtr)
			if err == nil {
				action = actionUpdated
			}
		}

		result, err := apiClient.ImportCheckRule(ctx, &rule, datasetPtr)
		if err != nil {
			return "", "", client.HandleAPIError(err, client.ErrorContext{
				AssetType: "check rule",
				AssetName: rule.Name,
			})
		}
		return result.Name, action, nil

	case "prometheusrule":
		var promRule PrometheusRule
		if err := yaml.Unmarshal(doc.raw, &promRule); err != nil {
			return "", "", fmt.Errorf("failed to parse PrometheusRule: %w", err)
		}
		name, action, err := applyPrometheusRule(ctx, apiClient, &promRule, datasetPtr)
		return name, action, err

	case "syntheticcheck":
		var check dash0.SyntheticCheckDefinition
		if err := yaml.Unmarshal(doc.raw, &check); err != nil {
			return "", "", fmt.Errorf("failed to parse SyntheticCheck: %w", err)
		}
		// Set origin to dash0-cli
		if check.Metadata.Labels == nil {
			check.Metadata.Labels = &dash0.SyntheticCheckLabels{}
		}
		origin := "dash0-cli"
		check.Metadata.Labels.Dash0Comorigin = &origin

		// Check if synthetic check exists
		action := actionCreated
		if check.Metadata.Labels.Dash0Comorigin != nil && *check.Metadata.Labels.Dash0Comorigin != "" {
			_, err := apiClient.GetSyntheticCheck(ctx, *check.Metadata.Labels.Dash0Comorigin, datasetPtr)
			if err == nil {
				action = actionUpdated
			}
		}

		result, err := apiClient.ImportSyntheticCheck(ctx, &check, datasetPtr)
		if err != nil {
			return "", "", client.HandleAPIError(err, client.ErrorContext{
				AssetType: "synthetic check",
				AssetName: check.Metadata.Name,
			})
		}
		return result.Metadata.Name, action, nil

	case "view":
		var view dash0.ViewDefinition
		if err := yaml.Unmarshal(doc.raw, &view); err != nil {
			return "", "", fmt.Errorf("failed to parse View: %w", err)
		}
		// Set origin to dash0-cli
		if view.Metadata.Labels == nil {
			view.Metadata.Labels = &dash0.ViewLabels{}
		}
		origin := "dash0-cli"
		view.Metadata.Labels.Dash0Comorigin = &origin

		// Check if view exists
		action := actionCreated
		if view.Metadata.Labels.Dash0Comorigin != nil && *view.Metadata.Labels.Dash0Comorigin != "" {
			_, err := apiClient.GetView(ctx, *view.Metadata.Labels.Dash0Comorigin, datasetPtr)
			if err == nil {
				action = actionUpdated
			}
		}

		result, err := apiClient.ImportView(ctx, &view, datasetPtr)
		if err != nil {
			return "", "", client.HandleAPIError(err, client.ErrorContext{
				AssetType: "view",
				AssetName: view.Metadata.Name,
			})
		}
		return result.Metadata.Name, action, nil

	default:
		return "", "", fmt.Errorf("unsupported kind: %s", doc.Kind)
	}
}

// applyPrometheusRule extracts rules from a PrometheusRule CRD and applies each as a CheckRule
func applyPrometheusRule(ctx context.Context, apiClient dash0.Client, promRule *PrometheusRule, datasetPtr *string) (string, applyAction, error) {
	var appliedNames []string
	hasCreated := false
	hasUpdated := false

	// Extract ID from metadata labels for upsert semantics
	var ruleID string
	if promRule.Metadata.Labels != nil {
		ruleID = promRule.Metadata.Labels["dash0.com/id"]
	}

	for _, group := range promRule.Spec.Groups {
		for _, rule := range group.Rules {
			// Skip recording rules (those without alert name)
			if rule.Alert == "" {
				continue
			}

			// Convert PrometheusRule rule to Dash0 CheckRule
			checkRule := convertToCheckRule(&rule, group.Interval, ruleID)

			// Check if this rule exists to determine action
			if checkRule.Id != nil && *checkRule.Id != "" {
				_, err := apiClient.GetCheckRule(ctx, *checkRule.Id, datasetPtr)
				if err == nil {
					hasUpdated = true
				} else {
					hasCreated = true
				}
			} else {
				hasCreated = true
			}

			result, err := apiClient.ImportCheckRule(ctx, checkRule, datasetPtr)
			if err != nil {
				if len(appliedNames) > 0 {
					return strings.Join(appliedNames, ", "), "", fmt.Errorf("applied %v, then failed on %q: %w", appliedNames, rule.Alert, client.HandleAPIError(err, client.ErrorContext{
						AssetType: "check rule",
						AssetName: rule.Alert,
					}))
				}
				return "", "", client.HandleAPIError(err, client.ErrorContext{
					AssetType: "check rule",
					AssetName: rule.Alert,
				})
			}
			appliedNames = append(appliedNames, result.Name)
		}
	}

	if len(appliedNames) == 0 {
		return "", "", fmt.Errorf("no alerting rules found in PrometheusRule (recording rules are not supported)")
	}

	// Determine the action based on what happened
	var action applyAction
	if hasCreated && hasUpdated {
		action = "created/updated"
	} else if hasUpdated {
		action = actionUpdated
	} else {
		action = actionCreated
	}

	return strings.Join(appliedNames, ", "), action, nil
}

// convertToCheckRule converts a Prometheus alerting rule to a Dash0 CheckRule
func convertToCheckRule(rule *PrometheusRule_, groupInterval string, ruleID string) *dash0.PrometheusAlertRule {
	checkRule := &dash0.PrometheusAlertRule{
		Name:       rule.Alert,
		Expression: rule.Expr,
	}

	// Set ID for upsert semantics
	if ruleID != "" {
		checkRule.Id = &ruleID
	}

	// Set labels and annotations as pointers
	if len(rule.Labels) > 0 {
		checkRule.Labels = &rule.Labels
	}
	if len(rule.Annotations) > 0 {
		checkRule.Annotations = &rule.Annotations
	}

	// Set origin to dash0-cli
	if checkRule.Labels == nil {
		labels := make(map[string]string)
		checkRule.Labels = &labels
	}
	(*checkRule.Labels)["dash0.com/origin"] = "dash0-cli"

	// Set 'for' duration
	if rule.For != "" {
		forDuration := dash0.Duration(rule.For)
		checkRule.For = &forDuration
	}

	// Use group interval if specified
	if groupInterval != "" {
		interval := dash0.Duration(groupInterval)
		checkRule.Interval = &interval
	}

	// Extract summary and description from annotations if present
	if rule.Annotations != nil {
		if summary, ok := rule.Annotations["summary"]; ok {
			checkRule.Summary = &summary
		}
		if description, ok := rule.Annotations["description"]; ok {
			checkRule.Description = &description
		}
	}

	return checkRule
}

// extractDashboardDisplayName extracts the display name from a dashboard definition
func extractDashboardDisplayName(dashboard *dash0.DashboardDefinition) string {
	if dashboard == nil || dashboard.Spec == nil {
		return ""
	}

	display, ok := dashboard.Spec["display"].(map[string]interface{})
	if !ok {
		return ""
	}

	name, ok := display["name"].(string)
	if !ok {
		return ""
	}

	return name
}
