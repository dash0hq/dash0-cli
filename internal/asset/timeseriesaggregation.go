package asset

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// GetTimeSeriesAggregationOrigin extracts the dash0.com/origin label. The API
// client ships accessors for the id, name, and dataset labels, but not this one.
func GetTimeSeriesAggregationOrigin(aggregation *dash0api.TimeSeriesAggregationDefinition) string {
	if aggregation == nil || aggregation.Metadata.Labels == nil || aggregation.Metadata.Labels.Dash0Comorigin == nil {
		return ""
	}
	return *aggregation.Metadata.Labels.Dash0Comorigin
}

// ImportTimeSeriesAggregation creates or updates a time series aggregation
// through RouteDocument. The path carries the origin, so the body never does.
// A PUT to a deleted aggregation brings it back, which is reported as
// ActionRestored.
func ImportTimeSeriesAggregation(
	ctx context.Context,
	apiClient dash0api.Client,
	aggregation *dash0api.TimeSeriesAggregationDefinition,
	dataset *string,
) (ImportResult, error) {
	plan, err := PlanTimeSeriesAggregation(ctx, apiClient, aggregation, dataset)
	if err != nil {
		return ImportResult{}, err
	}

	result, err := apiClient.UpdateTimeSeriesAggregation(ctx, plan.Key, aggregation, dataset)
	if err != nil {
		return ImportResult{}, wrapIfWrongDataset(err, plan.Key)
	}
	if err := RequireNotDeleted("Time series aggregation", dash0api.GetTimeSeriesAggregationName(result), dash0api.GetTimeSeriesAggregationID(result), TimeSeriesAggregationDeletedAt(result)); err != nil {
		return ImportResult{}, err
	}

	return ImportResult{
		Name:   dash0api.GetTimeSeriesAggregationName(result),
		ID:     dash0api.GetTimeSeriesAggregationID(result),
		Action: plan.Action,
		Before: plan.Before,
		After:  result,
	}, nil
}

// PlanTimeSeriesAggregation routes the aggregation with RouteDocument and reads
// the stored aggregation that ImportTimeSeriesAggregation would write.
//
// Any preflight error other than "not found" is returned, because the PUT
// would fail the same way. A 403 is the common case, since every endpoint for
// this kind needs the admin role.
func PlanTimeSeriesAggregation(ctx context.Context, apiClient dash0api.Client, aggregation *dash0api.TimeSeriesAggregationDefinition, dataset *string) (ImportPlan, error) {
	// The strip clears the origin label, so read it first.
	identity := TimeSeriesAggregationIdentity(aggregation, dataset)
	dash0api.StripTimeSeriesAggregationServerFields(aggregation)
	get := func(ctx context.Context, key string) (*dash0api.TimeSeriesAggregationDefinition, error) {
		return apiClient.GetTimeSeriesAggregation(ctx, key, dataset)
	}
	route, err := RouteDocument(ctx, identity, get, (*dash0api.TimeSeriesAggregationDefinition).GetOrigin)
	plan := ImportPlan{Key: route.Key, Action: ActionCreated, Proposed: aggregation}
	if err != nil {
		return plan, wrapIfWrongDataset(err, identity.CreateOrigin())
	}
	if !route.Create {
		plan.Action, plan.Before = actionForExisting(TimeSeriesAggregationDeletedAt(route.Stored)), route.Stored
	}
	return plan, nil
}

// TimeSeriesAggregationIdentity returns what RouteDocument routes a time series
// aggregation by.
func TimeSeriesAggregationIdentity(aggregation *dash0api.TimeSeriesAggregationDefinition, dataset *string) Identity {
	return identityOf("timeseriesaggregation", aggregation.Metadata.Name, dataset, aggregation)
}

// TimeSeriesAggregationDeletedAt returns when the aggregation was deleted, or
// nil when it is not deleted.
func TimeSeriesAggregationDeletedAt(aggregation *dash0api.TimeSeriesAggregationDefinition) *time.Time {
	if aggregation == nil || aggregation.Metadata.Annotations == nil {
		return nil
	}
	return aggregation.Metadata.Annotations.Dash0ComdeletedAt
}

// IsTimeSeriesAggregationWrongDataset reports whether err is the API's
// cross-dataset origin collision: HTTP 400 with a message saying the origin
// exists but belongs to a different dataset.
//
// Origins are unique per organization, while each aggregation belongs to one
// dataset. Only aggregations and signal-to-metrics rules combine the two, so
// the resulting 400 is easy to mistake for a malformed document. It must not be
// treated as a 404 either: the aggregation exists, so swallowing it under
// --force would report success while it is still live.
func IsTimeSeriesAggregationWrongDataset(err error) bool {
	return isOriginInAnotherDataset(err)
}

// isOriginInAnotherDataset detects the cross-dataset origin collision that the
// time series aggregation and signal-to-metrics endpoints return with the same
// message.
func isOriginInAnotherDataset(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *dash0api.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		return false
	}
	// Matching on the text is unavoidable, because the 400 carries no
	// machine-readable discriminator. The raw body is the fallback, since
	// Message is only set when the SDK finds a message field.
	haystack := strings.ToLower(apiErr.Message + " " + apiErr.Body)
	return strings.Contains(haystack, "associated with a different dataset")
}

// WrapTimeSeriesAggregationWrongDataset turns the API's cross-dataset 400 into
// an error that names the cause and the fix. The API states the fact but not
// what to do about it, and the fix is counterintuitive: origins are
// organization-wide, so one document cannot serve two datasets.
//
// The advice goes behind "\nHint:" so agent mode lifts it into the JSON
// error's hint field, as every other actionable error here does (see
// agentmode.PrintJSONError).
func WrapTimeSeriesAggregationWrongDataset(err error, origin string) error {
	return fmt.Errorf(
		"time series aggregation origin %q already exists in another dataset:\n  %w"+
			"\nHint: origins are unique per organization while each aggregation belongs to one dataset, "+
			"so one document cannot be applied to two datasets. Use a distinct origin per dataset, "+
			"or apply this document only to the dataset that owns it",
		origin, err,
	)
}

// wrapIfWrongDataset adds the cross-dataset explanation and passes any other
// error through. Both the preflight GET and the PUT can return that 400, and
// the rule is the same for either.
func wrapIfWrongDataset(err error, origin string) error {
	if IsTimeSeriesAggregationWrongDataset(err) {
		return WrapTimeSeriesAggregationWrongDataset(err, origin)
	}
	return err
}
