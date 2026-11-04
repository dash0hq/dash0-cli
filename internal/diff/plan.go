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
	// readDenied reports that the read answered 403 and the plan assumes a
	// create, because this run creates the dataset.
	readDenied bool
}

// planDocument reads the stored state of every asset doc would write, through
// the same asset.Plan* step apply runs before its write. Any read error other
// than "not found" is returned, so the caller can abort the whole preview.
// inNewDataset reports that this run creates the dataset, so a 403 from a
// dataset-scoped read means nothing is stored, and the plan stays a create.
func planDocument(ctx context.Context, apiClient dash0api.Client, doc asset.Document, dataset *string, inNewDataset bool) ([]docPlan, error) {
	switch asset.NormalizeKind(doc.Kind) {
	case "dashboard", "persesdashboard":
		dashboard, err := dash0yaml.ParseAsDashboard(doc.Raw)
		if err != nil {
			return nil, err
		}
		plan, err := asset.PlanDashboard(ctx, apiClient, dashboard, dataset)
		denied, err := readError(err, inNewDataset)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "dashboard", AssetID: plan.Key})
		}
		name := dash0api.GetDashboardName(dashboard)
		if name == "" {
			name = dashboard.Metadata.Name
		}
		return []docPlan{{displayKind: "Dashboard", name: name, plan: plan, readDenied: denied}}, nil

	case "checkrule":
		return planCheckRules(ctx, apiClient, doc, dataset, inNewDataset)

	case "prometheusrule":
		return planPrometheusRule(ctx, apiClient, doc, dataset, inNewDataset)

	case "syntheticcheck":
		var check dash0api.SyntheticCheckDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &check); err != nil {
			return nil, fmt.Errorf("failed to parse SyntheticCheck: %w", err)
		}
		plan, err := asset.PlanSyntheticCheck(ctx, apiClient, &check, dataset)
		denied, err := readError(err, inNewDataset)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "synthetic check", AssetID: plan.Key})
		}
		// Like a view, a synthetic check can get a new id and still be found
		// by the local one.
		if before, ok := plan.Before.(*dash0api.SyntheticCheckDefinition); ok && before.Metadata.Labels != nil && before.Metadata.Labels.Dash0Comid != nil && check.Metadata.Labels != nil {
			check.Metadata.Labels.Dash0Comid = before.Metadata.Labels.Dash0Comid
		}
		return []docPlan{{displayKind: doc.Kind, name: check.Metadata.Name, plan: plan, readDenied: denied}}, nil

	case "slo":
		var slo dash0api.SloDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &slo); err != nil {
			return nil, fmt.Errorf("failed to parse SLO: %w", err)
		}
		plan, err := asset.PlanSLO(ctx, apiClient, &slo, dataset)
		denied, err := readError(err, inNewDataset)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "SLO", AssetName: dash0api.GetSLOName(&slo)})
		}
		return []docPlan{{displayKind: "SLO", name: dash0api.GetSLOName(&slo), plan: plan, readDenied: denied}}, nil

	case "timeseriesaggregation":
		var aggregation dash0api.TimeSeriesAggregationDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &aggregation); err != nil {
			return nil, fmt.Errorf("failed to parse Dash0TimeSeriesAggregation: %w", err)
		}
		plan, err := asset.PlanTimeSeriesAggregation(ctx, apiClient, &aggregation, dataset)
		denied, err := readError(err, inNewDataset)
		if err != nil {
			// The cross-dataset collision carries its own explanation.
			if asset.IsTimeSeriesAggregationWrongDataset(err) {
				return nil, err
			}
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "time series aggregation", AssetID: plan.Key})
		}
		return []docPlan{{displayKind: "Dash0TimeSeriesAggregation", name: dash0api.GetTimeSeriesAggregationName(&aggregation), plan: plan, readDenied: denied}}, nil

	case "dataset":
		var ds dash0api.DatasetDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &ds); err != nil {
			return nil, fmt.Errorf("failed to parse Dash0Dataset: %w", err)
		}
		plan, err := asset.PlanDataset(ctx, apiClient, &ds)
		// A spec field left out of the document keeps its stored value on
		// update, so the preview is the document merged onto the stored dataset.
		if err == nil && plan.Action == asset.ActionUpdated {
			plan.Proposed, err = asset.PreviewUpdate[dash0api.DatasetDefinition](plan.Before, &ds)
		}
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
		denied, err := readError(err, inNewDataset)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "view", AssetID: plan.Key})
		}
		// The API can give a view a new id and still find it by the local id,
		// so the stored id names the same view and is not a change.
		if before, ok := plan.Before.(*dash0api.ViewDefinition); ok && before.Metadata.Labels != nil && before.Metadata.Labels.Dash0Comid != nil {
			view.Metadata.Labels.Dash0Comid = before.Metadata.Labels.Dash0Comid
		}
		return []docPlan{{displayKind: doc.Kind, name: view.Metadata.Name, plan: plan, readDenied: denied}}, nil

	case "spamfilter":
		return planSpamFilter(ctx, apiClient, doc, dataset, inNewDataset)

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
		ectx := client.ErrorContext{AssetType: "team", AssetName: dash0api.GetTeamDisplayName(&team)}
		plan, err := asset.PlanTeam(ctx, apiClient, &team)
		if err != nil {
			return nil, client.HandleAPIError(err, ectx)
		}
		// The stored team holds member ids and the document may list emails,
		// so compare both as emails. team is this preview's own copy and is
		// never sent. A failed lookup aborts like any other failed read.
		if before, ok := plan.Before.(*dash0api.TeamDefinitionV1Alpha1); ok && before != nil {
			if err := resolveMembersToEmails(ctx, apiClient, before, &team); err != nil {
				return nil, client.HandleAPIError(err, ectx)
			}
		}
		name := dash0api.GetTeamDisplayName(&team)
		if name == "" {
			name = dash0api.GetTeamName(&team)
		}
		return []docPlan{{displayKind: "Dash0Team", name: name, plan: plan}}, nil

	case "skill":
		var skill dash0api.SkillDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &skill); err != nil {
			return nil, fmt.Errorf("failed to parse Dash0Skill: %w", err)
		}
		name := asset.CustomSkillName(&skill)
		plan, err := asset.PlanCustomSkill(ctx, apiClient, &skill)
		if err != nil {
			if explained := asset.ExplainCustomSkillError(err, name); explained != nil {
				return nil, explained
			}
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "Agent0 skill", AssetName: name})
		}
		return []docPlan{{displayKind: asset.CustomSkillKind, name: name, plan: plan}}, nil

	case "awsmetricscrapeconfig":
		cfg, _, err := asset.ParseAwsMetricScrapeConfig(doc.Raw)
		if err != nil {
			return nil, err
		}
		plan, err := asset.PlanAwsMetricScrapeConfig(ctx, apiClient, cfg, doc.ID)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "AWS integration", AssetID: doc.ID})
		}
		return []docPlan{{displayKind: asset.AwsMetricScrapeConfigKind, name: dash0api.GetAwsMetricScrapeConfigName(cfg), plan: plan}}, nil

	default:
		return nil, fmt.Errorf("unsupported kind: %s", doc.Kind)
	}
}

