package timeseriesaggregations

import (
	"fmt"

	dash0api "github.com/dash0hq/dash0-api-client-go"
	"github.com/dash0hq/dash0-cli/internal/asset"
	sigsyaml "sigs.k8s.io/yaml"
)

// assetType is the display name used in error messages and ErrorContext.
const assetType = "time series aggregation"

// displayKind is the display name used in success messages and diffs, per the
// asset kind display-name table in docs/cli-naming-conventions.md.
const displayKind = "Time series aggregation"

// decode parses a YAML or JSON time series aggregation document.
func decode(raw []byte) (*dash0api.TimeSeriesAggregationDefinition, error) {
	var aggregation dash0api.TimeSeriesAggregationDefinition
	if err := sigsyaml.Unmarshal(raw, &aggregation); err != nil {
		return nil, fmt.Errorf("failed to parse time series aggregation definition: %w", err)
	}
	return &aggregation, nil
}

// interval renders spec.sample.interval, which is required, so an empty result
// means the document is malformed rather than the field being optional.
func interval(aggregation *dash0api.TimeSeriesAggregationDefinition) string {
	if aggregation == nil {
		return ""
	}
	return string(aggregation.Spec.Sample.Interval)
}

// origin is a thin alias over the asset helper so the command files read the
// same way they do for the other labels.
func origin(aggregation *dash0api.TimeSeriesAggregationDefinition) string {
	return asset.GetTimeSeriesAggregationOrigin(aggregation)
}
