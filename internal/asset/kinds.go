package asset

import (
	"cmp"
	"context"
	"fmt"
	"os"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"github.com/dash0hq/dash0-cli/internal/client"
)

// kindBehavior is what the CLI knows about one asset kind. Every kind in
// ValidKinds sets every field except idOnlyDelete, and a test checks this.
type kindBehavior struct {
	displayName string
	// decodeHeader returns the name and the identifier that validation and
	// the dry run show for a document.
	decodeHeader func(data []byte) (name, id string, err error)
	apply        func(ctx context.Context, apiClient dash0api.Client, doc Document, dataset *string) ([]AppliedAsset, error)
	// idOnlyDelete is the warning `apply --since` prints before it deletes an
	// asset its document identified by dash0.com/id alone, or nil.
	idOnlyDelete *idOnlyDeleteWarning
}

// kinds maps every normalized kind to its behavior. Recording rules and auth
// tokens only have a display name: apply reaches recording rules through
// PrometheusRule, and does not accept auth tokens.
var kinds = map[string]kindBehavior{
	"dashboard":          {displayName: "Dashboard", decodeHeader: dashboardHeader, apply: applyDashboard},
	"ephemeraldashboard": {displayName: "Ephemeral dashboard", decodeHeader: dashboardHeader, apply: applyDashboard},
	"persesdashboard": {
		displayName: "PersesDashboard",
		decodeHeader: header(func(perses *dash0api.PersesDashboard) (string, string) {
			id := dash0api.GetPersesDashboardID(perses)
			if id == "" {
				id = perses.Metadata.Labels[dash0api.LabelOrigin]
			}
			return dash0api.GetPersesDashboardName(perses), id
		}),
		apply: applyDashboard,
	},
	"checkrule": {
		displayName: "Check rule",
		decodeHeader: header(func(rule *dash0api.PrometheusAlertRule) (string, string) {
			return dash0api.GetCheckRuleName(rule), idThenOrigin(rule)
		}),
		apply: applyCheckRule,
	},
	"prometheusrule": {
		displayName: "PrometheusRule",
		// Only the metadata is read, which has no time.Duration fields.
		decodeHeader: header(func(partial *struct {
			Metadata dash0api.PrometheusRulesMetadata `json:"metadata"`
		}) (string, string) {
			id := partial.Metadata.Labels[dash0api.LabelID]
			if id == "" {
				id = partial.Metadata.Labels[dash0api.LabelOrigin]
			}
			return partial.Metadata.Name, id
		}),
		apply: applyPrometheusRule,
	},
	"recordingrule": {displayName: "Recording rule"},
	"syntheticcheck": {
		displayName: "Synthetic check",
		decodeHeader: header(func(check *dash0api.SyntheticCheckDefinition) (string, string) {
			return dash0api.GetSyntheticCheckName(check), idThenOrigin(check)
		}),
		apply: func(ctx context.Context, apiClient dash0api.Client, doc Document, dataset *string) ([]AppliedAsset, error) {
			var check dash0api.SyntheticCheckDefinition
			if err := dash0yaml.Unmarshal(doc.Raw, &check); err != nil {
				return nil, fmt.Errorf("failed to parse SyntheticCheck: %w", err)
			}
			result, err := ImportSyntheticCheck(ctx, apiClient, &check, dataset)
			if err != nil {
				return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "synthetic check", AssetName: check.Metadata.Name})
			}
			return []AppliedAsset{{Kind: doc.Kind, ImportResult: result}}, nil
		},
	},
	"slo": {
		displayName: "SLO",
		decodeHeader: header(func(slo *dash0api.SloDefinition) (string, string) {
			return dash0api.GetSLOName(slo), PreferredKey(slo)
		}),
		apply: func(ctx context.Context, apiClient dash0api.Client, doc Document, dataset *string) ([]AppliedAsset, error) {
			var slo dash0api.SloDefinition
			if err := dash0yaml.Unmarshal(doc.Raw, &slo); err != nil {
				return nil, fmt.Errorf("failed to parse SLO: %w", err)
			}
			result, err := ImportSLO(ctx, apiClient, &slo, dataset)
			if err != nil {
				return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "SLO", AssetName: dash0api.GetSLOName(&slo)})
			}
			return []AppliedAsset{{Kind: doc.Kind, ImportResult: result}}, nil
		},
		idOnlyDelete: idOnlyDelete("SLO", "an SLO", "SLOs", SLOUsesOrigin),
	},
	"view": {
		displayName: "View",
		decodeHeader: header(func(view *dash0api.ViewDefinition) (string, string) {
			return dash0api.GetViewName(view), idThenOrigin(view)
		}),
		apply: func(ctx context.Context, apiClient dash0api.Client, doc Document, dataset *string) ([]AppliedAsset, error) {
			var view dash0api.ViewDefinition
			if err := dash0yaml.Unmarshal(doc.Raw, &view); err != nil {
				return nil, fmt.Errorf("failed to parse View: %w", err)
			}
			result, err := ImportView(ctx, apiClient, &view, dataset)
			if err != nil {
				return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "view", AssetName: view.Metadata.Name})
			}
			return []AppliedAsset{{Kind: doc.Kind, ImportResult: result}}, nil
		},
	},
	"spamfilter": {
		displayName: "Spam filter",
		// The metadata is the same in v1alpha1 and v1alpha2, so the v1alpha1
		// type reads the header of both.
		decodeHeader: header(func(filter *dash0api.SpamFilter) (string, string) {
			return dash0api.GetSpamFilterName(filter), dash0api.GetSpamFilterID(filter)
		}),
		apply:        applySpamFilter,
		idOnlyDelete: idOnlyDelete("spam filter", "a spam filter", "spam filters", SpamFilterUsesOrigin),
	},
	"notificationchannel": {
		displayName: "Notification channel",
		decodeHeader: header(func(channel *dash0api.NotificationChannelDefinition) (string, string) {
			id := dash0api.GetNotificationChannelID(channel)
			if id == "" {
				id = dash0api.GetNotificationChannelOrigin(channel)
			}
			return dash0api.GetNotificationChannelName(channel), id
		}),
		apply: func(ctx context.Context, apiClient dash0api.Client, doc Document, _ *string) ([]AppliedAsset, error) {
			var channel dash0api.NotificationChannelDefinition
			if err := dash0yaml.Unmarshal(doc.Raw, &channel); err != nil {
				return nil, fmt.Errorf("failed to parse Dash0NotificationChannel: %w", err)
			}
			result, err := ImportNotificationChannel(ctx, apiClient, &channel)
			if err != nil {
				return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "notification channel", AssetName: dash0api.GetNotificationChannelName(&channel)})
			}
			return []AppliedAsset{{Kind: "Dash0NotificationChannel", ImportResult: result}}, nil
		},
	},
	"timeseriesaggregation": {
		displayName: "Time series aggregation",
		decodeHeader: header(func(aggregation *dash0api.TimeSeriesAggregationDefinition) (string, string) {
			return dash0api.GetTimeSeriesAggregationName(aggregation), GetTimeSeriesAggregationOrigin(aggregation)
		}),
		apply: func(ctx context.Context, apiClient dash0api.Client, doc Document, dataset *string) ([]AppliedAsset, error) {
			var aggregation dash0api.TimeSeriesAggregationDefinition
			if err := dash0yaml.Unmarshal(doc.Raw, &aggregation); err != nil {
				return nil, fmt.Errorf("failed to parse Dash0TimeSeriesAggregation: %w", err)
			}
			result, err := ImportTimeSeriesAggregation(ctx, apiClient, &aggregation, dataset)
			if err != nil {
				// The cross-dataset collision carries its own explanation.
				if IsTimeSeriesAggregationWrongDataset(err) {
					return nil, err
				}
				return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "time series aggregation", AssetName: dash0api.GetTimeSeriesAggregationName(&aggregation)})
			}
			return []AppliedAsset{{Kind: "Dash0TimeSeriesAggregation", ImportResult: result}}, nil
		},
	},
	"signaltometrics": {
		displayName: SignalToMetricsRuleDisplayKind,
		decodeHeader: header(func(rule *dash0api.SignalToMetricsDefinition) (string, string) {
			return dash0api.GetSignalToMetricsName(rule), SignalToMetricsRuleKey(rule)
		}),
		apply: func(ctx context.Context, apiClient dash0api.Client, doc Document, dataset *string) ([]AppliedAsset, error) {
			var rule dash0api.SignalToMetricsDefinition
			if err := dash0yaml.Unmarshal(doc.Raw, &rule); err != nil {
				return nil, fmt.Errorf("failed to parse Dash0SignalToMetrics: %w", err)
			}
			result, err := ImportSignalToMetricsRule(ctx, apiClient, &rule, dataset)
			if err != nil {
				// The cross-dataset collision carries its own explanation.
				if IsSignalToMetricsRuleWrongDataset(err) {
					return nil, err
				}
				return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "signal-to-metrics rule", AssetName: dash0api.GetSignalToMetricsName(&rule)})
			}
			return []AppliedAsset{{Kind: "Dash0SignalToMetrics", ImportResult: result}}, nil
		},
		idOnlyDelete: idOnlyDelete("signal-to-metrics rule", "a signal-to-metrics rule", "signal-to-metrics rules", SignalToMetricsRuleUsesOrigin),
	},
	"sampling": {
		displayName: SamplingRuleDisplayKind,
		decodeHeader: header(func(rule *dash0api.SamplingDefinition) (string, string) {
			return dash0api.GetSamplingRuleName(rule), SamplingRuleKey(rule)
		}),
		apply: func(ctx context.Context, apiClient dash0api.Client, doc Document, dataset *string) ([]AppliedAsset, error) {
			rule, err := ParseSamplingRule(doc.Raw)
			if err != nil {
				return nil, err
			}
			result, err := ImportSamplingRule(ctx, apiClient, rule, dataset)
			if err != nil {
				// The cross-dataset collision carries its own explanation.
				if IsSamplingRuleWrongDataset(err) {
					return nil, err
				}
				return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "sampling rule", AssetName: dash0api.GetSamplingRuleName(rule)})
			}
			for _, warning := range result.Warnings {
				fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
			}
			return []AppliedAsset{{Kind: "Dash0Sampling", ImportResult: result}}, nil
		},
		idOnlyDelete: idOnlyDelete("sampling rule", "a sampling rule", "sampling rules", SamplingRuleUsesOrigin),
	},
	"team": {
		displayName: "Team",
		decodeHeader: header(func(team *dash0api.TeamDefinitionV1Alpha1) (string, string) {
			name := dash0api.GetTeamDisplayName(team)
			if name == "" {
				name = dash0api.GetTeamName(team)
			}
			return name, PreferredKey(team)
		}),
		apply: func(ctx context.Context, apiClient dash0api.Client, doc Document, _ *string) ([]AppliedAsset, error) {
			var team dash0api.TeamDefinitionV1Alpha1
			if err := dash0yaml.Unmarshal(doc.Raw, &team); err != nil {
				return nil, fmt.Errorf("failed to parse Dash0Team: %w", err)
			}
			result, err := ImportTeam(ctx, apiClient, &team)
			if err != nil {
				return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "team", AssetName: dash0api.GetTeamDisplayName(&team)})
			}
			return []AppliedAsset{{Kind: "Dash0Team", ImportResult: result}}, nil
		},
		idOnlyDelete: idOnlyDelete("team", "a team", "teams", TeamUsesOrigin),
	},
	"dataset": {
		displayName: "Dataset",
		decodeHeader: header(func(dataset *dash0api.DatasetDefinition) (string, string) {
			return dash0api.GetDatasetName(dataset), dash0api.GetDatasetID(dataset)
		}),
		apply: func(ctx context.Context, apiClient dash0api.Client, doc Document, _ *string) ([]AppliedAsset, error) {
			var dataset dash0api.DatasetDefinition
			if err := dash0yaml.Unmarshal(doc.Raw, &dataset); err != nil {
				return nil, fmt.Errorf("failed to parse Dash0Dataset: %w", err)
			}
			result, err := ImportDataset(ctx, apiClient, &dataset)
			if err != nil {
				return nil, client.HandleAPIError(err, client.ErrorContext{
					AssetType: "dataset",
					AssetID:   dash0api.GetDatasetID(&dataset),
					AssetName: dash0api.GetDatasetName(&dataset),
				})
			}
			return []AppliedAsset{{Kind: "Dash0Dataset", ImportResult: result}}, nil
		},
	},
	"authtoken": {displayName: "Auth token"},
	"skill": {
		displayName: "Agent0 skill",
		decodeHeader: header(func(skill *dash0api.SkillDefinition) (string, string) {
			return CustomSkillName(skill), CustomSkillKey(skill)
		}),
		apply: func(ctx context.Context, apiClient dash0api.Client, doc Document, _ *string) ([]AppliedAsset, error) {
			var skill dash0api.SkillDefinition
			if err := dash0yaml.Unmarshal(doc.Raw, &skill); err != nil {
				return nil, fmt.Errorf("failed to parse Dash0Skill: %w", err)
			}
			name := CustomSkillName(&skill)
			result, err := ImportCustomSkill(ctx, apiClient, &skill)
			if err != nil {
				if explained := ExplainCustomSkillError(err, name); explained != nil {
					return nil, explained
				}
				return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "Agent0 skill", AssetName: name})
			}
			return []AppliedAsset{{Kind: "Dash0Skill", ImportResult: result}}, nil
		},
	},
	"awsmetricscrapeconfig": {
		displayName: "AWS metric scrape config",
		// The integration id is the only key: there is one config per integration.
		decodeHeader: header(func(cfg *dash0api.AwsMetricScrapeConfigDefinition) (string, string) {
			return dash0api.GetAwsMetricScrapeConfigName(cfg), dash0api.GetAwsMetricScrapeConfigID(cfg)
		}),
		// Organization-level: the integration decides the dataset.
		apply: func(ctx context.Context, apiClient dash0api.Client, doc Document, _ *string) ([]AppliedAsset, error) {
			cfg, _, err := ParseAwsMetricScrapeConfig(doc.Raw)
			if err != nil {
				return nil, err
			}
			result, err := ImportAwsMetricScrapeConfig(ctx, apiClient, cfg, doc.ID)
			if err != nil {
				return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "AWS integration", AssetID: doc.ID})
			}
			return []AppliedAsset{{Kind: AwsMetricScrapeConfigKind, ImportResult: result}}, nil
		},
	},
	"generativeaievaluation": {
		displayName: GenerativeAiEvaluationDisplayName,
		decodeHeader: header(func(evaluation *dash0api.GenerativeAiEvaluationDefinition) (string, string) {
			return dash0api.GetGenerativeAiEvaluationName(evaluation), PreferredKey(evaluation)
		}),
		apply: func(ctx context.Context, apiClient dash0api.Client, doc Document, dataset *string) ([]AppliedAsset, error) {
			var evaluation dash0api.GenerativeAiEvaluationDefinition
			if err := dash0yaml.Unmarshal(doc.Raw, &evaluation); err != nil {
				return nil, fmt.Errorf("failed to parse Dash0GenerativeAiEvaluation: %w", err)
			}
			name := dash0api.GetGenerativeAiEvaluationName(&evaluation)
			result, err := ImportGenerativeAiEvaluation(ctx, apiClient, &evaluation, dataset)
			if err != nil {
				if explained := ExplainGenerativeAiEvaluationError(err, name); explained != nil {
					return nil, explained
				}
				return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "eval", AssetName: name})
			}
			return []AppliedAsset{{Kind: "Dash0GenerativeAiEvaluation", ImportResult: result}}, nil
		},
		idOnlyDelete: idOnlyDelete("eval", "an eval", "evals", GenerativeAiEvaluationUsesOrigin),
	},
}

