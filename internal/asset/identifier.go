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
//   - Dash0SpamFilter, Dash0NotificationChannel, Dash0Team: dash0.com/origin,
//     falling back to dash0.com/id
//   - everything else (PersesDashboard, PrometheusRule, SyntheticCheck, View):
//     dash0.com/id
func ExtractIdentifier(data []byte) (string, error) {
	var probe identifierProbe
	if err := sigsyaml.Unmarshal(data, &probe); err != nil {
		return "", fmt.Errorf("failed to extract identifier: %w", err)
	}

	id := probe.Metadata.Labels["dash0.com/id"]
	origin := probe.Metadata.Labels["dash0.com/origin"]

	switch normalizeKindForIdentifier(probe.Kind) {
	case "dashboard":
		if probe.Metadata.Dash0Extensions.ID != "" {
			return probe.Metadata.Dash0Extensions.ID, nil
		}
		return id, nil
	case "checkrule":
		if probe.ID != "" {
			return probe.ID, nil
		}
		return id, nil
	case "spamfilter", "notificationchannel", "team":
		if origin != "" {
			return origin, nil
		}
		return id, nil
	default:
		if id != "" {
			return id, nil
		}
		return origin, nil
	}
}

func normalizeKindForIdentifier(kind string) string {
	k := strings.ToLower(strings.ReplaceAll(kind, "-", ""))
	k = strings.ReplaceAll(k, "_", "")
	return strings.TrimPrefix(k, "dash0")
}
