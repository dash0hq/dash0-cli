package asset

import (
	"fmt"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"sigs.k8s.io/yaml"
)

// ParseDashboardInput detects whether data is a Dashboard or PersesDashboard
// CRD, unmarshals it, and returns a normalized DashboardDefinition ready for
// the API. PersesDashboard CRDs are converted via ConvertToDashboard.
func ParseDashboardInput(data []byte) (*dash0api.DashboardDefinition, error) {
	kind := strings.ToLower(DetectKind(data))
	if kind == "persesdashboard" {
		var perses PersesDashboard
		if err := yaml.Unmarshal(data, &perses); err != nil {
			return nil, fmt.Errorf("failed to parse PersesDashboard definition: %w", err)
		}
		return ConvertToDashboard(&perses), nil
	}

	var dashboard dash0api.DashboardDefinition
	if err := yaml.Unmarshal(data, &dashboard); err != nil {
		return nil, fmt.Errorf("failed to parse dashboard definition: %w", err)
	}
	return &dashboard, nil
}

// ParseCheckRuleInputs detects whether data is a CheckRule or PrometheusRule
// CRD, unmarshals it, and returns one or more normalized check rules ready for
// the API. A plain CheckRule returns a slice of length 1. A PrometheusRule CRD
// returns one entry per alerting rule (recording rules are skipped).
func ParseCheckRuleInputs(data []byte) ([]*dash0api.PrometheusAlertRule, error) {
	kind := strings.ToLower(DetectKind(data))
	if kind == "prometheusrule" {
		var promRule PrometheusRule
		if err := yaml.Unmarshal(data, &promRule); err != nil {
			return nil, fmt.Errorf("failed to parse PrometheusRule definition: %w", err)
		}

		ruleID := ExtractPrometheusRuleID(&promRule)
		var rules []*dash0api.PrometheusAlertRule
		for _, group := range promRule.Spec.Groups {
			for _, rule := range group.Rules {
				if rule.Alert == "" {
					continue
				}
				rules = append(rules, ConvertToCheckRule(&rule, group.Interval, ruleID))
			}
		}
		if len(rules) == 0 {
			return nil, fmt.Errorf("no alerting rules found in PrometheusRule (recording rules are not supported)")
		}
		return rules, nil
	}

	var rule dash0api.PrometheusAlertRule
	if err := yaml.Unmarshal(data, &rule); err != nil {
		return nil, fmt.Errorf("failed to parse check rule definition: %w", err)
	}
	return []*dash0api.PrometheusAlertRule{&rule}, nil
}
