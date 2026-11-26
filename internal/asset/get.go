package asset

import (
	"context"
	"fmt"
	"io"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// GetRoutedAsset reads the asset of a routed kind at key. --since reads before
// it deletes, because several delete endpoints answer an unknown key with 2xx.
func GetRoutedAsset(ctx context.Context, apiClient dash0api.Client, kind, key string, dataset *string) error {
	var err error
	switch kind {
	case "dashboard", "ephemeraldashboard", "persesdashboard":
		_, err = apiClient.GetDashboard(ctx, key, dataset)
	case "checkrule":
		_, err = apiClient.GetCheckRule(ctx, key, dataset)
	case "syntheticcheck":
		_, err = apiClient.GetSyntheticCheck(ctx, key, dataset)
	case "slo":
		_, err = apiClient.GetSLO(ctx, key, dataset)
	case "recordingrule":
		_, err = apiClient.GetRecordingRule(ctx, key, dataset)
	case "view":
		_, err = apiClient.GetView(ctx, key, dataset)
	case "spamfilter":
		_, err = apiClient.GetSpamFilter(ctx, key, dataset)
	case "notificationchannel":
		_, err = apiClient.GetNotificationChannel(ctx, key)
	case "team":
		_, err = apiClient.GetTeam(ctx, key)
	case "skill":
		_, err = apiClient.GetCustomSkill(ctx, key)
	case "generativeaievaluation":
		_, err = apiClient.GetGenerativeAiEvaluation(ctx, key, dataset)
	case "signaltometrics":
		_, err = apiClient.GetSignalToMetrics(ctx, key, dataset)
	case "sampling":
		_, err = apiClient.GetSamplingRule(ctx, key, dataset)
	case "timeseriesaggregation":
		_, err = apiClient.GetTimeSeriesAggregation(ctx, key, dataset)
	default:
		return fmt.Errorf("unsupported kind for a read before deletion: %s", kind)
	}
	return err
}

// WarnNothingAtInferredOrigin reports a --since deletion that is skipped,
// because nothing is stored at the inferred origin.
func WarnNothingAtInferredOrigin(w io.Writer, origin string) {
	fmt.Fprintf(w, "warning: no asset at inferred origin %s; an asset created by an earlier dash0 version or in another dataset is not deleted\n", origin)
}
