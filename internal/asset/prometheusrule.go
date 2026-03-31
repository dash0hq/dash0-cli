package asset

import (
	dash0api "github.com/dash0hq/dash0-api-client-go"
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
	Rules    []PrometheusAlertingRule `yaml:"rules" json:"rules"`
}

// PrometheusAlertingRule represents an individual alerting rule within a group
type PrometheusAlertingRule struct {
	Alert       string            `yaml:"alert,omitempty" json:"alert,omitempty"`
	Expr        string            `yaml:"expr" json:"expr"`
	For         string            `yaml:"for,omitempty" json:"for,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`
}

// ExtractPrometheusRuleID extracts the ID from a PrometheusRule definition.
func ExtractPrometheusRuleID(rule *PrometheusRule) string {
	if rule.Metadata.Labels != nil {
		return rule.Metadata.Labels["dash0.com/id"]
	}
	return ""
}

// ExtractPrometheusRuleName extracts the name from a PrometheusRule definition.
func ExtractPrometheusRuleName(rule *PrometheusRule) string {
	return rule.Metadata.Name
}

// ConvertToCheckRule converts a Prometheus alerting rule to a Dash0 CheckRule
func ConvertToCheckRule(rule *PrometheusAlertingRule, groupInterval string, ruleID string) *dash0api.PrometheusAlertRule {
	checkRule := &dash0api.PrometheusAlertRule{
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

	// Set 'for' duration
	if rule.For != "" {
		forDuration := dash0api.Duration(rule.For)
		checkRule.For = &forDuration
	}

	// Use group interval if specified
	if groupInterval != "" {
		interval := dash0api.Duration(groupInterval)
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
