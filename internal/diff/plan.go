package diff

import (
	"context"
	"fmt"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
)

// docPlan is one asset a document would create or update. A PrometheusRule
// CRD expands to one per alert plus one for its recording rules.
type docPlan struct {
	displayKind string
	name        string
	plan        asset.ImportPlan
}

// planDocument reads the stored state of every asset doc would write, through
// the same asset.Plan* step apply runs before its write. Any read error other
// than "not found" is returned, so the caller can abort the whole preview.
func planDocument(ctx context.Context, apiClient dash0api.Client, doc asset.Document, dataset *string) ([]docPlan, error) {
	switch asset.NormalizeKind(doc.Kind) {
	case "dashboard", "persesdashboard":
		dashboard, err := dash0yaml.ParseAsDashboard(doc.Raw)
		if err != nil {
			return nil, err
		}
		plan, err := asset.PlanDashboard(ctx, apiClient, dashboard, dataset)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "dashboard", AssetID: plan.Key})
		}
		name := dash0api.GetDashboardName(dashboard)
		if name == "" {
			name = dashboard.Metadata.Name
		}
		return []docPlan{{displayKind: "Dashboard", name: name, plan: plan}}, nil

	case "checkrule":
		return planCheckRules(ctx, apiClient, doc, dataset)

	case "prometheusrule":
		return planPrometheusRule(ctx, apiClient, doc, dataset)

	case "syntheticcheck":
		var check dash0api.SyntheticCheckDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &check); err != nil {
			return nil, fmt.Errorf("failed to parse SyntheticCheck: %w", err)
		}
		plan, err := asset.PlanSyntheticCheck(ctx, apiClient, &check, dataset)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "synthetic check", AssetID: plan.Key})
		}
		return []docPlan{{displayKind: doc.Kind, name: check.Metadata.Name, plan: plan}}, nil

	case "slo":
		var slo dash0api.SloDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &slo); err != nil {
			return nil, fmt.Errorf("failed to parse SLO: %w", err)
		}
		plan, err := asset.PlanSLO(ctx, apiClient, &slo, dataset)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "SLO", AssetName: dash0api.GetSLOName(&slo)})
		}
		return []docPlan{{displayKind: "SLO", name: dash0api.GetSLOName(&slo), plan: plan}}, nil

	case "timeseriesaggregation":
		var aggregation dash0api.TimeSeriesAggregationDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &aggregation); err != nil {
			return nil, fmt.Errorf("failed to parse Dash0TimeSeriesAggregation: %w", err)
		}
		plan, err := asset.PlanTimeSeriesAggregation(ctx, apiClient, &aggregation, dataset)
		if err != nil {
			// The cross-dataset collision carries its own explanation.
			if asset.IsTimeSeriesAggregationWrongDataset(err) {
				return nil, err
			}
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "time series aggregation", AssetID: plan.Key})
		}
		return []docPlan{{displayKind: "Dash0TimeSeriesAggregation", name: dash0api.GetTimeSeriesAggregationName(&aggregation), plan: plan}}, nil

	case "dataset":
		var ds dash0api.DatasetDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &ds); err != nil {
			return nil, fmt.Errorf("failed to parse Dash0Dataset: %w", err)
		}
		plan, err := asset.PlanDataset(ctx, apiClient, &ds)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "dataset", AssetID: plan.Key})
		}
		return []docPlan{{displayKind: "Dash0Dataset", name: dash0api.GetDatasetName(&ds), plan: plan}}, nil

	case "view":
		var view dash0api.ViewDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &view); err != nil {
			return nil, fmt.Errorf("failed to parse View: %w", err)
		}
		plan, err := asset.PlanView(ctx, apiClient, &view, dataset)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "view", AssetID: plan.Key})
		}
		// The API can give a view a new id and still find it by the local id,
		// so the stored id names the same view and is not a change.
		if before, ok := plan.Before.(*dash0api.ViewDefinition); ok && before.Metadata.Labels != nil && before.Metadata.Labels.Dash0Comid != nil {
			view.Metadata.Labels.Dash0Comid = before.Metadata.Labels.Dash0Comid
		}
		return []docPlan{{displayKind: doc.Kind, name: view.Metadata.Name, plan: plan}}, nil

	case "spamfilter":
		return planSpamFilter(ctx, apiClient, doc, dataset)

	case "notificationchannel":
		var channel dash0api.NotificationChannelDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &channel); err != nil {
			return nil, fmt.Errorf("failed to parse Dash0NotificationChannel: %w", err)
		}
		plan, err := asset.PlanNotificationChannel(ctx, apiClient, &channel)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "notification channel", AssetID: plan.Key})
		}
		return []docPlan{{displayKind: "Dash0NotificationChannel", name: dash0api.GetNotificationChannelName(&channel), plan: plan}}, nil

	case "team":
		var team dash0api.TeamDefinitionV1Alpha1
		if err := dash0yaml.Unmarshal(doc.Raw, &team); err != nil {
			return nil, fmt.Errorf("failed to parse Dash0Team: %w", err)
		}
		plan, err := asset.PlanTeam(ctx, apiClient, &team)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "team", AssetName: dash0api.GetTeamDisplayName(&team)})
		}
		name := dash0api.GetTeamDisplayName(&team)
		if name == "" {
			name = dash0api.GetTeamName(&team)
		}
		return []docPlan{{displayKind: "Dash0Team", name: name, plan: plan}}, nil

	default:
		return nil, fmt.Errorf("unsupported kind: %s", doc.Kind)
	}
}