// AppliedAsset is one asset that apply created, updated, or restored. Kind is
// the kind its success line names.
type AppliedAsset struct {
	Kind string
	ImportResult
}

// ApplyDocument writes every asset of doc. A PrometheusRule writes one check
// rule per alert and one recording rule, so it can return several.
func ApplyDocument(ctx context.Context, apiClient dash0api.Client, doc Document, dataset *string) ([]AppliedAsset, error) {
	behavior := kinds[NormalizeKind(doc.Kind)]
	if behavior.apply == nil {
		return nil, fmt.Errorf("unsupported kind: %s", doc.Kind)
	}
	return behavior.apply(ctx, apiClient, doc, dataset)
}

// header decodes a document into T and reads its name and identifier.
func header[T any](read func(*T) (name, id string)) func([]byte) (string, string, error) {
	return func(data []byte) (string, string, error) {
		var definition T
		if err := dash0yaml.Unmarshal(data, &definition); err != nil {
			return "", "", fmt.Errorf("failed to decode document: %w", err)
		}
		name, id := read(&definition)
		return name, id, nil
	}
}

var dashboardHeader = header(func(dashboard *dash0api.DashboardDefinition) (string, string) {
	return cmp.Or(dash0api.GetDashboardName(dashboard), dashboard.Metadata.Name), DashboardKey(dashboard)
})

