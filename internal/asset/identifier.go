package asset

import (
	"fmt"
	"strings"

	sigsyaml "sigs.k8s.io/yaml"
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
		Labels          map[string]string `json:"labels"`
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
//   - Dash0SpamFilter, Dash0Team: dash0.com/origin, then dash0.com/id
//   - everything else (PersesDashboard, PrometheusRule, SyntheticCheck, View):
//     dash0.com/id
//
// Each kind reads only the field its Import helper actually upserts by. A
// fallback onto some other field would return an identifier no live asset can
// match, which --since would then "delete" to a 404 and report as already
// deleted -- returning "" instead routes the document to the NoIdentifier
// hard-fail. Only teams and spam filters genuinely accept either field.
func ExtractIdentifier(data []byte) (string, error) {
	var probe identifierProbe
	if err := sigsyaml.Unmarshal(data, &probe); err != nil {
		return "", fmt.Errorf("failed to extract identifier: %w", err)
	}

	origin := probe.Metadata.Labels["dash0.com/origin"]

	switch normalizeKindForIdentifier(probe.Kind) {
	case "dashboard":
		return probe.Metadata.Dash0Extensions.ID, nil
	case "checkrule":
		return probe.ID, nil
	case "notificationchannel", "timeseriesaggregation":
		return origin, nil
	case "spamfilter", "team":
		if origin != "" {
			return origin, nil
		}
		return probe.Metadata.Labels["dash0.com/id"], nil
	default:
		return probe.Metadata.Labels["dash0.com/id"], nil
	}
}

func normalizeKindForIdentifier(kind string) string {
	k := strings.ToLower(strings.ReplaceAll(kind, "-", ""))
	k = strings.ReplaceAll(k, "_", "")
	return strings.TrimPrefix(k, "dash0")
}
