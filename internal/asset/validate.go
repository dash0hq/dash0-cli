package asset

import (
	"fmt"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	sigsyaml "sigs.k8s.io/yaml"
)

// ValidateDocuments checks all documents up front, collecting all errors so a
// multi-document apply/diff is never partially triggered by a problem
// detectable before the first API call. Non-fatal warnings are collected
// separately — callers only print them when validation succeeds, since a
// warning about a document that never gets applied would be noise next to a
// hard error. Shared between apply and diff.
func ValidateDocuments(documents []Document) (validationErrors, validationWarnings []string) {
	for _, doc := range documents {
		if doc.Kind == "" {
			validationErrors = append(validationErrors, fmt.Sprintf("%s: missing 'kind' field", doc.Location()))
		} else if !IsValidKind(doc.Kind) {
			validationErrors = append(validationErrors, fmt.Sprintf("%s: unsupported kind %q (supported: Dashboard, PersesDashboard, CheckRule, PrometheusRule, SyntheticCheck, View, Dash0SpamFilter, Dash0NotificationChannel, Dash0Team)", doc.Location(), doc.Kind))
		} else if NormalizeKind(doc.Kind) == "spamfilter" {
			// Catch unknown spam filter apiVersions during validation rather
			// than after the first PUT, so a partial apply of a multi-doc input
			// is never triggered by a typo in apiVersion.
			if _, err := DetectSpamFilterAPIVersion(doc.Raw); err != nil {
				validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.Location(), err.Error()))
			}
		} else if NormalizeKind(doc.Kind) == "prometheusrule" {
			// Catch CRDs that contain no usable rules at all up front, before
			// any API call.
			if err := ValidatePrometheusRuleCRD(doc.Raw); err != nil {
				validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.Location(), err.Error()))
			}
		} else if NormalizeKind(doc.Kind) == "notificationchannel" {
			// A document carrying spec.routing.assets gets a non-fatal warning — the API treats
			// the field as API-managed and silently ignores it on write; the apply proceeds as
			// usual. Parse errors are already caught during metadata extraction in
			// ReadMultiDocumentYAML.
			var channel dash0api.NotificationChannelDefinition
			if err := sigsyaml.Unmarshal(doc.Raw, &channel); err == nil {
				if warning := RoutingAssetsWarning(&channel); warning != "" {
					validationWarnings = append(validationWarnings, fmt.Sprintf("%s: %s", doc.Location(), warning))
				}
			}
		}
	}
	return validationErrors, validationWarnings
}

// FormatValidationError formats one or more validation issues into a
// consistent "validation failed with N error/errors:" message.
func FormatValidationError(issues ...string) error {
	return fmt.Errorf("validation failed with %s:\n  %s", Pluralize(len(issues), "error"), strings.Join(issues, "\n  "))
}

// ParsePrometheusRuleCRD parses raw bytes as a PrometheusRule CRD (the typed
// dash0api.RecordingRule, an alias for the generated PrometheusRule type that
// captures both Alert and Record per rule).
func ParsePrometheusRuleCRD(data []byte) (*dash0api.RecordingRule, error) {
	var crd dash0api.RecordingRule
	if err := sigsyaml.Unmarshal(data, &crd); err != nil {
		return nil, fmt.Errorf("failed to parse PrometheusRule: %w", err)
	}
	return &crd, nil
}

// ValidatePrometheusRuleCRD rejects a PrometheusRule CRD that contains no
// alerting and no recording rules, so the failure surfaces in the validation
// phase rather than after the first request.
func ValidatePrometheusRuleCRD(data []byte) error {
	crd, err := ParsePrometheusRuleCRD(data)
	if err != nil {
		return err
	}
	if !PrometheusRuleHasAlerts(crd) && RecordingOnlyPrometheusRule(crd) == nil {
		return fmt.Errorf("PrometheusRule contains no alerting or recording rules")
	}
	return nil
}
