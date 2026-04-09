package asset

import (
	"strings"
)

// KindDisplayName returns the human-readable name for an asset kind.
// Multi-word kinds like "CheckRule" become "Check rule" and "SyntheticCheck"
// becomes "Synthetic check".
func KindDisplayName(kind string) string {
	k := strings.ToLower(kind)
	k = strings.ReplaceAll(k, "-", "")
	k = strings.ReplaceAll(k, "_", "")
	k = strings.TrimPrefix(k, "dash0")
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
	case "recordingrulegroup":
		return "Recording rule"
	default:
		return kind
	}
}
