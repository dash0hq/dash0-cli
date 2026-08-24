package asset

import (
	"fmt"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"gopkg.in/yaml.v3"
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

	names, err := ExtractPrometheusAlertNames(data)
	if err != nil {
		return err
	}
	for i, name := range names {
		if i >= len(rules) {
			return nil
		}
		rules[i].Name = name.CheckRuleName()
	}
	return nil
}

// ExtractPrometheusAlertNames parses a PrometheusRule CRD document and
// returns the (group name, alert name) pair for every alerting rule, in
// document order. Recording rules are skipped.
//
// Unlike a struct-typed unmarshal (sigs.k8s.io/yaml decoding into a *string
// field, as PrometheusRuleEndpoints and the rest of this file otherwise
// use), this reads each name's literal scalar value directly off the raw
// YAML node tree. sigs.k8s.io/yaml's YAML->JSON->struct path resolves an
// unquoted YAML 1.1/1.2 boolean literal (Y, N, yes, no, on, off, true,
// false, and case variants) to a real JSON boolean, then silently coerces
// that boolean into the destination string field as "true"/"false" instead
// of erroring — so an alert genuinely named e.g. "Y" would otherwise be
// corrupted to "true" everywhere its name is used (the composed check-rule
// name here, and --since's alert-tracking diff in internal/git, which calls
// this function via internal/git/snapshot.go instead of
// dash0-api-client-go/yaml's identically-named, differently-implemented
// ExtractPrometheusAlertNames for exactly this reason).
func ExtractPrometheusAlertNames(data []byte) ([]dash0yaml.PrometheusAlertName, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	groups := yamlMapValue(yamlMapValue(doc.Content[0], "spec"), "groups")
	if groups == nil {
		return nil, nil
	}

	var names []dash0yaml.PrometheusAlertName
	for _, group := range groups.Content {
		groupName := ""
		if n := yamlMapValue(group, "name"); n != nil {
			groupName = n.Value
		}
		rules := yamlMapValue(group, "rules")
		if rules == nil {
			continue
		}
		for _, rule := range rules.Content {
			alert := yamlMapValue(rule, "alert")
			if alert == nil || alert.Value == "" {
				continue
			}
			names = append(names, dash0yaml.PrometheusAlertName{GroupName: groupName, AlertName: alert.Value})
		}
	}
	return names, nil
}

// yamlMapValue returns the value node for key within a YAML mapping node, or
// nil if node is nil, not a mapping, or key isn't present.
func yamlMapValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