// planCheckRules handles a CheckRule document, or the alerting rules of a
// PrometheusRule CRD: asset.ParseCheckRules returns one rule per alert.
func planCheckRules(ctx context.Context, apiClient dash0api.Client, doc asset.Document, dataset *string, inNewDataset bool) ([]docPlan, error) {
	rules, err := asset.ParseCheckRules(doc.Raw)
	if err != nil {
		return nil, err
	}
	var plans []docPlan
	for _, rule := range rules {
		plan, err := asset.PlanCheckRule(ctx, apiClient, rule, dataset)
		denied, err := readError(err, inNewDataset)
		if err != nil {
			return plans, client.HandleAPIError(err, client.ErrorContext{AssetType: "check rule", AssetID: plan.Key, AssetName: rule.Name})
		}
		plans = append(plans, docPlan{displayKind: "CheckRule", name: rule.Name, plan: plan, readDenied: denied})
	}
	return plans, nil
}

// planPrometheusRule handles a PrometheusRule CRD with alerting rules,
// recording rules, or both, the same way applyPrometheusRule dispatches it.
func planPrometheusRule(ctx context.Context, apiClient dash0api.Client, doc asset.Document, dataset *string, inNewDataset bool) ([]docPlan, error) {
	crd, err := asset.ParsePrometheusRuleCRD(doc.Raw)
	if err != nil {
		return nil, err
	}

	var plans []docPlan
	if asset.PrometheusRuleHasAlerts(crd) {
		alertPlans, err := planCheckRules(ctx, apiClient, doc, dataset, inNewDataset)
		plans = append(plans, alertPlans...)
		if err != nil {
			return plans, err
		}
	}

	if recordingOnly := asset.RecordingOnlyPrometheusRule(crd); recordingOnly != nil {
		name := dash0api.GetRecordingRuleName(recordingOnly)
		plan, err := asset.PlanRecordingRule(ctx, apiClient, recordingOnly, dataset)
		denied, err := readError(err, inNewDataset)
		if err != nil {
			return plans, client.HandleAPIError(err, client.ErrorContext{AssetType: "recording rule", AssetID: plan.Key, AssetName: name})
		}
		plans = append(plans, docPlan{displayKind: "RecordingRule", name: name, plan: plan, readDenied: denied})
	}
	return plans, nil
}

