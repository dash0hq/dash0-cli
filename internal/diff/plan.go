package diff

import (
	"context"
	"fmt"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	dash0yaml "github.com/dash0hq/dash0-api-client-go/yaml"
	"github.com/dash0hq/dash0-cli/internal/asset"
	"github.com/dash0hq/dash0-cli/internal/client"
)

// docPlan is one displayed diff unit: an asset that would be created (before
// == nil) or updated (before != nil) if this document were applied. A single
// input document can expand to several docPlans — a PrometheusRule CRD with
// multiple alerts, or one that mixes alerting and recording rules.
type docPlan struct {
	displayKind string
	name        string
	id          string
	before      any // nil for a create
	after       any
}

// fetchIfExists calls fetch and classifies the result: nil (create) when id
// is empty or the asset genuinely does not exist yet, or fetch's result
// (update) when it does. Any error fetch returns other than a plain "not
// found" is surfaced as-is, so the caller's all-or-nothing fetch gate can
// abort the whole plan instead of misreporting a real failure (auth, 5xx,
// network) as a create.
func fetchIfExists(id string, fetch func() (any, error)) (any, error) {
	if id == "" {
		return nil, nil
	}
	existing, err := fetch()
	if err == nil {
		return existing, nil
	}
	if dash0api.IsNotFound(err) {
		return nil, nil
	}
	return nil, err
}