// planCheckRules handles a CheckRule document, or the alerting rules of a
// PrometheusRule CRD: asset.ParseCheckRules returns one rule per alert.
func planCheckRules(ctx context.Context, apiClient dash0api.Client, doc asset.Document, dataset *string) ([]docPlan, error) {
	rules, err := asset.ParseCheckRules(doc.Raw)
	if err != nil {
		return nil, err
	}
	var plans []docPlan
	for _, rule := range rules {
		plan, err := asset.PlanCheckRule(ctx, apiClient, rule, dataset)
		if err != nil {
			return plans, client.HandleAPIError(err, client.ErrorContext{AssetType: "check rule", AssetID: plan.Key, AssetName: rule.Name})
		}
		plans = append(plans, docPlan{displayKind: "CheckRule", name: rule.Name, plan: plan})
	}
	return plans, nil
}

// planPrometheusRule handles a PrometheusRule CRD with alerting rules,
// recording rules, or both, the same way applyPrometheusRule dispatches it.
func planPrometheusRule(ctx context.Context, apiClient dash0api.Client, doc asset.Document, dataset *string) ([]docPlan, error) {
	crd, err := asset.ParsePrometheusRuleCRD(doc.Raw)
	if err != nil {
		return nil, err
	}

	var plans []docPlan
	if asset.PrometheusRuleHasAlerts(crd) {
		alertPlans, err := planCheckRules(ctx, apiClient, doc, dataset)
		plans = append(plans, alertPlans...)
		if err != nil {
			return plans, err
		}
	}

	if recordingOnly := asset.RecordingOnlyPrometheusRule(crd); recordingOnly != nil {
		name := dash0api.GetRecordingRuleName(recordingOnly)
		plan, err := asset.PlanRecordingRule(ctx, apiClient, recordingOnly, dataset)
		if err != nil {
			return plans, client.HandleAPIError(err, client.ErrorContext{AssetType: "recording rule", AssetID: plan.Key, AssetName: name})
		}
		plans = append(plans, docPlan{displayKind: "RecordingRule", name: name, plan: plan})
	}
	return plans, nil
}

// planSpamFilter handles v1alpha1 and v1alpha2 spam filter documents. The
// apiVersion field selects the schema.
func planSpamFilter(ctx context.Context, apiClient dash0api.Client, doc asset.Document, dataset *string) ([]docPlan, error) {
	apiVersion, err := asset.DetectSpamFilterAPIVersion(doc.Raw)
	if err != nil {
		return nil, err
	}

	switch apiVersion {
	case string(dash0api.SpamFilterApiVersionV1Alpha1V1alpha1):
		var filter dash0api.SpamFilter
		if err := dash0yaml.Unmarshal(doc.Raw, &filter); err != nil {
			return nil, fmt.Errorf("failed to parse v1alpha1 SpamFilter: %w", err)
		}
		plan, err := asset.PlanSpamFilter(ctx, apiClient, &filter, dataset)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "spam filter", AssetID: plan.Key})
		}
		return []docPlan{{displayKind: doc.Kind, name: dash0api.GetSpamFilterName(&filter), plan: plan}}, nil

	case string(dash0api.SpamFilterApiVersionV1Alpha2V1alpha2):
		var filter dash0api.SpamFilterV1Alpha2
		if err := dash0yaml.Unmarshal(doc.Raw, &filter); err != nil {
			return nil, fmt.Errorf("failed to parse v1alpha2 SpamFilter: %w", err)
		}
		plan, err := asset.PlanSpamFilterV1Alpha2(ctx, apiClient, &filter, dataset)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "spam filter", AssetID: plan.Key})
		}
		return []docPlan{{displayKind: doc.Kind, name: filter.Metadata.Name, plan: plan}}, nil

	default:
		// Unreachable: DetectSpamFilterAPIVersion only returns supported values or an error.
		return nil, fmt.Errorf("unsupported spam filter apiVersion %q", apiVersion)
	}
}
