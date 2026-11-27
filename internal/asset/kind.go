package asset

import (
	"slices"
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
	return slices.Contains(ValidKinds, NormalizeKind(kind))
}

// ValidKinds lists every normalized kind apply accepts. A test checks that
// kinds holds every behavior for each one, and that diff's planDocument
// handles each one.
var ValidKinds = []string{"dashboard", "ephemeraldashboard", "checkrule", "syntheticcheck", "slo", "view", "prometheusrule", "persesdashboard", "spamfilter", "notificationchannel", "team", "timeseriesaggregation", "dataset", "skill", "awsmetricscrapeconfig", "generativeaievaluation", "signaltometrics", "sampling"}

// KindDisplayName returns the human-readable name for an asset kind.
// Multi-word kinds like "CheckRule" become "Check rule" and "SyntheticCheck"
// becomes "Synthetic check".
func KindDisplayName(kind string) string {
	if behavior, ok := kinds[NormalizeKind(kind)]; ok {
		return behavior.displayName
	}
	return kind
}