// planSpamFilter handles v1alpha1 and v1alpha2 spam filter documents. The
// apiVersion field selects the schema.
func planSpamFilter(ctx context.Context, apiClient dash0api.Client, doc asset.Document, dataset *string, inNewDataset bool) ([]docPlan, error) {
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
		denied, err := readError(err, inNewDataset)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "spam filter", AssetID: plan.Key})
		}
		return []docPlan{{displayKind: doc.Kind, name: dash0api.GetSpamFilterName(&filter), plan: plan, readDenied: denied}}, nil

	case string(dash0api.SpamFilterApiVersionV1Alpha2V1alpha2):
		var filter dash0api.SpamFilterV1Alpha2
		if err := dash0yaml.Unmarshal(doc.Raw, &filter); err != nil {
			return nil, fmt.Errorf("failed to parse v1alpha2 SpamFilter: %w", err)
		}
		plan, err := asset.PlanSpamFilterV1Alpha2(ctx, apiClient, &filter, dataset)
		denied, err := readError(err, inNewDataset)
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "spam filter", AssetID: plan.Key})
		}
		return []docPlan{{displayKind: doc.Kind, name: filter.Metadata.Name, plan: plan, readDenied: denied}}, nil

	default:
		// Unreachable: DetectSpamFilterAPIVersion only returns supported values or an error.
		return nil, fmt.Errorf("unsupported spam filter apiVersion %q", apiVersion)
	}
}

// readError drops the 403 of a read in a dataset this run creates, and
// reports that it did.
func readError(err error, inNewDataset bool) (bool, error) {
	if inNewDataset && dash0api.IsForbidden(err) {
		return true, nil
	}
	return false, err
}

// resolveMembersToEmails resolves the members of both teams with one members
// lookup instead of one per team. A team without members is left as it is.
func resolveMembersToEmails(ctx context.Context, apiClient dash0api.Client, before, local *dash0api.TeamDefinitionV1Alpha1) error {
	beforeCount := len(before.Spec.Members)
	ids := append(append([]string{}, before.Spec.Members...), local.Spec.Members...)
	if len(ids) == 0 {
		return nil
	}
	emails, err := apiClient.ResolveMemberIDsToEmails(ctx, ids)
	if err != nil {
		return err
	}
	if beforeCount > 0 {
		before.Spec.Members = emails[:beforeCount]
	}
	if len(local.Spec.Members) > 0 {
		local.Spec.Members = emails[beforeCount:]
	}
	return nil
}
