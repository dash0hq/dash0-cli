package asset

import (
	"fmt"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
)

// PrometheusAlertName identifies a single alerting rule inside a PrometheusRule
// CRD by the group it lives in and its own alert name.
//
// Dash0 has no per-alert server-side id — a CRD's alerting rules share the
// CRD's own identifier — so this pair is the only stable handle on one alert
// within a CRD. --since uses it to resolve a removed alert by its composed
// check-rule name.
type PrometheusAlertName struct {
	GroupName string
	AlertName string
}

// CheckRuleName composes the check-rule name Dash0 gives an alerting rule
// converted from a PrometheusRule CRD: "<group name> - <alert name>", matching
// the Dash0 Kubernetes operator and the Terraform provider.
func (p PrometheusAlertName) CheckRuleName() string {
	return fmt.Sprintf("%s - %s", p.GroupName, p.AlertName)
}

type identifierProbe struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name string `json:"name"`
		// Values are not all strings: sampling rules nest custom labels
		// under labels.custom, so only string values are read.
		Labels map[string]any `json:"labels"`
	} `json:"metadata"`
}

// ExtractIdentifier returns the user-defined identifier a document is upserted
// by, or "" when the document carries none. The field location varies by kind,
// per the "Asset identifiers and idempotent upsert" table in the command
// reference:
//
//   - Dashboard, EphemeralDashboard, PersesDashboard: dash0.com/id, then
//     dash0.com/origin, then metadata.dash0Extensions.id (see DashboardIdentity)
//   - CheckRule: top-level id, then labels["dash0.com/origin"]
//   - every other routed kind: dash0.com/id, then dash0.com/origin
//   - Dash0Dataset: spec.slug, then dash0.com/id
//   - Dash0AwsMetricScrapeConfig: dash0.com/id
//
// The kinds RouteDocument routes read the id first, as apply does, so --since
// deletes where apply wrote. For the other kinds, a fallback onto another
// field would return an identifier no live asset matches, so they return "".
// --since then keys a routed document by its inferred origin
// (ExtractInferredKeys), and fails for the rest.
func ExtractIdentifier(data []byte) (string, error) {
	var probe identifierProbe
	if err := dash0yaml.Unmarshal(data, &probe); err != nil {
		return "", fmt.Errorf("failed to extract identifier: %w", err)
	}
	// Labels decode leniently for labels.custom, so an identifier label that
	// is not a string must fail here instead of reading as "no identifier".
	for _, key := range []string{"dash0.com/id", "dash0.com/origin"} {
		if value, ok := probe.Metadata.Labels[key]; ok && value != nil {
			if _, isString := value.(string); !isString {
				return "", fmt.Errorf("failed to extract identifier: metadata.labels[%q] must be a string, not %v; quote it in YAML", key, value)
			}
		}
	}

	kind := NormalizeKind(probe.Kind)
	identity, _, routed, err := parseRoutedIdentity(kind, data, nil)
	if err != nil {
		return "", fmt.Errorf("failed to extract identifier: %w", err)
	}
	if routed {
		if identity.ID != "" {
			return identity.ID, nil
		}
		return identity.Origin, nil
	}

	switch kind {
	case "dataset":
		// The client's own helper, so spec.slug wins over the label exactly as it
		// does on create, update, and apply.
		var dataset dash0api.DatasetDefinition
		if err := dash0yaml.Unmarshal(data, &dataset); err != nil {
			return "", fmt.Errorf("failed to extract identifier: %w", err)
		}
		return dash0api.GetDatasetID(&dataset), nil
	default:
		return probe.label("dash0.com/id"), nil
	}
}

// ExtractDeleteFallbacks returns the other keys --since may have to delete a
// routed document at. alias is the origin of a document that also has an id:
// apply writes at the id when it exists, else at the origin, so --since
// deletes in that order. inferredOriginName is the metadata.name of a document
// without an origin: apply writes it at its inferred origin when it has no id
// or the id is not found, so --since must delete it there. Both are "" for
// datasets and AWS metric scrape configs, which --since deletes at their id.
func ExtractDeleteFallbacks(data []byte) (alias, inferredOriginName string, err error) {
	kind, err := dash0yaml.DetectKind(data)
	if err != nil {
		return "", "", err
	}
	identity, name, _, err := parseRoutedIdentity(kind, data, nil)
	if err != nil {
		return "", "", err
	}
	if identity.CreatesAtID && identity.ID != "" {
		// apply writes at the id whether or not it was found.
		return "", "", nil
	}
	if identity.Origin == "" {
		inferredOriginName = name
	}
	return originAlias(identity.ID, identity.Origin), inferredOriginName, nil
}

// label returns the string value of a metadata label, or "" when it is absent
// or not a string.
func (p identifierProbe) label(key string) string {
	s, _ := p.Metadata.Labels[key].(string)
	return s
}

// originAlias returns the origin --since falls back to when the id is not
// found, or "" when the document has no id, so its identifier is the origin.
func originAlias(id, origin string) string {
	if id == "" || origin == id {
		return ""
	}
	return origin
}

// InferredKey is an asset that apply writes at its inferred origin, because
// its document has no origin.
type InferredKey struct {
	// Kind is the normalized kind of the asset: "checkrule" and
	// "recordingrule" for the assets of a PrometheusRule.
	Kind   string
	Origin string
	// Name is the display name of an asset of a PrometheusRule, else "".
	Name string
}

// ExtractInferredKeys returns the assets that a document without an origin
// writes at their inferred origins, the same ones apply computes for the
// dataset. A document with an id writes there only when the id is not found.
// It returns false for a document with an origin, and for datasets and AWS
// metric scrape configs, whose id is required.
func ExtractInferredKeys(data []byte, dataset *string) ([]InferredKey, bool, error) {
	kind, err := dash0yaml.DetectKind(data)
	if err != nil {
		return nil, false, err
	}
	kind = NormalizeKind(kind)
	identity, name, routed, err := parseRoutedIdentity(kind, data, dataset)
	if err != nil || !routed || identity.Origin != "" {
		return nil, false, err
	}
	if kind != "prometheusrule" {
		return []InferredKey{{Kind: kind, Origin: identity.Inferred}}, true, nil
	}

	var alerts []PrometheusAlertName
	if identity.ID == "" {
		// An alert with an id is created at that id when it is not found.
		alerts, err = ExtractPrometheusAlertNames(data)
		if err != nil {
			return nil, false, err
		}
	}
	var keys []InferredKey
	for _, alert := range alerts {
		origin := InferKindOrigin("prometheusrule", PrometheusAlertInferredName(name, alert.CheckRuleName()), dataset)
		keys = append(keys, InferredKey{Kind: "checkrule", Origin: origin, Name: alert.CheckRuleName()})
	}
	hasRecordingRule, err := PrometheusRuleHasRecordingRule(data)
	if err != nil {
		return nil, false, err
	}
	if hasRecordingRule {
		keys = append(keys, InferredKey{Kind: "recordingrule", Origin: InferKindOrigin("recordingrule", name, dataset), Name: name})
	}
	return keys, true, nil
}
