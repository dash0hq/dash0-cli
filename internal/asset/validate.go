package asset

import (
	"fmt"
	"github.com/google/uuid"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
)

// ValidateDocuments checks all documents up front, collecting all errors so a multi-doc apply is
// never partially triggered by a problem detectable before the first API call. Non-fatal warnings
// are collected separately — callers only print them when validation succeeds, since a warning
// about a document that never gets applied would be noise next to a hard error.
func ValidateDocuments(documents []Document) (validationErrors, validationWarnings []string) {
	for _, doc := range documents {
		if doc.Kind == "" {
			validationErrors = append(validationErrors, fmt.Sprintf("%s: missing 'kind' field", doc.Location()))
		} else if NormalizeKind(doc.Kind) == "authtoken" {
			// The API returns a token's secret only on create, and apply has no safe
			// place to deliver it, so a created token's secret would be lost.
			validationErrors = append(validationErrors, fmt.Sprintf("%s: apply does not support kind %q, because it cannot deliver a new token's secret; use `dash0 --experimental auth-tokens create -f <file> --secret-file <path>` instead", doc.Location(), doc.Kind))
		} else if !IsValidKind(doc.Kind) {
			validationErrors = append(validationErrors, fmt.Sprintf("%s: unsupported kind %q (supported: Dashboard, PersesDashboard, CheckRule, PrometheusRule, SyntheticCheck, SLO, View, Dash0SpamFilter, Dash0NotificationChannel, Dash0Team, Dash0TimeSeriesAggregation, Dash0TelemetryTransformationRule, Dash0Dataset, Dash0Skill, Dash0AwsMetricScrapeConfig, Dash0GenerativeAiEvaluation)", doc.Location(), doc.Kind))
		} else if NormalizeKind(doc.Kind) == "telemetrytransformationrule" {
			if _, err := ParseTelemetryTransformationRule(doc.Raw); err != nil {
				validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.Location(), err))
			}
		} else if NormalizeKind(doc.Kind) == "spamfilter" {
			// Catch unknown spam filter apiVersions during validation rather
			// than after the first PUT, so a partial apply of a multi-doc input
			// is never triggered by a typo in apiVersion.
			if _, err := DetectSpamFilterAPIVersion(doc.Raw); err != nil {
				validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.Location(), err.Error()))
			}
		} else if NormalizeKind(doc.Kind) == "prometheusrule" {
			// Catch CRDs that contain no usable rules at all up front, before
			// any API call. ParseAsPrometheusAlertRules already rejects
			// alert-only-empty CRDs, but a CRD with zero rules of either kind
			// would otherwise slip through to applyDocument.
			if err := ValidatePrometheusRule(doc.Raw); err != nil {
				validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.Location(), err.Error()))
			}
		} else if NormalizeKind(doc.Kind) == "notificationchannel" {
			// A document carrying spec.routing.assets gets a non-fatal warning — the API treats
			// the field as API-managed and silently ignores it on write; the apply proceeds as
			// usual. Parse errors are already caught during metadata extraction in
			// readMultiDocumentYAML.
			var channel dash0api.NotificationChannelDefinition
			if err := dash0yaml.Unmarshal(doc.Raw, &channel); err == nil {
				if warning := RoutingAssetsWarning(&channel); warning != "" {
					validationWarnings = append(validationWarnings, fmt.Sprintf("%s: %s", doc.Location(), warning))
				}
			}
		} else if NormalizeKind(doc.Kind) == "dataset" {
			// A missing slug (the server cannot assign one) or an unknown apiVersion
			// fails here, before any document is applied, instead of as a server
			// 400 mid-run.
			var dataset dash0api.DatasetDefinition
			if err := dash0yaml.Unmarshal(doc.Raw, &dataset); err == nil {
				if err := ValidateDataset(&dataset); err != nil {
					validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.Location(), err.Error()))
				}
			}
		} else if NormalizeKind(doc.Kind) == "skill" {
			var skill dash0api.SkillDefinition
			if err := dash0yaml.Unmarshal(doc.Raw, &skill); err == nil {
				if err := ValidateCustomSkill(&skill); err != nil {
					validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.Location(), err.Error()))
				}
			}
		} else if NormalizeKind(doc.Kind) == "generativeaievaluation" {
			if _, err := ParseGenerativeAiEvaluation(doc.Raw); err != nil {
				validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.Location(), err.Error()))
			}
		} else if NormalizeKind(doc.Kind) == "awsmetricscrapeconfig" {
			// A missing spec.namespaces would be sent as [] and stop all
			// scraping, so the document is validated before anything is applied.
			_, warnings, err := ParseAwsMetricScrapeConfig(doc.Raw)
			switch {
			case err != nil:
				validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.Location(), err.Error()))
			case doc.ID == "":
				validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.Location(), ErrAwsMetricScrapeConfigMissingID.Error()))
			default:
				// The client rejects a non-UUID id only when it sends the request,
				// which would be after the documents before this one were applied.
				if _, err := uuid.Parse(doc.ID); err != nil {
					validationErrors = append(validationErrors, fmt.Sprintf("%s: dash0.com/id %q is not a valid AWS integration id: it must be a UUID", doc.Location(), doc.ID))
				}
			}
			for _, warning := range warnings {
				validationWarnings = append(validationWarnings, fmt.Sprintf("%s: %s", doc.Location(), warning))
			}
		} else if NormalizeKind(doc.Kind) == "timeseriesaggregation" && doc.ID == "" {
			// Origin is mandatory for this kind and there is no fallback create
			// path, so a document missing it fails here rather than after the
			// run has written its other documents. parseDocumentHeader already
			// stored the origin in doc.ID, so there is nothing to re-parse.
			validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.Location(), ErrTimeSeriesAggregationMissingOrigin.Error()))
		}
	}
	validationErrors = append(validationErrors, checkRuleIDCollisions(documents)...)
	return validationErrors, validationWarnings
}

