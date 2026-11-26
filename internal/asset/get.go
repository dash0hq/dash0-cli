package asset

import (
	"context"
	"fmt"
	"io"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// GetRoutedAsset reads the asset of a routed kind at key, and reports whether
// it is soft-deleted. --since reads before it deletes, because several delete
// endpoints answer an unknown key with 2xx, and the API keeps a deleted asset.
func GetRoutedAsset(ctx context.Context, apiClient dash0api.Client, kind, key string, dataset *string) (deleted bool, err error) {
	var deletedAt *time.Time
	switch kind {
	case "dashboard", "ephemeraldashboard", "persesdashboard":
		stored, getErr := apiClient.GetDashboard(ctx, key, dataset)
		deletedAt, err = DashboardDeletedAt(stored), getErr
	case "checkrule":
		stored, getErr := apiClient.GetCheckRule(ctx, key, dataset)
		deletedAt, err = CheckRuleDeletedAt(stored), getErr
	case "syntheticcheck":
		stored, getErr := apiClient.GetSyntheticCheck(ctx, key, dataset)
		deletedAt, err = SyntheticCheckDeletedAt(stored), getErr
	case "slo":
		stored, getErr := apiClient.GetSLO(ctx, key, dataset)
		deletedAt, err = SLODeletedAt(stored), getErr
	case "recordingrule":
		stored, getErr := apiClient.GetRecordingRule(ctx, key, dataset)
		deletedAt, err = RecordingRuleDeletedAt(stored), getErr
	case "view":
		stored, getErr := apiClient.GetView(ctx, key, dataset)
		deletedAt, err = ViewDeletedAt(stored), getErr
	case "spamfilter":
		_, err = apiClient.GetSpamFilter(ctx, key, dataset)
	case "notificationchannel":
		_, err = apiClient.GetNotificationChannel(ctx, key)
	case "team":
		_, err = apiClient.GetTeam(ctx, key)
	case "skill":
		stored, getErr := apiClient.GetCustomSkill(ctx, key)
		deletedAt, err = CustomSkillDeletedAt(stored), getErr
	case "generativeaievaluation":
		stored, getErr := apiClient.GetGenerativeAiEvaluation(ctx, key, dataset)
		deletedAt, err = GenerativeAiEvaluationDeletedAt(stored), getErr
	case "signaltometrics":
		stored, getErr := apiClient.GetSignalToMetrics(ctx, key, dataset)
		deletedAt, err = SignalToMetricsRuleDeletedAt(stored), getErr
	case "sampling":
		_, err = apiClient.GetSamplingRule(ctx, key, dataset)
	case "timeseriesaggregation":
		stored, getErr := apiClient.GetTimeSeriesAggregation(ctx, key, dataset)
		deletedAt, err = TimeSeriesAggregationDeletedAt(stored), getErr
	default:
		return false, fmt.Errorf("unsupported kind for a read before deletion: %s", kind)
	}
	return deletedAt != nil, err
}

// WarnNothingAtInferredOrigin reports a --since deletion that is skipped,
// because nothing is stored at the inferred origin.
func WarnNothingAtInferredOrigin(w io.Writer, origin string) {
	fmt.Fprintf(w, "warning: no asset at inferred origin %s; an asset created by an earlier dash0 version or in another dataset is not deleted\n", origin)
}
