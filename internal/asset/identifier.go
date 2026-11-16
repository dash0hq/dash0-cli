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
	ID       string `json:"id"`
	Metadata struct {
		// Values are not all strings: sampling rules nest custom labels
		// under labels.custom, so only string values are read.
		Labels          map[string]any `json:"labels"`
		Dash0Extensions struct {
			ID string `json:"id"`
		} `json:"dash0Extensions"`
	} `json:"metadata"`
}

// ExtractIdentifier returns the user-defined identifier a document is upserted
// by, or "" when the document carries none. The field location varies by kind,
// per the "Asset identifiers and idempotent upsert" table in the command
// reference:
//
//   - Dashboard: metadata.dash0Extensions.id
//   - CheckRule: top-level id
//   - Dash0NotificationChannel, Dash0TimeSeriesAggregation: dash0.com/origin
//   - Dash0Dataset: spec.slug, then dash0.com/id
//   - SLO, Dash0SpamFilter, Dash0Team, Dash0Skill, Dash0GenerativeAiEvaluation,
//     Dash0SignalToMetrics, Dash0Sampling:
//     dash0.com/origin, then dash0.com/id
//   - everything else (PersesDashboard, PrometheusRule, SyntheticCheck, View,
//     Dash0AwsMetricScrapeConfig):
//     dash0.com/id
//
// Each kind reads only the field its Import helper actually upserts by. A
// fallback onto some other field would return an identifier no live asset can
// match, which --since would then "delete" to a 404 and report as already
// deleted -- returning "" instead routes the document to the NoIdentifier
// hard-fail. Only SLOs, teams, spam filters, Agent0 skills, evals,
// signal-to-metrics rules, and sampling rules genuinely accept either field.
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

	origin := probe.label("dash0.com/origin")

	switch NormalizeKind(probe.Kind) {
	case "dashboard":
		return probe.Metadata.Dash0Extensions.ID, nil
	case "checkrule":
		return probe.ID, nil
	case "notificationchannel", "timeseriesaggregation":
		return origin, nil
	case "dataset":
		// The client's own helper, so spec.slug wins over the label exactly as it
		// does on create, update, and apply.
		var dataset dash0api.DatasetDefinition
		if err := dash0yaml.Unmarshal(data, &dataset); err != nil {
			return "", fmt.Errorf("failed to extract identifier: %w", err)
		}
		return dash0api.GetDatasetID(&dataset), nil
	case "slo", "spamfilter", "team", "skill", "generativeaievaluation", "signaltometrics", "sampling":
		if origin != "" {
			return origin, nil
		}
		return probe.label("dash0.com/id"), nil
	default:
		return probe.label("dash0.com/id"), nil
	}
}

// label returns the string value of a metadata label, or "" when it is absent
// or not a string.
func (p identifierProbe) label(key string) string {
	s, _ := p.Metadata.Labels[key].(string)
	return s
}
