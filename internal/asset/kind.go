package asset

import (
	"strings"

	"sigs.k8s.io/yaml"
)

// DetectKind extracts the "kind" field from raw YAML/JSON bytes. When the
// document has no explicit "kind" (e.g. a check rule exported via
// `check-rules get -o yaml`), the kind is inferred from the document
// structure: the "expression" field is required for check rules and absent
// in all other asset types.
func DetectKind(data []byte) string {
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return ""
	}
	if kind, ok := doc["kind"].(string); ok && kind != "" {
		return kind
	}
	_, hasName := doc["name"]
	_, hasExpr := doc["expression"]
	if hasName && hasExpr {
		return "CheckRule"
	}
	return ""
}

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
	default:
		return kind
	}
}