func applyDashboard(ctx context.Context, apiClient dash0api.Client, doc Document, dataset *string) ([]AppliedAsset, error) {
	dashboard, inferredName, err := ParseDashboard(doc.Raw)
	if err != nil {
		return nil, err
	}
	result, err := ImportDashboard(ctx, apiClient, dashboard, inferredName, dataset)
	if err != nil {
		return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "dashboard", AssetName: dash0api.GetDashboardName(dashboard)})
	}
	return []AppliedAsset{{Kind: dashboardKind(dashboard), ImportResult: result}}, nil
}

// applyCheckRule writes the check rules of a CheckRule document, or of the
// alerting rules of a PrometheusRule.
func applyCheckRule(ctx context.Context, apiClient dash0api.Client, doc Document, dataset *string) ([]AppliedAsset, error) {
	rules, err := ParseCheckRules(doc.Raw)
	if err != nil {
		return nil, err
	}
	var applied []AppliedAsset
	for _, rule := range rules {
		result, importErr := ImportCheckRule(ctx, apiClient, rule, dataset)
		if importErr != nil {
			return applied, client.HandleAPIError(importErr, client.ErrorContext{AssetType: "check rule", AssetName: rule.Name})
		}
		applied = append(applied, AppliedAsset{Kind: "CheckRule", ImportResult: result})
	}
	return applied, nil
}

