package asset

import (
	"fmt"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"gopkg.in/yaml.v3"
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

// PrometheusRuleHasRecordingRule reports whether a PrometheusRule CRD
// document has at least one recording rule (a `record:` entry). Returns
// false for a document that isn't a PrometheusRule CRD at all.
//
// --since uses this as a coarse presence/absence signal to detect a CRD
// that survives (its own identifier is still present in both snapshots) but
// whose recording-rule role disappeared entirely -- e.g. its last `record:`
// entry was removed while an `alert:` entry keeps the CRD's identifier
// alive. Unlike alerting rules, which become one check rule per alert (and
// so can be tracked and deleted individually by name), Dash0 models a CRD's
// recording rules as a single server-side resource, so there is no
// per-record identity to track -- only whether the role exists at all.
func PrometheusRuleHasRecordingRule(data []byte) (bool, error) {
	kind, err := dash0yaml.DetectKind(data)
	if err != nil {
		return false, err
	}
	if !strings.EqualFold(kind, "PrometheusRule") {
		return false, nil
	}

	var crd dash0api.RecordingRule
	if err := sigsyaml.Unmarshal(data, &crd); err != nil {
		return false, fmt.Errorf("failed to parse PrometheusRule: %w", err)
	}
	return RecordingOnlyPrometheusRule(&crd) != nil, nil
}

// composePrometheusRuleNames rewrites the name of each check rule produced from
// a PrometheusRule CRD to "<group name> - <alert name>". It is a no-op for
// plain CheckRule documents.
//
// The rule order here must match ParseAsPrometheusAlertRules: iterate groups in
// document order, then rules in document order, skipping recording rules (those
// without an `alert`). That alignment lets the names zip onto the returned
// rules by index.
//
// For a CRD with more than one alerting rule, this also rewrites each rule's
// Id: the SDK conversion (ParseAsPrometheusAlertRules) stamps the CRD's own
// shared dash0.com/id onto every alert identically, since that's the only id
// a CRD carries. A single-alert CRD is fine with that -- the shared id
// unambiguously names its one check rule -- but for 2+ alerts it means every
// alert upserts (PUT, create-or-*replace*) to the exact same id, so each
// apply silently overwrites whatever the previous alert in the same run just
// wrote: only the last alert in document order ends up with a real check
// rule server-side, even though the CLI reports success for all of them.
// Deriving a distinct id per alert -- the shared id plus a slug of the
// alert's own composed name -- gives each one its own upsert target. The
// derivation is stable across repeated applies of the same content (the
// dash0.com/id label doesn't change, and an alert's composed name doesn't
// change unless the alert itself is renamed) and across reordering the
// CRD's rules (it depends on the name, not position), so upsert idempotency
// holds the same way it already does for a single-alert CRD.
//
// Migration note: re-applying an existing multi-alert CRD under this fix
// creates a fresh check rule per alert at each alert's derived id; the CRD's
// literal shared dash0.com/id, which used to hold whichever alert applied
// last under the old behavior, is not touched by the new per-alert ids and
// becomes an orphaned duplicate -- delete it by hand once the new per-alert
// check rules look correct.
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
	multiAlert := len(names) > 1
	for i, name := range names {
		if i >= len(rules) {
			return nil
		}
		composedName := name.CheckRuleName()
		rules[i].Name = composedName
		if multiAlert && rules[i].Id != nil && *rules[i].Id != "" {
			derived := deriveAlertCheckRuleID(*rules[i].Id, composedName)
			rules[i].Id = &derived
		}
	}
	return nil
}

// deriveAlertCheckRuleID derives a per-alert check-rule identifier for a
// PrometheusRule CRD with more than one alerting rule, from the CRD's own
// shared id and the alert's composed name. See composePrometheusRuleNames'
// doc comment for the full rationale.
func deriveAlertCheckRuleID(sharedID, composedName string) string {
	return sharedID + "--" + slugify(composedName)
}

// slugify lowercases s and replaces every run of characters that aren't
// lowercase letters or digits with a single hyphen, trimming any leading or
// trailing hyphen. Used to fold a human-readable composed check-rule name
// (e.g. "rule-group - DiskFull") into a predictable, URL-safe identifier
// fragment (e.g. "rule-group-diskfull").
func slugify(s string) string {
	var b strings.Builder
	lastWasHyphen := true // avoid a leading hyphen
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastWasHyphen = false
			continue
		}
		if !lastWasHyphen {
			b.WriteByte('-')
			lastWasHyphen = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
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