// planDocument fetches doc's current state from Dash0 (when it carries an
// identifier) without creating, updating, or deleting anything, classifying
// the document as a create (no existing asset) or an update (existing asset
// found). Mirrors applyDocument's per-kind dispatch in internal/apply, but
// stops short of ever calling a mutating asset.Import* function.
func planDocument(ctx context.Context, apiClient dash0api.Client, doc asset.Document, dataset *string) ([]docPlan, error) {
	switch asset.NormalizeKind(doc.Kind) {
	case "dashboard", "persesdashboard":
		dashboard, err := dash0yaml.ParseAsDashboard(doc.Raw)
		if err != nil {
			return nil, err
		}
		dash0api.StripDashboardServerFields(dashboard)
		id := ""
		if dashboard.Metadata.Dash0Extensions != nil && dashboard.Metadata.Dash0Extensions.Id != nil {
			id = *dashboard.Metadata.Dash0Extensions.Id
		}
		before, err := fetchIfExists(id, func() (any, error) {
			existing, err := apiClient.GetDashboard(ctx, id, dataset)
			return liveOrNil(existing, err, asset.DashboardDeletedAt)
		})
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "dashboard", AssetID: id})
		}
		name := dash0api.GetDashboardName(dashboard)
		if name == "" {
			name = dashboard.Metadata.Name
		}
		return []docPlan{{displayKind: "Dashboard", name: name, id: id, before: before, after: dashboard}}, nil

	case "checkrule":
		return planCheckRules(ctx, apiClient, doc, dataset)

	case "prometheusrule":
		return planPrometheusRule(ctx, apiClient, doc, dataset)

	case "syntheticcheck":
		var check dash0api.SyntheticCheckDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &check); err != nil {
			return nil, fmt.Errorf("failed to parse SyntheticCheck: %w", err)
		}
		dash0api.StripSyntheticCheckServerFields(&check)
		id := ""
		if check.Metadata.Labels != nil && check.Metadata.Labels.Dash0Comid != nil {
			id = *check.Metadata.Labels.Dash0Comid
		}
		before, err := fetchIfExists(id, func() (any, error) {
			existing, err := apiClient.GetSyntheticCheck(ctx, id, dataset)
			return liveOrNil(existing, err, asset.SyntheticCheckDeletedAt)
		})
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "synthetic check", AssetID: id})
		}
		return []docPlan{{displayKind: doc.Kind, name: check.Metadata.Name, id: id, before: before, after: &check}}, nil

	case "slo":
		var slo dash0api.SloDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &slo); err != nil {
			return nil, fmt.Errorf("failed to parse SLO: %w", err)
		}
		// Capture before stripping -- StripSLOServerFields clears both labels.
		// Origin wins over id, matching asset.ImportSLO.
		upsertKey := dash0api.GetSLOOrigin(&slo)
		if upsertKey == "" {
			upsertKey = dash0api.GetSLOID(&slo)
		}
		dash0api.StripSLOServerFields(&slo)
		before, err := fetchIfExists(upsertKey, func() (any, error) {
			existing, err := apiClient.GetSLO(ctx, upsertKey, dataset)
			return liveOrNil(existing, err, asset.SLODeletedAt)
		})
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "SLO", AssetID: upsertKey})
		}
		return []docPlan{{displayKind: "SLO", name: dash0api.GetSLOName(&slo), id: upsertKey, before: before, after: &slo}}, nil

	case "timeseriesaggregation":
		var aggregation dash0api.TimeSeriesAggregationDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &aggregation); err != nil {
			return nil, fmt.Errorf("failed to parse Dash0TimeSeriesAggregation: %w", err)
		}
		// Origin is the only upsert key for this kind; ValidateDocuments
		// already rejected a document without one.
		origin := asset.GetTimeSeriesAggregationOrigin(&aggregation)
		dash0api.StripTimeSeriesAggregationServerFields(&aggregation)
		before, err := fetchIfExists(origin, func() (any, error) {
			existing, err := apiClient.GetTimeSeriesAggregation(ctx, origin, dataset)
			return liveOrNil(existing, err, asset.TimeSeriesAggregationDeletedAt)
		})
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "time series aggregation", AssetID: origin})
		}
		return []docPlan{{displayKind: "Dash0TimeSeriesAggregation", name: dash0api.GetTimeSeriesAggregationName(&aggregation), id: origin, before: before, after: &aggregation}}, nil

	case "dataset":
		var ds dash0api.DatasetDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &ds); err != nil {
			return nil, fmt.Errorf("failed to parse Dash0Dataset: %w", err)
		}
		// Organization-level: no dataset parameter, matching asset.ImportDataset.
		slug := dash0api.GetDatasetID(&ds)
		dash0api.StripDatasetServerFields(&ds)
		before, err := fetchIfExists(slug, func() (any, error) { return apiClient.GetDataset(ctx, slug) })
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "dataset", AssetID: slug})
		}
		return []docPlan{{displayKind: "Dash0Dataset", name: dash0api.GetDatasetName(&ds), id: slug, before: before, after: &ds}}, nil

	case "view":
		var view dash0api.ViewDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &view); err != nil {
			return nil, fmt.Errorf("failed to parse View: %w", err)
		}
		dash0api.StripViewServerFields(&view)
		id := ""
		if view.Metadata.Labels != nil && view.Metadata.Labels.Dash0Comid != nil {
			id = *view.Metadata.Labels.Dash0Comid
		}
		before, err := fetchIfExists(id, func() (any, error) {
			existing, err := apiClient.GetView(ctx, id, dataset)
			return liveOrNil(existing, err, asset.ViewDeletedAt)
		})
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "view", AssetID: id})
		}
		return []docPlan{{displayKind: doc.Kind, name: view.Metadata.Name, id: id, before: before, after: &view}}, nil

	case "spamfilter":
		return planSpamFilter(ctx, apiClient, doc, dataset)

	case "notificationchannel":
		var channel dash0api.NotificationChannelDefinition
		if err := dash0yaml.Unmarshal(doc.Raw, &channel); err != nil {
			return nil, fmt.Errorf("failed to parse Dash0NotificationChannel: %w", err)
		}
		dash0api.StripNotificationChannelServerFields(&channel)
		origin := dash0api.GetNotificationChannelOrigin(&channel)
		before, err := fetchIfExists(origin, func() (any, error) { return apiClient.GetNotificationChannel(ctx, origin) })
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "notification channel", AssetID: origin})
		}
		return []docPlan{{displayKind: "Dash0NotificationChannel", name: dash0api.GetNotificationChannelName(&channel), id: origin, before: before, after: &channel}}, nil

	case "team":
		var team dash0api.TeamDefinitionV1Alpha1
		if err := dash0yaml.Unmarshal(doc.Raw, &team); err != nil {
			return nil, fmt.Errorf("failed to parse Dash0Team: %w", err)
		}
		// Capture before stripping -- StripTeamServerFields clears the
		// dash0.com/id label along with the server source label.
		origin := dash0api.GetTeamOrigin(&team)
		id := dash0api.GetTeamID(&team)
		dash0api.StripTeamServerFields(&team)
		upsertKey := origin
		if upsertKey == "" {
			upsertKey = id
		}
		before, err := fetchIfExists(upsertKey, func() (any, error) { return apiClient.GetTeam(ctx, upsertKey) })
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "team", AssetID: upsertKey})
		}
		name := dash0api.GetTeamDisplayName(&team)
		if name == "" {
			name = dash0api.GetTeamName(&team)
		}
		return []docPlan{{displayKind: "Dash0Team", name: name, id: upsertKey, before: before, after: &team}}, nil

	default:
		return nil, fmt.Errorf("unsupported kind: %s", doc.Kind)
	}
}

