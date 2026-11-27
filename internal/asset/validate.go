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
			validationErrors = append(validationErrors, fmt.Sprintf("%s: unsupported kind %q (supported: Dashboard, EphemeralDashboard, PersesDashboard, CheckRule, PrometheusRule, SyntheticCheck, SLO, View, Dash0SpamFilter, Dash0NotificationChannel, Dash0Team, Dash0TimeSeriesAggregation, Dash0SignalToMetrics, Dash0Sampling, Dash0Dataset, Dash0Skill, Dash0AwsMetricScrapeConfig, Dash0GenerativeAiEvaluation)", doc.Location(), doc.Kind))
		} else if NormalizeKind(doc.Kind) == "spamfilter" {
			// Catch unknown spam filter apiVersions during validation rather
			// than after the first PUT, so a partial apply of a multi-doc input
			// is never triggered by a typo in apiVersion.
			if _, err := DetectSpamFilterAPIVersion(doc.Raw); err != nil {
				validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.Location(), err.Error()))
			}
		} else if kind := NormalizeKind(doc.Kind); kind == "dashboard" || kind == "ephemeraldashboard" {
			var dashboard dash0api.DashboardDefinition
			if err := dash0yaml.Unmarshal(doc.Raw, &dashboard); err == nil {
				for _, validate := range []func(*dash0api.DashboardDefinition) error{ValidateDashboard, ValidateEphemeralDashboard} {
					if err := validate(&dashboard); err != nil {
						validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.Location(), err.Error()))
					}
				}
			}
		} else if NormalizeKind(doc.Kind) == "prometheusrule" {
			// Catch CRDs that contain no usable rules at all up front, before
			// any API call. ParseAsPrometheusAlertRules already rejects
			// alert-only-empty CRDs, but a CRD with zero rules of either kind
			// would otherwise slip through to asset.ApplyDocument.
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
		} else if NormalizeKind(doc.Kind) == "sampling" {
			// A rule without spec.enabled, or above the condition limit, is
			// accepted by the API but cannot work as written, so it fails here,
			// before any document is applied.
			if _, err := ParseSamplingRule(doc.Raw); err != nil {
				validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", doc.Location(), err.Error()))
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
		}
	}
	validationErrors = append(validationErrors, checkRuleIDCollisions(documents)...)
	validationErrors = append(validationErrors, checkRuleIDOriginConflicts(documents)...)
	validationErrors = append(validationErrors, checkDatasetOriginCollisions(documents)...)
	return validationErrors, validationWarnings
}

// checkDatasetOriginCollisions fails datasets with different slugs that
// create at the same origin, because a slug cannot change, so the second
// write would fail after the first was applied.
func checkDatasetOriginCollisions(documents []Document) []string {
	var origins []string
	slugsByOrigin := make(map[string]map[string]bool)
	usesByOrigin := make(map[string][]string)
	for _, doc := range documents {
		if NormalizeKind(doc.Kind) != "dataset" {
			continue
		}
		var dataset dash0api.DatasetDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &dataset); err != nil {
			continue
		}
		identity := DatasetIdentity(&dataset)
		if identity.ID == "" {
			continue
		}
		origin := identity.CreateOrigin()
		if _, seen := slugsByOrigin[origin]; !seen {
			origins = append(origins, origin)
			slugsByOrigin[origin] = map[string]bool{}
		}
		slugsByOrigin[origin][identity.ID] = true
		usesByOrigin[origin] = append(usesByOrigin[origin], fmt.Sprintf("%s: slug %q", doc.Location(), identity.ID))
	}

	var collisions []string
	for _, origin := range origins {
		if len(slugsByOrigin[origin]) > 1 {
			collisions = append(collisions, fmt.Sprintf("dataset origin %q is used by datasets with different slugs, and a dataset slug cannot change, so only the first would be created; set another dash0.com/origin:\n    %s", origin, strings.Join(usesByOrigin[origin], "\n    ")))
		}
	}
	return collisions
}

func checkRuleIDCollisions(documents []Document) []string {
	var keys []string
	usesByKey := make(map[string][]string)
	fieldsByKey := make(map[string]map[string]bool)
	for _, doc := range documents {
		for _, key := range checkRuleWriteKeys(doc) {
			if _, seen := usesByKey[key.key]; !seen {
				keys = append(keys, key.key)
				fieldsByKey[key.key] = map[string]bool{}
			}
			usesByKey[key.key] = append(usesByKey[key.key], fmt.Sprintf("%s: %q", doc.Location(), key.name))
			fieldsByKey[key.key][key.field] = true
		}
	}

	var collisions []string
	for _, key := range keys {
		if uses := usesByKey[key]; len(uses) > 1 {
			field := "id"
			switch fields := fieldsByKey[key]; {
			case fields["id"] && fields["origin"]:
				field = "id and origin"
			case fields["origin"]:
				field = "origin"
			}
			collisions = append(collisions, fmt.Sprintf("check rule %s %q is used more than once, so only the last one would be kept:\n    %s", field, key, strings.Join(uses, "\n    ")))
		}
	}
	return collisions
}

func checkRuleIDOriginConflicts(documents []Document) []string {
	var conflicts []string
	for _, doc := range documents {
		kind := NormalizeKind(doc.Kind)
		if kind != "checkrule" && kind != "prometheusrule" {
			continue
		}
		rules, err := ParseCheckRules(doc.Raw)
		if err != nil {
			continue
		}
		for _, rule := range rules {
			if err := checkIDOrigin(rule.Identity(nil)); err != nil {
				conflicts = append(conflicts, fmt.Sprintf("%s: %q: %s", doc.Location(), rule.Name, err))
			}
		}
	}
	return conflicts
}

type checkRuleWriteKey struct {
	key   string
	field string
	name  string
}

// checkRuleWriteKeys returns the explicit key each check rule of doc is
// written at: its id, else its origin. A check rule is created at its id, so
// an id and another rule's origin with the same value are one key. Inferred
// origins are checked by CheckInferredOriginCollisions.
func checkRuleWriteKeys(doc Document) []checkRuleWriteKey {
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
	var keys []checkRuleWriteKey
	for _, rule := range rules {
		// rule.Origin, not rule.GetOrigin(): a PrometheusRule keeps its origin
		// in metadata.labels, which the conversion does not copy to the alerts.
		if id := rule.GetID(); id != "" {
			keys = append(keys, checkRuleWriteKey{key: id, field: "id", name: rule.Name})
		} else if rule.Origin != "" {
			keys = append(keys, checkRuleWriteKey{key: rule.Origin, field: "origin", name: rule.Name})
		}
	}
	return keys
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
