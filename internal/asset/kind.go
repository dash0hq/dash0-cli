package asset

import (
	"strings"
)

// NormalizeKind lowercases kind, strips "-"/"_", and trims a leading "dash0"
// prefix, so callers can compare kind strings regardless of how they were
// cased or hyphenated in the source document (e.g. "Dash0Team" and "team"
// both normalize to "team").
func NormalizeKind(kind string) string {
	k := strings.ToLower(kind)
	k = strings.ReplaceAll(k, "-", "")
	k = strings.ReplaceAll(k, "_", "")
	return strings.TrimPrefix(k, "dash0")
}

// IsValidKind reports whether kind (in any casing/hyphenation NormalizeKind
// accepts) is one of the Dash0 asset kinds apply/create/--since know how to
// handle. Used to distinguish a genuine Dash0 document from unrelated YAML
// (e.g. a stray Kubernetes ConfigMap) that happens to sit in a scanned scope.
func IsValidKind(kind string) bool {
	switch NormalizeKind(kind) {
	case "dashboard", "checkrule", "syntheticcheck", "view", "prometheusrule", "persesdashboard", "spamfilter", "notificationchannel", "team":
		return true
	default:
		return false
	}
}

// KindDisplayName returns the human-readable name for an asset kind.
// Multi-word kinds like "CheckRule" become "Check rule" and "SyntheticCheck"
// becomes "Synthetic check".
func KindDisplayName(kind string) string {
	k := NormalizeKind(kind)
	switch k {
	case "dashboard":
		return "Dashboard"
	case "checkrule":
		return "Check rule"
	case "syntheticcheck":
		return "Synthetic check"
	case "view":
		return "View"
	case "prometheusrule":
		return "PrometheusRule"
	case "persesdashboard":
		return "PersesDashboard"
	case "recordingrule":
		return "Recording rule"
	case "notificationchannel":
		return "Notification channel"
	case "spamfilter":
		return "Spam filter"
	case "team":
		return "Team"
	default:
		return kind
	}
}
