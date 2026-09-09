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
// Unlike every other asset kind, origin is mandatory here: the API rejects a
// POST without it, and rejects a POST whose origin already exists, so there is
// no create path the CLI can fall back to. Failing during validation keeps a
// multi-document apply from writing half its documents before hitting this.
var ErrTimeSeriesAggregationMissingOrigin = fmt.Errorf(
	"Dash0TimeSeriesAggregation requires %s: the Dash0 API rejects a time series aggregation without an origin, and the origin is the key `apply` upserts by",
	OriginLabel,
)

// GetTimeSeriesAggregationOrigin extracts the dash0.com/origin label from a
// time series aggregation definition. The API client ships no accessor for it,
// unlike the id/name/dataset labels.
func GetTimeSeriesAggregationOrigin(aggregation *dash0api.TimeSeriesAggregationDefinition) string {
	if aggregation == nil || aggregation.Metadata.Labels == nil || aggregation.Metadata.Labels.Dash0Comorigin == nil {
		return ""
	}
	return *aggregation.Metadata.Labels.Dash0Comorigin
}

// ImportTimeSeriesAggregation upserts a time series aggregation by its
// dash0.com/origin label.
//
// This diverges from every other Import helper, which branches between POST and
// PUT on whether the document carries an identifier. The reason is the API, not
// a CLI preference: origin is mandatory on create, and POST rejects an origin
// that already exists, so the POST branch is unreachable for any workflow that
// runs twice. PUT /{origin} is create-or-replace and is therefore the only
// idempotent path. CreateTimeSeriesAggregation is never called from here.
//
// The origin is read before StripTimeSeriesAggregationServerFields runs,
// because that helper clears the origin label along with version, source,
// dataset, and the annotation timestamps. Reading it afterwards would yield ""
// on every apply and turn a working upsert into a validation failure.
//
// The body's dash0.com/id is left alone. The server ignores it and targets the
// path's origin — verified against the aggregation's own id, a nonexistent id,
// and another aggregation's id — so an exported document reapplies safely.
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
		// Any other error leaves it unknown whether the aggregation exists.
		// The PUT would surface the same error either way, so what failing
		// here buys is not surfacing the cause but skipping a write against
		// unknown state, and keeping Action honest instead of reporting a
		// create for what may be an update. Every endpoint for this asset
		// kind requires the organization admin role, so a 403 is the common
		// case; the cost is that a transient error on the GET now fails a
		// call the PUT might have completed.
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
// Time series aggregation origins are unique per organization, while each
// aggregation belongs to exactly one dataset. No other asset kind combines the
// two, and the resulting 400 is easy to mistake for a malformed document.
// It must never be treated as a 404 either: the aggregation exists, so
// swallowing it under --force would report success while it is still live.
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
	// Matching on the message text is unavoidable: the API returns a plain 400
	// with no machine-readable discriminator. Fall back to the raw body, since
	// Message is only populated when the SDK finds a message field.
	haystack := strings.ToLower(apiErr.Message + " " + apiErr.Body)
	return strings.Contains(haystack, "associated with a different dataset")
}

// WrapTimeSeriesAggregationWrongDataset turns the API's cross-dataset 400 into
// an error that names the cause and the fix. The API's own message states the
// fact but not what to do, and the fix is counterintuitive: origins are
// organization-wide, so the same document cannot serve two datasets the way it
// can for every other asset kind.
//
// The advice goes behind "\nHint:" so agent mode lifts it into the JSON
// error's own hint field, the same way every other actionable error in this
// CLI does (see agentmode.PrintJSONError).
func WrapTimeSeriesAggregationWrongDataset(err error, origin string) error {
	return fmt.Errorf(
		"time series aggregation origin %q already exists in another dataset:\n  %w"+
			"\nHint: origins are unique per organization while each aggregation belongs to one dataset, "+
			"so one document cannot be applied to two datasets. Use a distinct origin per dataset, "+
			"or apply this document only to the dataset that owns it",
		origin, err,
	)
}

// wrapIfWrongDataset applies the cross-dataset explanation when the error is
// that, and passes anything else through untouched. Both the preflight GET and
// the PUT can return the cross-dataset 400, and the rule for it is the same on
// either, so it is written once here rather than at each call site.
func wrapIfWrongDataset(err error, origin string) error {
	if IsTimeSeriesAggregationWrongDataset(err) {
		return WrapTimeSeriesAggregationWrongDataset(err, origin)
	}
	return err
}