// planCheckRules handles a single CheckRule (native, non-CRD) document, or
// the alerting rules extracted from a PrometheusRule CRD (called from
// planPrometheusRule) -- asset.ParseCheckRules returns one rule per alert in
// document order either way.
func planCheckRules(ctx context.Context, apiClient dash0api.Client, doc asset.Document, dataset *string) ([]docPlan, error) {
	rules, err := asset.ParseCheckRules(doc.Raw)
	if err != nil {
		return nil, err
	}
	var plans []docPlan
	for _, rule := range rules {
		dash0api.StripCheckRuleServerFields(rule)
		id := ""
		if rule.Id != nil {
			id = *rule.Id
		}
		before, err := fetchIfExists(id, func() (any, error) {
			existing, err := apiClient.GetCheckRule(ctx, id, dataset)
			return liveOrNil(existing, err, asset.CheckRuleDeletedAt)
		})
		if err != nil {
			return plans, client.HandleAPIError(err, client.ErrorContext{AssetType: "check rule", AssetID: id, AssetName: rule.Name})
		}
		plans = append(plans, docPlan{displayKind: "CheckRule", name: rule.Name, id: id, before: before, after: rule})
	}
	return plans, nil
}

// planPrometheusRule handles a PrometheusRule CRD that may contain alerting
// rules, recording rules, or both -- mirroring applyPrometheusRule's dispatch
// in internal/apply, but only fetching, never writing.
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

	recordingOnly := asset.RecordingOnlyPrometheusRule(crd)
	if recordingOnly != nil {
		// Capture before stripping -- StripRecordingRuleServerFields clears
		// the dash0.com/id label.
		id := dash0api.GetRecordingRuleID(recordingOnly)
		dash0api.StripRecordingRuleServerFields(recordingOnly)
		before, err := fetchIfExists(id, func() (any, error) {
			existing, err := apiClient.GetRecordingRule(ctx, id, dataset)
			return liveOrNil(existing, err, asset.RecordingRuleDeletedAt)
		})
		if err != nil {
			return plans, client.HandleAPIError(err, client.ErrorContext{AssetType: "recording rule", AssetID: id, AssetName: dash0api.GetRecordingRuleName(recordingOnly)})
		}
		plans = append(plans, docPlan{displayKind: "RecordingRule", name: dash0api.GetRecordingRuleName(recordingOnly), id: id, before: before, after: recordingOnly})
	}

	return plans, nil
}

// planSpamFilter handles both v1alpha1 and v1alpha2 spam filter documents.
// The apiVersion field on the document selects the schema; an unknown value
// is rejected before any API call.
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
		// Capture before stripping -- StripSpamFilterServerFields clears the
		// dash0.com/id label along with the server source label.
		id := dash0api.GetSpamFilterID(&filter)
		origin := ""
		if filter.Metadata.Labels != nil && filter.Metadata.Labels.Dash0Comorigin != nil {
			origin = *filter.Metadata.Labels.Dash0Comorigin
		}
		dash0api.StripSpamFilterServerFields(&filter)
		upsertKey := origin
		if upsertKey == "" {
			upsertKey = id
		}
		before, err := fetchIfExists(upsertKey, func() (any, error) { return apiClient.GetSpamFilter(ctx, upsertKey, dataset) })
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "spam filter", AssetID: upsertKey})
		}
		return []docPlan{{displayKind: doc.Kind, name: dash0api.GetSpamFilterName(&filter), id: upsertKey, before: before, after: &filter}}, nil

	case string(dash0api.V1alpha2):
		var filter dash0api.SpamFilterV1Alpha2
		if err := dash0yaml.Unmarshal(doc.Raw, &filter); err != nil {
			return nil, fmt.Errorf("failed to parse v1alpha2 SpamFilter: %w", err)
		}
		id := asset.SpamFilterV1Alpha2ID(&filter)
		origin := ""
		if filter.Metadata.Labels != nil && filter.Metadata.Labels.Dash0Comorigin != nil {
			origin = *filter.Metadata.Labels.Dash0Comorigin
		}
		upsertKey := origin
		if upsertKey == "" {
			upsertKey = id
		}
		before, err := fetchIfExists(upsertKey, func() (any, error) { return apiClient.GetSpamFilter(ctx, upsertKey, dataset) })
		if err != nil {
			return nil, client.HandleAPIError(err, client.ErrorContext{AssetType: "spam filter", AssetID: upsertKey})
		}
		return []docPlan{{displayKind: doc.Kind, name: filter.Metadata.Name, id: upsertKey, before: before, after: &filter}}, nil

	default:
		// Unreachable: DetectSpamFilterAPIVersion only returns supported values or an error.
		return nil, fmt.Errorf("unsupported spam filter apiVersion %q", apiVersion)
	}
}

// liveOrNil turns a soft-deleted asset into "not found", so diff reports it as
// a create: apply restores it rather than diffing against the deleted copy,
// whose deleted-at marker the diff normalization strips anyway.
func liveOrNil[T any](existing *T, err error, deletedAt func(*T) *time.Time) (any, error) {
	if err != nil {
		return nil, err
	}
	if deletedAt(existing) != nil {
		return nil, nil
	}
	return existing, nil
}
