package asset

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	dash0api "github.com/dash0hq/dash0-api-client-go"
)

// ErrTimeSeriesAggregationMissingOrigin is returned when a time series
// aggregation document carries no dash0.com/origin label.
//
// Origin is mandatory for this kind, so there is no create path to fall back
// to. Failing during validation stops a multi-document apply from writing half
// its documents first.
var ErrTimeSeriesAggregationMissingOrigin = fmt.Errorf(
	"Dash0TimeSeriesAggregation requires %s: the Dash0 API rejects a time series aggregation without an origin, and the origin is the key `apply` upserts by",
	OriginLabel,
)

// GetTimeSeriesAggregationOrigin extracts the dash0.com/origin label. The API
// client ships accessors for the id, name, and dataset labels, but not this one.
func GetTimeSeriesAggregationOrigin(aggregation *dash0api.TimeSeriesAggregationDefinition) string {
	if aggregation == nil || aggregation.Metadata.Labels == nil || aggregation.Metadata.Labels.Dash0Comorigin == nil {
		return ""
	}
	return *aggregation.Metadata.Labels.Dash0Comorigin
}

// ImportTimeSeriesAggregation upserts a time series aggregation by its
// dash0.com/origin label.
//
// Unlike the other Import helpers, this never sends a POST. Origin is
// mandatory on create and POST rejects an origin that already exists, so
// PUT /{origin} is the only idempotent path.
//
// Read the origin before StripTimeSeriesAggregationServerFields, which clears
// the label. Reading it after would yield "" on every apply.
//
// The body's dash0.com/id is left alone. The server ignores it and uses the
// path's origin, so an exported document reapplies safely.
func ImportTimeSeriesAggregation(
	ctx context.Context,
	apiClient dash0api.Client,
	aggregation *dash0api.TimeSeriesAggregationDefinition,
	dataset *string,
) (ImportResult, error) {
	origin := GetTimeSeriesAggregationOrigin(aggregation)
	if origin == "" {
		return ImportResult{}, ErrTimeSeriesAggregationMissingOrigin
	}

	dash0api.StripTimeSeriesAggregationServerFields(aggregation)

	action := ActionCreated
	var before any
	existing, err := apiClient.GetTimeSeriesAggregation(ctx, origin, dataset)
	switch {
	case err == nil:
		action = ActionUpdated
		before = existing
	case dash0api.IsNotFound(err):
		// The aggregation does not exist yet, so the PUT below creates it and
		// ActionCreated is correct.
	default:
		// Whether the aggregation exists is now unknown. The PUT would fail
		// the same way, so stopping here only avoids a write against unknown
		// state and keeps Action honest. A 403 is the common case, since every
		// endpoint for this kind needs the admin role. The cost is that a
		// transient GET error fails a call the PUT might have completed.
		return ImportResult{}, wrapIfWrongDataset(err, origin)
	}

	result, err := apiClient.UpdateTimeSeriesAggregation(ctx, origin, aggregation, dataset)
	if err != nil {
		return ImportResult{}, wrapIfWrongDataset(err, origin)
	}

	return ImportResult{
		Name:   dash0api.GetTimeSeriesAggregationName(result),
		ID:     dash0api.GetTimeSeriesAggregationID(result),
		Action: action,
		Before: before,
		After:  result,
	}, nil
}

// IsTimeSeriesAggregationWrongDataset reports whether err is the API's
// cross-dataset origin collision: HTTP 400 with a message saying the origin
// exists but belongs to a different dataset.
//
// Origins are unique per organization, while each aggregation belongs to one
// dataset. No other asset kind combines the two, so the resulting 400 is easy
// to mistake for a malformed document. It must not be treated as a 404 either:
// the aggregation exists, so swallowing it under --force would report success
// while it is still live.
func IsTimeSeriesAggregationWrongDataset(err error) bool {
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
