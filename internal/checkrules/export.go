package checkrules

import (
	"context"
	"fmt"

	dash0 "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/client"
	res "github.com/dash0hq/dash0-cli/internal/resource"
	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	var flags res.ExportFlags

	cmd := &cobra.Command{
		Use:   "export <id>",
		Short: "Export a check rule to a file",
		Long:  `Export a check rule definition to a YAML or JSON file`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(cmd.Context(), args[0], &flags)
		},
	}

	res.RegisterExportFlags(cmd, &flags)
	return cmd
}

func runExport(ctx context.Context, id string, flags *res.ExportFlags) error {
	apiClient, err := client.NewClient(flags.ApiUrl, flags.AuthToken)
	if err != nil {
		return err
	}

	rule, err := apiClient.GetCheckRule(ctx, id, client.DatasetPtr(flags.Dataset))
	if err != nil {
		return client.HandleAPIError(err)
	}

	// Ensure the rule ID is preserved for upsert semantics on apply
	if rule.Id == nil {
		rule.Id = &id
	}

	// Wrap in PrometheusRule CRD format
	promRule := convertToPrometheusRule(rule)

	if flags.File != "" {
		if err := res.WriteDefinitionFile(flags.File, promRule); err != nil {
			return fmt.Errorf("failed to write check rule to file: %w", err)
		}
		fmt.Printf("Check rule exported to %s\n", flags.File)
	} else {
		format := "yaml"
		if flags.Output == "json" {
			format = "json"
		}
		if err := res.WriteToStdout(format, promRule); err != nil {
			return fmt.Errorf("failed to write check rule: %w", err)
		}
	}

	return nil
}

// PrometheusRule represents the Prometheus Operator PrometheusRule CRD
type PrometheusRule struct {
	APIVersion string                 `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                 `yaml:"kind" json:"kind"`
	Metadata   PrometheusRuleMetadata `yaml:"metadata" json:"metadata"`
	Spec       PrometheusRuleSpec     `yaml:"spec" json:"spec"`
}

// PrometheusRuleMetadata contains metadata for a PrometheusRule
type PrometheusRuleMetadata struct {
	Name        string            `yaml:"name" json:"name"`
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
	Rules    []PrometheusAlert `yaml:"rules" json:"rules"`
}

// PrometheusAlert represents an individual alerting rule within a group
type PrometheusAlert struct {
	Alert       string            `yaml:"alert" json:"alert"`
	Expr        string            `yaml:"expr" json:"expr"`
	For         string            `yaml:"for,omitempty" json:"for,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`
}

// convertToPrometheusRule wraps a Dash0 CheckRule in a PrometheusRule CRD
func convertToPrometheusRule(rule *dash0.PrometheusAlertRule) *PrometheusRule {
	alert := PrometheusAlert{
		Alert: rule.Name,
		Expr:  rule.Expression,
	}

	// Set 'for' duration (omit if zero)
	if rule.For != nil && string(*rule.For) != "0s" {
		alert.For = string(*rule.For)
	}

	// Set labels
	if rule.Labels != nil {
		alert.Labels = *rule.Labels
	}

	// Build annotations from rule fields
	annotations := make(map[string]string)
	if rule.Annotations != nil {
		for k, v := range *rule.Annotations {
			annotations[k] = v
		}
	}
	if rule.Summary != nil && *rule.Summary != "" {
		annotations["summary"] = *rule.Summary
	}
	if rule.Description != nil && *rule.Description != "" {
		annotations["description"] = *rule.Description
	}
	if len(annotations) > 0 {
		alert.Annotations = annotations
	}

	// Determine group name from rule name
	groupName := "dash0-alerts"
	if rule.Name != "" {
		groupName = rule.Name
	}

	// Build interval from rule
	var interval string
	if rule.Interval != nil {
		interval = string(*rule.Interval)
	}

	// Build metadata labels to preserve Dash0-specific fields
	metadataLabels := make(map[string]string)
	if rule.Id != nil {
		metadataLabels["dash0.com/id"] = *rule.Id
	}
	if rule.Dataset != nil {
		metadataLabels["dash0.com/dataset"] = *rule.Dataset
	}

	return &PrometheusRule{
		APIVersion: "monitoring.coreos.com/v1",
		Kind:       "PrometheusRule",
		Metadata: PrometheusRuleMetadata{
			Name:   rule.Name,
			Labels: metadataLabels,
		},
		Spec: PrometheusRuleSpec{
			Groups: []PrometheusRuleGroup{
				{
					Name:     groupName,
					Interval: interval,
					Rules:    []PrometheusAlert{alert},
				},
			},
		},
	}
}