// applyPrometheusRule sends the alerting rules to the check-rule endpoint and
// the recording rules, as one recording-only CRD, to the recording-rule
// endpoint.
func applyPrometheusRule(ctx context.Context, apiClient dash0api.Client, doc Document, dataset *string) ([]AppliedAsset, error) {
	crd, err := ParsePrometheusRuleCRD(doc.Raw)
	if err != nil {
		return nil, err
	}
	var applied []AppliedAsset
	if PrometheusRuleHasAlerts(crd) {
		alerts, err := applyCheckRule(ctx, apiClient, doc, dataset)
		applied = append(applied, alerts...)
		if err != nil {
			return applied, err
		}
	}
	if recordingOnly := RecordingOnlyPrometheusRule(crd); recordingOnly != nil {
		result, importErr := ImportRecordingRule(ctx, apiClient, recordingOnly, dataset)
		if importErr != nil {
			return applied, client.HandleAPIError(importErr, client.ErrorContext{AssetType: "recording rule", AssetName: dash0api.GetRecordingRuleName(recordingOnly)})
		}
		applied = append(applied, AppliedAsset{Kind: "RecordingRule", ImportResult: result})
	}
	return applied, nil
}

// applySpamFilter picks the schema by the document's apiVersion.
func applySpamFilter(ctx context.Context, apiClient dash0api.Client, doc Document, dataset *string) ([]AppliedAsset, error) {
	apiVersion, err := DetectSpamFilterAPIVersion(doc.Raw)
	if err != nil {
		return nil, err
	}
	switch apiVersion {
	case string(dash0api.SpamFilterApiVersionV1Alpha1V1alpha1):
		var filter dash0api.SpamFilter
		if err := dash0yaml.Unmarshal(doc.Raw, &filter); err != nil {
			return nil, fmt.Errorf("failed to parse v1alpha1 SpamFilter: %w", err)
		}
		result, importErr := ImportSpamFilter(ctx, apiClient, &filter, dataset)
		if importErr != nil {
			return nil, client.HandleAPIError(importErr, client.ErrorContext{AssetType: "spam filter", AssetName: dash0api.GetSpamFilterName(&filter)})
		}
		return []AppliedAsset{{Kind: doc.Kind, ImportResult: result}}, nil
	case string(dash0api.SpamFilterApiVersionV1Alpha2V1alpha2):
		var filter dash0api.SpamFilterV1Alpha2
		if err := dash0yaml.Unmarshal(doc.Raw, &filter); err != nil {
			return nil, fmt.Errorf("failed to parse v1alpha2 SpamFilter: %w", err)
		}
		result, importErr := ImportSpamFilterV1Alpha2(ctx, apiClient, &filter, dataset)
		if importErr != nil {
			return nil, client.HandleAPIError(importErr, client.ErrorContext{AssetType: "spam filter", AssetName: filter.Metadata.Name})
		}
		return []AppliedAsset{{Kind: doc.Kind, ImportResult: result}}, nil
	default:
		// Unreachable: DetectSpamFilterAPIVersion only returns supported values or an error.
		return nil, fmt.Errorf("unsupported spam filter apiVersion %q", apiVersion)
	}
}
