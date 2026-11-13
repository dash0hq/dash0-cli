package asset

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
)

// LossyMetadataWarnings inspects the original document, before typed decoding
// or CRD conversion can discard metadata. fallbackKind is the command's kind
// for file formats that allow kind to be omitted. It never changes the input.
// Parsing and schema errors remain the responsibility of the caller's parser.
func LossyMetadataWarnings(raw []byte, fallbackKind string) []string {
	var doc struct {
		Kind string `json:"kind"`
		Spec struct {
			Groups []struct {
				Rules []struct {
					Alert string `json:"alert"`
				} `json:"rules"`
			} `json:"groups"`
		} `json:"spec"`
	}
	// Decode metadata separately: metadata.name and other scalar metadata are
	// legitimate, but labels/annotations must be inspected as maps.
	var envelope struct {
		Kind     string                     `json:"kind"`
		Metadata map[string]json.RawMessage `json:"metadata"`
		Spec     json.RawMessage            `json:"spec"`
	}
	if err := dash0yaml.Unmarshal(raw, &envelope); err != nil {
		return nil
	}
	doc.Kind = envelope.Kind
	if doc.Kind == "" {
		doc.Kind = fallbackKind
	}
	kind := NormalizeKind(doc.Kind)
	labels, annotations, known := metadataKeys(kind)
	if !known {
		return nil
	}
	// Alert annotations are inherited from metadata by the SDK. Do not apply
	// the recording-rule API's restricted metadata policy to those annotations.
	allowAlertAnnotations := false
	if kind == "prometheusrule" {
		_ = json.Unmarshal(envelope.Spec, &doc.Spec)
		for _, group := range doc.Spec.Groups {
			for _, rule := range group.Rules {
				allowAlertAnnotations = allowAlertAnnotations || rule.Alert != ""
			}
		}
	}
	var warnings []string
	for _, field := range []string{"labels", "annotations"} {
		if field == "annotations" && allowAlertAnnotations {
			continue
		}
		var entries map[string]json.RawMessage
		if json.Unmarshal(envelope.Metadata[field], &entries) != nil {
			continue
		}
		allowed := labels
		if field == "annotations" {
			allowed = annotations
		}
		for key := range entries {
			if slices.Contains(strings.Fields(allowed), key) || quietServerAnnotation(field, key) {
				continue
			}
			warnings = append(warnings, fmt.Sprintf("metadata.%s[%q] will not be stored by Dash0", field, key))
		}
	}
	sort.Strings(warnings)
	return warnings
}

// Known exported server fields stay quiet so get -> edit -> apply does not
// produce warnings for metadata the user never authored. Identifiers below
// can also be consumed by the CLI as lookup keys before they are stripped.
func quietServerAnnotation(field, key string) bool {
	return field == "annotations" && slices.Contains([]string{
		"dash0.com/created-at", "dash0.com/updated-at", "dash0.com/deleted-at",
	}, key)
}

func metadataKeys(kind string) (labels, annotations string, known bool) {
	const resourceLabels = "dash0.com/id dash0.com/origin dash0.com/source dash0.com/dataset dash0.com/version"
	const sharing = "dash0.com/folder-path dash0.com/sharing"
	switch kind {
	case "dashboard":
		return "dash0.com/source", sharing + " dash0.com/source", true
	case "persesdashboard":
		return "dash0.com/id dash0.com/dataset dash0.com/origin", sharing + " dash0.com/source", true
	case "view":
		return resourceLabels, sharing, true
	case "syntheticcheck":
		return resourceLabels + " custom", sharing, true
	case "spamfilter": // v1alpha1 and v1alpha2 have the same metadata.
		return "dash0.com/id dash0.com/origin dash0.com/source dash0.com/dataset custom", "dash0.com/enabled", true
	case "slo":
		return resourceLabels, sharing + " dash0.com/enabled", true
	case "team", "notificationchannel", "authtoken":
		return "dash0.com/id dash0.com/origin dash0.com/source", "", true
	case "dataset":
		return "dash0.com/id dash0.com/origin dash0.com/source dash0.com/version", "", true
	case "generativeaievaluation":
		return resourceLabels, sharing + " dash0.com/created-by", true
	case "skill":
		return "dash0.com/id dash0.com/origin dash0.com/source dash0.com/version",
			"dash0.com/created-by dash0.com/creator-avatar-url dash0.com/creator-name dash0.com/enabled dash0.com/overridden-by", true
	case "timeseriesaggregation":
		return resourceLabels + " custom", "", true
	case "prometheusrule", "recordingrule":
		return resourceLabels, sharing + " dash0.com/enabled dash0.com/first-evaluation-at dash0.com/runtime-disabled-at dash0.com/runtime-disabled-reason", true
	case "checkrule":
		// Native CheckRule labels and annotations live at the top level, and
		// are intentionally outside this metadata-only check.
		return "", "", true
	default:
		// AWS scrape configs already return their own metadata diagnostics.
		return "", "", false
	}
}

// PrintLossyMetadataWarnings writes diagnostics to stderr's writer without
// touching the structured result stream. It prints keys, never field values.
func PrintLossyMetadataWarnings(w io.Writer, raw []byte, kind, location string) {
	if location == "" || location == "-" {
		location = "stdin"
	}
	for _, warning := range LossyMetadataWarnings(raw, kind) {
		fmt.Fprintf(w, "warning: %s: %s\n", location, warning)
	}
}