func checkRuleIDCollisions(documents []Document) []string {
	var ids []string
	usesByID := make(map[string][]string)
	for _, doc := range documents {
		for _, rule := range checkRulesWithID(doc) {
			id := *rule.Id
			if _, seen := usesByID[id]; !seen {
				ids = append(ids, id)
			}
			usesByID[id] = append(usesByID[id], fmt.Sprintf("%s: %q", doc.Location(), rule.Name))
		}
	}

	var collisions []string
	for _, id := range ids {
		if uses := usesByID[id]; len(uses) > 1 {
			collisions = append(collisions, fmt.Sprintf("check rule id %q is used more than once, so only the last one would be kept:\n    %s", id, strings.Join(uses, "\n    ")))
		}
	}
	return collisions
}

func checkRulesWithID(doc Document) []*dash0api.PrometheusAlertRule {
	kind := NormalizeKind(doc.Kind)
	if kind != "checkrule" && kind != "prometheusrule" {
		return nil
	}
	// Parse errors, and recording-only PrometheusRules, are reported by the
	// other checks or when the document is applied.
	rules, err := ParseCheckRules(doc.Raw)
	if err != nil {
		return nil
	}
	var withID []*dash0api.PrometheusAlertRule
	for _, rule := range rules {
		if rule.Id != nil && *rule.Id != "" {
			withID = append(withID, rule)
		}
	}
	return withID
}

// FormatValidationError formats one or more validation issues into a consistent
// "validation failed with N error/errors:" message.
func FormatValidationError(issues ...string) error {
	return fmt.Errorf("validation failed with %s:\n  %s", Pluralize(len(issues), "error"), strings.Join(issues, "\n  "))
}

// ParsePrometheusRuleCRD parses raw bytes as a PrometheusRule CRD (the typed
// dash0api.RecordingRule, an alias for the generated PrometheusRule type that
// captures both Alert and Record per rule).
func ParsePrometheusRuleCRD(data []byte) (*dash0api.RecordingRule, error) {
	var crd dash0api.RecordingRule
	if err := dash0yaml.Unmarshal(data, &crd); err != nil {
		return nil, fmt.Errorf("failed to parse PrometheusRule: %w", err)
	}
	return &crd, nil
}

// ValidatePrometheusRule rejects a PrometheusRule CRD that contains no
// alerting and no recording rules, so the failure surfaces in the validation
// phase rather than after the first request.
func ValidatePrometheusRule(data []byte) error {
	crd, err := ParsePrometheusRuleCRD(data)
	if err != nil {
		return err
	}
	if !PrometheusRuleHasAlerts(crd) && RecordingOnlyPrometheusRule(crd) == nil {
		return fmt.Errorf("PrometheusRule contains no alerting or recording rules")
	}
	names, err := ExtractPrometheusAlertNames(data)
	if err != nil {
		return err
	}
	if err := CheckAlertNameCollisions(names); err != nil {
		return err
	}
	return nil
}
