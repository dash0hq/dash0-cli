package asset

import (
	"fmt"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	sigsyaml "sigs.k8s.io/yaml"
)

// ParseCheckRules parses a CheckRule or PrometheusRule CRD document into one or
// more check rules ready for import.
//
// For PrometheusRule CRDs the check-rule name is composed as
// "<group name> - <alert name>", matching the Dash0 Kubernetes operator and the
// Terraform provider. A plain CheckRule document keeps its name verbatim.
//
// The underlying SDK conversion (ParseAsPrometheusAlertRules) names each check
// rule after the alert only and discards the group name, so the group-name
// prefix is reapplied here from the raw CRD.
func ParseCheckRules(data []byte) ([]*dash0api.PrometheusAlertRule, error) {
	rules, err := dash0yaml.ParseAsPrometheusAlertRules(data)
	if err != nil {
		return nil, err
	}
	if err := composePrometheusRuleNames(data, rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// composePrometheusRuleNames rewrites the name of each check rule produced from
// a PrometheusRule CRD to "<group name> - <alert name>". It is a no-op for
// plain CheckRule documents.
//
// The rule order here must match ParseAsPrometheusAlertRules: iterate groups in
// document order, then rules in document order, skipping recording rules (those
// without an `alert`). That alignment lets the names zip onto the returned
// rules by index.
func composePrometheusRuleNames(data []byte, rules []*dash0api.PrometheusAlertRule) error {
	kind, err := dash0yaml.DetectKind(data)
	if err != nil {
		return err
	}
	if !strings.EqualFold(kind, "PrometheusRule") {
		return nil
	}

	var crd dash0api.RecordingRule
	if err := sigsyaml.Unmarshal(data, &crd); err != nil {
		return fmt.Errorf("failed to parse PrometheusRule: %w", err)
	}

	i := 0
	for _, group := range crd.Spec.Groups {
		for _, rule := range group.Rules {
			if rule.Alert == nil || *rule.Alert == "" {
				continue
			}
			if i >= len(rules) {
				return nil
			}
			rules[i].Name = fmt.Sprintf("%s - %s", group.Name, *rule.Alert)
			i++
		}
	}
	return nil
}
